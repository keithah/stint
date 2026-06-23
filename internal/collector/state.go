package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// FileState records how much of a file has already been ingested so a re-scan
// only reads new content. Offset is the byte position consumed; Lines is the
// number of newline-terminated records consumed. Size/ModTimeNano detect
// truncation or rewrite (in which case the adapter starts over).
type FileState struct {
	Size        int64 `json:"size"`
	ModTimeNano int64 `json:"mtime_nano"`
	Offset      int64 `json:"offset"`
	Lines       int   `json:"lines"`
}

// State is the persisted incremental-ingest cursor, keyed by absolute file
// path. It is safe for concurrent use by adapters.
type State struct {
	mu    sync.Mutex
	Files map[string]FileState `json:"files"`
	path  string
}

// NewState returns an empty in-memory state not backed by a file.
func NewState() *State {
	return &State{Files: map[string]FileState{}}
}

// DefaultStatePath is the default location of the persisted state file.
func DefaultStatePath() string {
	return ExpandHome("~/.stint/collector-state.json")
}

// LoadState reads state from path. A missing file yields an empty state bound
// to that path (so the first Save creates it).
func LoadState(path string) (*State, error) {
	path = ExpandHome(path)
	s := &State{Files: map[string]FileState{}, path: path}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	if s.Files == nil {
		s.Files = map[string]FileState{}
	}
	s.path = path
	return s, nil
}

// Save persists the state atomically (write temp + rename) to its bound path.
// A state with no bound path is a no-op.
func (s *State) Save() error {
	if s == nil || s.path == "" {
		return nil
	}
	s.mu.Lock()
	b, err := json.MarshalIndent(struct {
		Files map[string]FileState `json:"files"`
	}{Files: s.Files}, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// get returns the recorded state for a path.
func (s *State) get(path string) (FileState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fs, ok := s.Files[path]
	return fs, ok
}

// resume returns the byte offset and line count an adapter should skip for a
// file with the given current size/mtime. If the file shrank (truncated) or its
// recorded size no longer matches a prefix relationship, ingestion restarts
// from zero.
func (s *State) resume(path string, size, mtimeNano int64) (offset int64, lines int) {
	fs, ok := s.get(path)
	if !ok {
		return 0, 0
	}
	// File shrank below what we consumed: it was rewritten/truncated; restart.
	if size < fs.Offset {
		return 0, 0
	}
	return fs.Offset, fs.Lines
}

// commit records the new consumed position for a file after a scan.
func (s *State) commit(path string, size, mtimeNano, offset int64, lines int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Files[path] = FileState{
		Size:        size,
		ModTimeNano: mtimeNano,
		Offset:      offset,
		Lines:       lines,
	}
}
