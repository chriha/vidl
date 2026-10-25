package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/chriha/vidl/internal/config"
	"github.com/chriha/vidl/internal/ytdlp"
)

// Runner runs a single download and estimates its size beforehand.
// ytdlp.Runner implements it.
type Runner interface {
	Run(ctx context.Context, job ytdlp.Job, onEvent func(ytdlp.Event)) error
	Estimate(ctx context.Context, url string, f ytdlp.Format, resolution, cookiesFromBrowser string) (*ytdlp.Estimate, error)
}

// maxEstimates is how many size estimates run at once, apart from downloads.
const maxEstimates = 2

// Reasons for stopping a running download, passed as context cancel causes.
var (
	errPaused    = errors.New("paused")
	errCancelled = errors.New("cancelled")
	errRemoved   = errors.New("removed")
	errShutdown  = errors.New("shutting down")
)

var ErrNotFound = errors.New("download not found")

// NewItem is a request to queue a download.
type NewItem struct {
	URL        string
	Title      string
	Uploader   string
	Duration   float64
	Extractor  string
	Format     ytdlp.Format
	Resolution string

	// Optional overrides of the settings; zero values follow the settings.
	Clip      ytdlp.Clip // single videos only
	Template  string     // filename template
	Subtitles *bool      // embed subtitles

	// SponsorBlock false skips the sponsorblock_remove setting; nil follows it.
	SponsorBlock *bool

	// Paused adds the item paused: it waits for Start or Requeue instead of
	// starting once a slot is free.
	Paused bool
}

// Manager owns the download queue and history. Queued items start
// automatically, in queue order, while fewer than Concurrency downloads run.
// All methods are safe for concurrent use.
type Manager struct {
	mu      sync.Mutex
	cfg     config.Config
	runner  Runner
	store   Store
	queue   []*Item // not finished, in queue order
	history []*Item // finished, most recent first
	active  map[string]context.CancelCauseFunc
	closed  bool
	held    bool // session only: scheduleLocked starts nothing
	saveErr error
	wg      sync.WaitGroup
	changes chan struct{}

	// Session only: Stop holds the queue and records the ids it paused,
	// which Resume requeues.
	stopped    bool
	stopPaused map[string]bool

	estCtx     context.Context
	estStop    context.CancelFunc
	estimating int             // estimates running
	estTried   map[string]bool // ids estimated (or being estimated) this session

	// Wait timer: after a download ends, nothing starts automatically until
	// the countdown expires. Running while waitTimer is set (until
	// waitUntil), frozen with waitLeft otherwise.
	waiting   bool
	waitUntil time.Time
	waitLeft  time.Duration
	waitTimer waitTimer
	waitGen   int // bumped on every stop, so stale timer callbacks do nothing

	// Clock of the wait timer, replaced by tests.
	now       func() time.Time
	afterFunc func(time.Duration, func()) waitTimer
}

// New loads the persisted state and starts queued downloads. Downloads that
// were interrupted by a previous shutdown are queued again.
func New(cfg config.Config, runner Runner, store Store) (*Manager, error) {
	st, err := store.Load()
	m := &Manager{
		cfg:        cfg.Normalize(),
		runner:     runner,
		store:      store,
		active:     map[string]context.CancelCauseFunc{},
		changes:    make(chan struct{}, 1),
		estTried:   map[string]bool{},
		stopPaused: map[string]bool{},
		now:        time.Now,
		afterFunc:  func(d time.Duration, f func()) waitTimer { return time.AfterFunc(d, f) },
	}
	m.estCtx, m.estStop = context.WithCancel(context.Background())
	for _, it := range st.Queue {
		if it.Status == Downloading || it.Status == Finished {
			it.Status = Queued
		}
		it.Progress.Speed, it.Progress.ETA = 0, 0
		m.queue = append(m.queue, &it)
	}
	for _, it := range st.History {
		m.history = append(m.history, &it)
	}
	m.mu.Lock()
	m.scheduleLocked()
	m.mu.Unlock()
	return m, err
}

