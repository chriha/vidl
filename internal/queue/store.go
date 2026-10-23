package queue

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/chriha/vidl/internal/config"
)

// LockName is the instance lock's file name, next to the queue file.
const LockName = "queue.lock"

// ErrLocked means another vidl holds the queue lock.
var ErrLocked = errors.New("queue is locked by a running vidl")

// State is everything the manager persists.
type State struct {
	Queue   []Item `json:"queue"`
	History []Item `json:"history"`
}

type Store interface {
	Load() (State, error)
	Save(State) error
}

// FileStore keeps the state as JSON in a single file.
type FileStore struct {
	Path string
}

func (s FileStore) Load() (State, error) {
	var st State
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	return st, json.Unmarshal(data, &st)
}

func (s FileStore) Save(st State) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".queue-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.Path)
}

// Lock is the exclusive instance lock on a queue file. The vidl that owns
// the file holds it for its whole lifetime.
type Lock struct {
	f    *os.File
	path string
}

// Lock takes the instance lock next to s.Path without waiting. It fails with
// ErrLocked while another vidl holds it; call Release when done.
func (s FileStore) Lock() (*Lock, error) {
	path := s.LockPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return acquireLock(path)
}

// LockPath is the instance lock's path, next to s.Path.
func (s FileStore) LockPath() string {
	return filepath.Join(filepath.Dir(s.Path), LockName)
}

// Release gives up the lock.
func (l *Lock) Release() error {
	return l.release()
}

// AppendQueued adds queued downloads to the file while no vidl is running:
// it takes the lock, loads the file, appends and saves, keeping existing
// items as they are. A missing format or resolution follows cfg. It fails
// with ErrLocked while a vidl holds the lock.
func (s FileStore) AppendQueued(cfg config.Config, reqs ...NewItem) (ids []string, err error) {
	l, err := s.Lock()
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, l.Release()) }()
	st, err := s.Load()
	if err != nil {
		return nil, err
	}
	cfg = cfg.Normalize()
	for _, r := range reqs {
		it := newItem(r, cfg)
		st.Queue = append(st.Queue, *it)
		ids = append(ids, it.ID)
	}
	if err := s.Save(st); err != nil {
		return nil, err
	}
	return ids, nil
}

// MemStore is an in-memory Store, useful for tests.
type MemStore struct {
	State State
}

func (s *MemStore) Load() (State, error) { return s.State, nil }
func (s *MemStore) Save(st State) error  { s.State = st; return nil }
