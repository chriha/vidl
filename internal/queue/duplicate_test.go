package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/chriha/vidl/internal/ytdlp"
)

func TestVideoIDPersists(t *testing.T) {
	s := tempStore(t)
	if err := s.Save(State{Queue: []Item{{ID: "a", URL: "https://youtu.be/x", Extractor: "Youtube", VideoID: "x"}}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"video_id": "x"`) {
		t.Errorf("queue.json lacks video_id:\n%s", data)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Queue) != 1 || got.Queue[0].VideoID != "x" {
		t.Errorf("queue = %+v", got.Queue)
	}
}

func TestLoadWithoutVideoID(t *testing.T) {
	got, err := FileStore{Path: "testdata/queue_legacy.json"}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Queue) != 1 || got.Queue[0].URL != "a" || got.Queue[0].VideoID != "" {
		t.Errorf("queue = %+v", got.Queue)
	}
}

func TestAddKeepsVideoID(t *testing.T) {
	m, _ := queuedBehindBusy(t, &MemStore{})
	id := m.Add(NewItem{URL: "https://youtu.be/x", Extractor: "Youtube", VideoID: "x"})[0]
	if it, _ := m.Get(id); it.VideoID != "x" {
		t.Errorf("VideoID = %q, want x", it.VideoID)
	}
}

func TestEstimateFillsEmptyVideoID(t *testing.T) {
	est := &ytdlp.Estimate{Size: 1, ID: "from-estimate"}
	tests := []struct {
		name, id, want string
	}{
		{"empty id is filled", "", "from-estimate"},
		{"set id is kept", "probed", "probed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, r := queuedBehindBusy(t, &MemStore{})
			id := m.Add(NewItem{URL: "a", VideoID: tt.id})[0]
			r.waitEstimating(t, "a")
			r.answerEstimate("a", est, nil)
			waitFor(t, "estimate", func() bool { return estimatedSize(m, id) == 1 })
			if it, _ := m.Get(id); it.VideoID != tt.want {
				t.Errorf("VideoID = %q, want %q", it.VideoID, tt.want)
			}
		})
	}
}

func TestFindDuplicate(t *testing.T) {
	store := &MemStore{State: State{
		Queue: []Item{
			{ID: "downloading", URL: "https://www.youtube.com/watch?v=dl", Extractor: "Youtube", VideoID: "dl", Status: Queued},
			{ID: "queued", URL: "https://www.youtube.com/watch?v=q", Extractor: "Youtube", VideoID: "q", Status: Queued},
			{ID: "paused", URL: "https://www.youtube.com/watch?v=p", Extractor: "Youtube", VideoID: "p", Status: Paused},
			{ID: "failed", URL: "https://www.youtube.com/watch?v=f", Extractor: "Youtube", VideoID: "f", Status: Failed},
			{ID: "cancelled", URL: "https://www.youtube.com/watch?v=c", Extractor: "Youtube", VideoID: "c", Status: Cancelled},
		},
		History: []Item{
			{ID: "finished", URL: "https://www.youtube.com/watch?v=fin", Extractor: "Youtube", VideoID: "fin", Status: Finished},
			{ID: "old", URL: "https://x.test/old", Status: Finished},
		},
	}}
	m, r := newManager(t, 1, store)
	r.waitStarted(t, "https://www.youtube.com/watch?v=dl")

	tests := []struct {
		name string
		req  NewItem
		want string // id of the duplicate; empty for none
	}{
		{"id match across different URLs", NewItem{URL: "https://youtu.be/fin", Extractor: "Youtube", VideoID: "fin"}, "finished"},
		{"extractor compared case-insensitively", NewItem{URL: "https://youtu.be/fin", Extractor: "youtube", VideoID: "fin"}, "finished"},
		{"downloading counts", NewItem{URL: "https://youtu.be/dl", Extractor: "Youtube", VideoID: "dl"}, "downloading"},
		{"queued counts", NewItem{URL: "https://youtu.be/q", Extractor: "Youtube", VideoID: "q"}, "queued"},
		{"paused counts", NewItem{URL: "https://youtu.be/p", Extractor: "Youtube", VideoID: "p"}, "paused"},
		{"URL fallback when neither has an id", NewItem{URL: "https://x.test/old"}, "old"},
		{"URL fallback when the request has no id", NewItem{URL: "https://www.youtube.com/watch?v=fin"}, "finished"},
		{"no URL fallback for a short link without an id", NewItem{URL: "https://youtu.be/fin"}, ""},
		{"extractor mismatch", NewItem{URL: "https://vimeo.com/fin", Extractor: "Vimeo", VideoID: "fin"}, ""},
		{"failed is ignored", NewItem{URL: "https://www.youtube.com/watch?v=f", Extractor: "Youtube", VideoID: "f"}, ""},
		{"cancelled is ignored", NewItem{URL: "https://www.youtube.com/watch?v=c"}, ""},
		{"no match", NewItem{URL: "https://x.test/new", Extractor: "Generic", VideoID: "new"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := m.FindDuplicate(tt.req)
			if ok != (tt.want != "") || got.ID != tt.want {
				t.Errorf("FindDuplicate = %q, %v; want %q", got.ID, ok, tt.want)
			}
		})
	}
}