// Changes delivers a value whenever the state changed. Notifications are
// coalesced: a single receive may stand for many changes.
func (m *Manager) Changes() <-chan struct{} {
	return m.changes
}

func (m *Manager) notify() {
	select {
	case m.changes <- struct{}{}:
	default:
	}
}

// Snapshot returns copies of the queue, with running downloads first, and
// of the history, most recently finished first.
func (m *Manager) Snapshot() (queue, history []Item) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, it := range m.displayOrderLocked() {
		queue = append(queue, it.clone())
	}
	for _, it := range m.history {
		history = append(history, it.clone())
	}
	return queue, history
}

// Get returns a copy of the item with the given id.
func (m *Manager) Get(id string) (Item, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if it := m.findLocked(id); it != nil {
		return it.clone(), true
	}
	return Item{}, false
}

// Config returns the current settings.
func (m *Manager) Config() config.Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// SetConfig updates the settings. Running downloads keep their settings; a
// lower concurrency takes effect as running downloads finish. Disabling the
// wait timer drops a running countdown; a new duration applies to the next.
func (m *Manager) SetConfig(cfg config.Config) {
	m.mu.Lock()
	m.cfg = cfg.Normalize()
	if !m.cfg.WaitEnabled {
		m.dropWaitLocked()
	}
	m.scheduleLocked()
	m.mu.Unlock()
	m.notify()
}

// SetHeld sets the queue-wide hold. While held, nothing starts automatically
// (running downloads finish; Start still starts an item) and the wait
// countdown is frozen; releasing it fills the free slots, and releasing a
// stopped queue works like Resume. The hold is not persisted.
func (m *Manager) SetHeld(held bool) {
	m.mu.Lock()
	if !held && m.stopped {
		m.resumeLocked()
	}
	m.held = held
	if held {
		m.freezeWaitLocked()
	} else {
		m.runWaitLocked()
	}
	m.scheduleLocked()
	m.mu.Unlock()
	m.notify()
}

// Held reports whether the queue is held.
func (m *Manager) Held() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.held
}

// Stop pauses every running download, keeping partial files, and holds the
// queue. It returns the number of downloads it paused.
func (m *Manager) Stop() int {
	m.mu.Lock()
	m.stopped, m.held = true, true
	m.freezeWaitLocked()
	n := 0
	for _, it := range m.queue {
		if it.Status == Downloading {
			// The worker sets Paused once yt-dlp has exited.
			m.active[it.ID](errPaused)
			m.stopPaused[it.ID] = true
			n++
		}
	}
	m.mu.Unlock()
	m.notify()
	return n
}

// Resume requeues the downloads Stop paused that are still paused and lifts
// the hold. It returns the number of downloads it requeued.
func (m *Manager) Resume() int {
	m.mu.Lock()
	n := m.resumeLocked()
	m.held = false
	m.runWaitLocked()
	m.scheduleLocked()
	m.saveLocked()
	m.mu.Unlock()
	m.notify()
	return n
}

// resumeLocked requeues the stop-paused downloads and ends the stop. A download
// whose pause is still in flight stays in the set and is requeued by its worker.
func (m *Manager) resumeLocked() int {
	m.stopped = false
	n := 0
	for _, it := range m.queue {
		if !m.stopPaused[it.ID] {
			continue
		}
		if it.Status == Downloading {
			n++ // its worker requeues it
			continue
		}
		if it.Status == Paused {
			it.Status = Queued
			n++
		}
		delete(m.stopPaused, it.ID)
	}
	return n
}

// Stopped reports whether the queue is stopped.
func (m *Manager) Stopped() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

// SaveError returns the error of the most recent failed save, if any.
func (m *Manager) SaveError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveErr
}

