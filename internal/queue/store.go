package queue

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

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

// MemStore is an in-memory Store, useful for tests.
type MemStore struct {
	State State
}

func (s *MemStore) Load() (State, error) { return s.State, nil }
func (s *MemStore) Save(st State) error  { s.State = st; return nil }
