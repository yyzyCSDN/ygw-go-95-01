package record

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

var (
	ErrClosed  = errors.New("recorder is closed")
	ErrNoEntry = errors.New("empty record entry")
)

type Recorder struct {
	dir       string
	mu        sync.Mutex
	store     *Store
	closed    bool
	openFiles atomic.Int64
}

func NewRecorder(dir string, store *Store) (*Recorder, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create record dir: %w", err)
	}
	rec := &Recorder{dir: dir, store: store}
	if err := store.Resume(rec.path()); err != nil {
		return nil, fmt.Errorf("resume records: %w", err)
	}
	return rec, nil
}

func (r *Recorder) path() string {
	return filepath.Join(r.dir, "run.log")
}

func (r *Recorder) Append(e Entry) (string, error) {
	if e.Kind == "" && e.Vessel == "" && e.Message == "" {
		return "", ErrNoEntry
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return "", ErrClosed
	}
	fp := Fingerprint(e)
	if r.store.Seen(fp) {
		return fp, nil
	}
	f, err := r.openFile(r.path())
	if err != nil {
		return "", err
	}
	defer r.closeFile(f)
	line, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	line = append(line, '\n')
	if _, err := f.Write(line); err != nil {
		return "", fmt.Errorf("write record: %w", err)
	}
	if err := f.Sync(); err != nil {
		return "", fmt.Errorf("sync record: %w", err)
	}
	r.store.Record(fp, e)
	return fp, nil
}

func (r *Recorder) openFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open record file: %w", err)
	}
	r.openFiles.Add(1)
	return f, nil
}

func (r *Recorder) closeFile(f *os.File) {
	_ = f.Close()
	r.openFiles.Add(-1)
}

func (r *Recorder) OpenHandles() int64 {
	return r.openFiles.Load()
}

func (r *Recorder) Lease() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrClosed
	}
	r.openFiles.Add(1)
	return nil
}

func (r *Recorder) Release() {
	r.openFiles.Add(-1)
}

func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrClosed
	}
	r.closed = true
	return nil
}

func (r *Recorder) Flush() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrClosed
	}
	return nil
}

func (r *Recorder) Store() *Store {
	return r.store
}