// Add queues new downloads at the end of the queue and returns their ids.
// Downloads requested as paused wait for Start or Requeue.
func (m *Manager) Add(reqs ...NewItem) []string {
	m.mu.Lock()
	defer m.notify()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(reqs))
	for _, r := range reqs {
		it := newItem(r, m.cfg)
		m.queue = append(m.queue, it)
		ids = append(ids, it.ID)
	}
	m.scheduleLocked()
	m.saveLocked()
	return ids
}

// newItem builds a queued item for r, or a paused one when r asks for it; a
// missing format or resolution follows cfg.
func newItem(r NewItem, cfg config.Config) *Item {
	it := &Item{
		ID:         newID(),
		URL:        r.URL,
		Title:      r.Title,
		Uploader:   r.Uploader,
		Duration:   r.Duration,
		Extractor:  r.Extractor,
		Format:     r.Format,
		Resolution: r.Resolution,
		Status:     Queued,
		AddedAt:    time.Now(),
		ClipStart:  r.Clip.Start,
		ClipEnd:    r.Clip.End,
		Template:   r.Template,
	}
	if r.Subtitles != nil {
		it.Subtitles = new(*r.Subtitles)
	}
	if r.SponsorBlock != nil {
		it.SponsorBlock = new(*r.SponsorBlock)
	}
	if r.Paused {
		it.Status = Paused
	}
	if !it.Format.Valid() {
		it.Format = cfg.DefaultFormat
	}
	if it.Resolution == "" {
		it.Resolution = cfg.DefaultResolution
	}
	return it
}

// Start starts or resumes a download immediately, even if that exceeds the
// concurrency limit. Failed and cancelled downloads are retried; finished
// ones move back to the queue and are downloaded again, replacing the file.
func (m *Manager) Start(id string) error {
	m.mu.Lock()
	defer m.notify()
	defer m.mu.Unlock()
	it := m.findQueuedLocked(id)
	if it == nil {
		i := slices.IndexFunc(m.history, byID(id))
		if i < 0 {
			return ErrNotFound
		}
		if m.closed {
			return nil
		}
		it = m.history[i]
		m.history = slices.Delete(m.history, i, i+1)
		it.Progress = ytdlp.Progress{}
		m.queue = append(m.queue, it)
	}
	if it.Status == Downloading || m.closed {
		return nil
	}
	delete(m.stopPaused, id)
	m.startLocked(it)
	m.saveLocked()
	return nil
}

// Pause stops a running or queued download, keeping partial files so it can
// be resumed with Start.
func (m *Manager) Pause(id string) error {
	return m.stop(id, errPaused, Paused)
}

// Requeue puts a paused download back in the queue at its position, so it
// starts once a slot is free. Downloads in any other status are left alone.
func (m *Manager) Requeue(id string) error {
	m.mu.Lock()
	defer m.notify()
	defer m.mu.Unlock()
	it := m.findQueuedLocked(id)
	if it == nil {
		if slices.ContainsFunc(m.history, byID(id)) {
			return nil
		}
		return ErrNotFound
	}
	if it.Status != Paused {
		return nil
	}
	it.Status = Queued
	m.scheduleLocked()
	m.saveLocked()
	return nil
}

// Cancel stops a running or queued download and deletes its partial files.
func (m *Manager) Cancel(id string) error {
	return m.stop(id, errCancelled, Cancelled)
}

func (m *Manager) stop(id string, cause error, status Status) error {
	m.mu.Lock()
	it := m.findQueuedLocked(id)
	if it == nil {
		m.mu.Unlock()
		return ErrNotFound
	}
	if status == Cancelled {
		delete(m.stopPaused, id)
	}
	var cleanup []string
	switch it.Status {
	case Downloading:
		// The worker sets the final status once yt-dlp has exited.
		m.active[id](cause)
	case Queued, Paused, Failed:
		it.Status = status
		if status == Cancelled {
			cleanup = it.takePartFiles()
			it.Progress = ytdlp.Progress{}
		}
		m.saveLocked()
	}
	m.mu.Unlock()
	removePartFiles(cleanup)
	m.notify()
	return nil
}

// Remove deletes a download from the queue or history. A running download
// is stopped and its partial files are deleted; finished files are kept.
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	var cleanup []string
	if i := slices.IndexFunc(m.history, byID(id)); i >= 0 {
		m.history = slices.Delete(m.history, i, i+1)
	} else if i := slices.IndexFunc(m.queue, byID(id)); i >= 0 {
		it := m.queue[i]
		m.queue = slices.Delete(m.queue, i, i+1)
		delete(m.stopPaused, id)
		if cancel, ok := m.active[id]; ok {
			cancel(errRemoved)
		} else {
			cleanup = it.takePartFiles()
		}
	} else {
		m.mu.Unlock()
		return ErrNotFound
	}
	m.saveLocked()
	m.mu.Unlock()
	removePartFiles(cleanup)
	m.notify()
	return nil
}

// Move shifts a waiting (not running) download up (delta < 0) or down
// within the queue. Running downloads always stay on top.
func (m *Manager) Move(id string, delta int) error {
	m.mu.Lock()
	defer m.notify()
	defer m.mu.Unlock()
	view := m.displayOrderLocked()
	i := slices.IndexFunc(view, byID(id))
	if i < 0 {
		return ErrNotFound
	}
	j := i + delta
	if j < 0 || j >= len(view) || view[i].Status == Downloading || view[j].Status == Downloading {
		return nil
	}
	// The display order is a stable partition of m.queue, so moving within
	// the waiting part means moving within m.queue.
	a := slices.Index(m.queue, view[i])
	b := slices.Index(m.queue, view[j])
	it := m.queue[a]
	m.queue = slices.Insert(slices.Delete(m.queue, a, a+1), b, it)
	m.scheduleLocked()
	m.saveLocked()
	return nil
}

// ClearHistory removes all finished downloads from the history.
func (m *Manager) ClearHistory() {
	m.mu.Lock()
	m.history = nil
	m.saveLocked()
	m.mu.Unlock()
	m.notify()
}

// Close stops all running downloads, keeping them queued for the next start,
// and saves the state.
func (m *Manager) Close() error {
	m.mu.Lock()
	m.closed = true
	m.dropWaitLocked()
	m.estStop()
	for _, cancel := range m.active {
		cancel(errShutdown)
	}
	m.mu.Unlock()
	m.wg.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveLocked()
	return m.saveErr
}

func (m *Manager) displayOrderLocked() []*Item {
	view := make([]*Item, 0, len(m.queue))
	for _, it := range m.queue {
		if it.Status == Downloading {
			view = append(view, it)
		}
	}
	for _, it := range m.queue {
		if it.Status != Downloading {
			view = append(view, it)
		}
	}
	return view
}

func (m *Manager) findLocked(id string) *Item {
	if it := m.findQueuedLocked(id); it != nil {
		return it
	}
	if i := slices.IndexFunc(m.history, byID(id)); i >= 0 {
		return m.history[i]
	}
	return nil
}

func (m *Manager) findQueuedLocked(id string) *Item {
	if i := slices.IndexFunc(m.queue, byID(id)); i >= 0 {
		return m.queue[i]
	}
	return nil
}

func (m *Manager) scheduleLocked() {
	if m.closed || m.held {
		return
	}
	for _, it := range m.queue {
		if len(m.active) >= m.cfg.Concurrency || m.waiting {
			break
		}
		if it.Status == Queued {
			m.startLocked(it)
		}
	}
	m.scheduleEstimatesLocked()
}

// scheduleEstimatesLocked starts size estimates for queued, never-started
// items without one, in queue order. Each item is tried once per session.
func (m *Manager) scheduleEstimatesLocked() {
	if m.closed {
		return
	}
	for _, it := range m.queue {
		if m.estimating >= maxEstimates {
			return
		}
		if it.Status != Queued || !it.StartedAt.IsZero() || it.EstimatedSize > 0 || m.estTried[it.ID] {
			continue
		}
		m.estTried[it.ID] = true
		m.estimating++
		m.wg.Add(1)
		go m.estimate(it.ID, it.URL, it.Format, it.Resolution, m.cfg.CookiesFromBrowser)
	}
}

func (m *Manager) estimate(id, url string, f ytdlp.Format, resolution, cookies string) {
	defer m.wg.Done()
	est, err := m.runner.Estimate(m.estCtx, url, f, resolution, cookies)
	m.mu.Lock()
	m.estimating--
	// A removed item's result is dropped, as is one for an item whose
	// download started meanwhile: its progress reports the real size.
	if it := m.findQueuedLocked(id); it != nil && !m.closed && it.StartedAt.IsZero() {
		applyEstimate(it, est, err)
		m.saveLocked()
	}
	m.scheduleEstimatesLocked()
	m.mu.Unlock()
	m.notify()
}

// applyEstimate sets the item's estimated size and fills its empty metadata. A
// failure, or an estimate without a size, goes to the item's log.
func applyEstimate(it *Item, est *ytdlp.Estimate, err error) {
	if err != nil {
		it.appendLog("vidl: size estimate failed: " + err.Error())
		return
	}
	if it.Title == "" {
		it.Title = est.Title
	}
	if it.Uploader == "" {
		it.Uploader = est.Uploader
	}
	if it.Extractor == "" {
		it.Extractor = est.Extractor
	}
	if it.Duration == 0 {
		it.Duration = est.Duration
	}
	if est.Size <= 0 {
		it.appendLog("vidl: size estimate failed: yt-dlp reported no size")
		return
	}
	if it.EstimatedSize = clipShare(est.Size, it.Clip(), it.Duration); it.EstimatedSize == 0 {
		it.appendLog("vidl: size estimate failed: unknown duration, can't scale to the clip")
	}
}

// clipShare scales size, the size of the whole video, to clip; 0 when the
// clip's share of an unknown duration can't be known.
func clipShare(size int64, clip ytdlp.Clip, duration float64) int64 {
	if clip.IsZero() {
		return size
	}
	end := clip.End.Seconds()
	if end == 0 || end > duration {
		end = duration
	}
	length := end - clip.Start.Seconds()
	if duration <= 0 || length <= 0 {
		return 0
	}
	return int64(float64(size) * length / duration)
}

func (m *Manager) startLocked(it *Item) {
	ctx, cancel := context.WithCancelCause(context.Background())
	m.active[it.ID] = cancel
	it.Status = Downloading
	it.Error = ""
	it.Log = nil
	it.StartedAt = time.Now()
	job := m.jobLocked(it)
	m.wg.Add(1)
	go m.run(ctx, cancel, it, job)
}

func (m *Manager) jobLocked(it *Item) ytdlp.Job {
	c := m.cfg
	template := c.FilenameTemplate
	if it.Template != "" {
		template = it.Template
	}
	subtitles := c.EmbedSubtitles
	if it.Subtitles != nil {
		subtitles = *it.Subtitles
	}
	sponsorBlock := c.SponsorBlockRemove
	if it.SponsorBlock != nil && !*it.SponsorBlock {
		sponsorBlock = ""
	}
	return ytdlp.Job{
		URL:                it.URL,
		Dir:                c.DownloadPath(),
		Template:           template,
		Format:             it.Format,
		Resolution:         it.Resolution,
		EmbedSubtitles:     subtitles,
		SubtitleLangs:      c.SubtitleLangs,
		EmbedThumbnail:     c.EmbedThumbnail,
		EmbedMetadata:      c.EmbedMetadata,
		RateLimit:          c.RateLimit,
		CookiesFromBrowser: c.CookiesFromBrowser,
		Clip:               it.Clip(),
		ForceOverwrites:    it.redownload(),
		SponsorBlockRemove: sponsorBlock,
	}
}

func (m *Manager) run(ctx context.Context, cancel context.CancelCauseFunc, it *Item, job ytdlp.Job) {
	defer m.wg.Done()
	err := m.runner.Run(ctx, job, func(ev ytdlp.Event) {
		m.mu.Lock()
		applyEvent(it, ev)
		m.mu.Unlock()
		m.notify()
	})
	cause := context.Cause(ctx)
	cancel(nil)

	// No events arrive once Run has returned, so FilePath is stable here and
	// the stat can stay outside the lock.
	var size int64
	var statErr error
	if err == nil && it.FilePath != "" {
		var fi os.FileInfo
		if fi, statErr = os.Stat(it.FilePath); statErr == nil {
			size = fi.Size()
		}
	}

	m.mu.Lock()
	delete(m.active, it.ID)
	// Pauses by Stop suspend rather than end a download: Resume continues it.
	if cause != errShutdown && !m.stopPaused[it.ID] {
		m.armWaitLocked()
	}
	var cleanup []string
	it.Progress.Speed, it.Progress.ETA = 0, 0
	switch {
	case cause == errRemoved:
		// Part files of a single-stream download include its final path, so
		// keep them if the download completed before the stop took effect.
		if err != nil {
			cleanup = it.takePartFiles()
		}
	case err == nil:
		// Also covers a stop request that raced with a completed download.
		it.Status = Finished
		it.FinishedAt = time.Now()
		it.Progress.Percent = 100
		it.Size = size
		if statErr != nil {
			it.appendLog("vidl: " + statErr.Error())
		}
		it.PartFiles = nil
		if i := slices.Index(m.queue, it); i >= 0 {
			m.queue = slices.Delete(m.queue, i, i+1)
		}
		m.history = slices.Insert(m.history, 0, it)
	case cause == errPaused:
		it.Status = Paused
		if m.stopPaused[it.ID] && !m.stopped { // resumed while pausing
			it.Status = Queued
		}
	case cause == errShutdown:
		it.Status = Queued
	case cause == errCancelled:
		it.Status = Cancelled
		it.Progress = ytdlp.Progress{}
		cleanup = it.takePartFiles()
	default:
		it.Status = Failed
		it.Error = err.Error()
	}
	if it.Status != Paused {
		delete(m.stopPaused, it.ID)
	}
	m.scheduleLocked()
	m.saveLocked()
	m.mu.Unlock()
	removePartFiles(cleanup)
	m.notify()
}

func applyEvent(it *Item, ev ytdlp.Event) {
	switch ev.Kind {
	case ytdlp.EventLog:
		it.appendLog(ev.Line)
	case ytdlp.EventInfo:
		if ev.Info.Title != "" {
			it.Title = ev.Info.Title
		}
		if ev.Info.Uploader != "" {
			it.Uploader = ev.Info.Uploader
		}
		if ev.Info.Extractor != "" {
			it.Extractor = ev.Info.Extractor
		}
		if ev.Info.Duration > 0 {
			it.Duration = ev.Info.Duration
		}
	case ytdlp.EventProgress, ytdlp.EventPostprocess:
		it.Progress = ev.Progress
		it.addPartFiles(ev.Files)
	case ytdlp.EventFile:
		it.FilePath = ev.Path
	}
}

func (m *Manager) saveLocked() {
	st := State{}
	for _, it := range m.queue {
		st.Queue = append(st.Queue, it.saved())
	}
	for _, it := range m.history {
		st.History = append(st.History, it.saved())
	}
	m.saveErr = m.store.Save(st)
}

// removePartFiles deletes partial download files, including yt-dlp's
// fragment and resume bookkeeping files.
func removePartFiles(files []string) {
	for _, f := range files {
		os.Remove(f)
		os.Remove(f + ".ytdl")
		frags, _ := filepath.Glob(globEscape(f) + "-Frag*")
		for _, frag := range frags {
			os.Remove(frag)
		}
	}
}

// globEscape escapes glob metacharacters, which are common in video titles.
func globEscape(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '*', '?', '[', ']', '\\':
			b = append(b, '\\')
		}
		b = append(b, s[i])
	}
	return string(b)
}

func byID(id string) func(*Item) bool {
	return func(it *Item) bool { return it.ID == id }
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
