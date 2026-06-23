package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// FileState records how much of a file has already been ingested so a re-scan
// only reads new content. The same four fields carry different watermark kinds
// depending on the adapter — always read/write them through the typed accessors
// on State (ByteOffset/CommitBytes, Rowid/CommitRowid, MaxUnixMillis/RowCount,
// FileUnchanged/CommitFile) so each adapter's intent is explicit:
//
//   - byte-offset adapters: Offset = bytes consumed, Lines = records consumed.
//   - max-rowid adapters:   Offset = highest SQLite rowid emitted.
//   - watermark adapters:   Lines = a monotonic watermark (max unix-millis or a
//     data-row count), Offset unused.
//   - whole-file adapters:  Size+ModTimeNano are the change check; Offset=Size
//     and Lines=record count are bookkeeping.
//
// Size/ModTimeNano always record the file's size/mtime at commit time.
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

// commit records the new consumed position for a file after a scan. It is the
// low-level setter behind the typed Commit* accessors; adapters should prefer
// those so each watermark's kind is explicit.
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

// Watermark accessors
//
// FileState carries four physical fields (Size, ModTimeNano, Offset, Lines) but
// adapters use them in four distinct ways. The accessors below name each usage
// so an adapter's intent — and the correct resume/commit semantics for its kind
// — is explicit at the call site. The on-disk JSON shape is unchanged; only the
// in-code meaning is made self-documenting.

// --- Byte-offset adapters (Claude, Codex, Copilot, OpenClaw) ---
//
// Offset is the byte position consumed and Lines the number of complete records
// consumed. A re-scan resumes from Offset; if the file shrank below Offset it
// was truncated/rewritten, so ingestion restarts from zero.

// ByteOffset returns the byte offset and line count to skip for a byte-offset
// file of the given current size. A file that shrank below the consumed offset
// is treated as truncated and restarts from zero.
func (s *State) ByteOffset(path string, size int64) (offset int64, lines int) {
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

// CommitBytes records the new consumed byte offset and line count for a
// byte-offset file.
func (s *State) CommitBytes(path string, size, mtimeNano, offset int64, lines int) {
	s.commit(path, size, mtimeNano, offset, lines)
}

// --- Max-rowid adapters (Goose) ---
//
// Offset holds the highest SQLite rowid already emitted; a re-scan reads rows
// with a strictly greater rowid. Size/mtime are recorded for bookkeeping only
// (rowids are monotonic, so there is no truncation check).

// Rowid returns the highest rowid already emitted for a DB, or 0 if unseen.
func (s *State) Rowid(path string) int64 {
	fs, ok := s.get(path)
	if !ok {
		return 0
	}
	return fs.Offset
}

// CommitRowid records the highest rowid emitted (and the row count read) for a
// max-rowid DB.
func (s *State) CommitRowid(path string, size, mtimeNano, maxRowID int64, lines int) {
	s.commit(path, size, mtimeNano, maxRowID, lines)
}

// --- Max-unix-millis / row-count watermark adapters (OpenCode, Cursor) ---
//
// These stash a monotonic watermark (a max time_created in millis for OpenCode,
// or a CSV data-row count for Cursor) in Lines, with Offset unused (0). A
// re-scan reads only rows past the watermark; if the file shrank below the
// recorded size it is treated as reset and the watermark drops to 0.

// MaxUnixMillis returns the recorded millisecond watermark for a file, or 0 if
// unseen or the file shrank below its recorded size (a reset).
func (s *State) MaxUnixMillis(path string, size int64) int64 {
	fs, ok := s.get(path)
	if !ok || size < fs.Size {
		return 0
	}
	return int64(fs.Lines)
}

// CommitUnixMillis records a millisecond watermark for a file (Offset unused).
func (s *State) CommitUnixMillis(path string, size, mtimeNano, ms int64) {
	s.commit(path, size, mtimeNano, 0, int(ms))
}

// RowCount returns the recorded data-row watermark for a file, or 0 if unseen or
// the file shrank below its recorded size (a fresh/rewritten file).
func (s *State) RowCount(path string, size int64) int {
	fs, ok := s.get(path)
	if !ok || size < fs.Size {
		return 0
	}
	return fs.Lines
}

// CommitRowCount records a data-row watermark for a file (Offset unused).
func (s *State) CommitRowCount(path string, size, mtimeNano int64, rows int) {
	s.commit(path, size, mtimeNano, 0, rows)
}

// --- Whole-file adapters (Zed, Gemini) ---
//
// These rewrite the whole file in place, so there is no byte offset to resume
// from. The recorded Size+ModTimeNano detect change: an unchanged file is
// skipped entirely and a changed one is fully re-parsed (eventId dedup absorbs
// the overlap). Offset is set to Size and Lines to the record count purely as
// bookkeeping.

// FileUnchanged reports whether a whole-file adapter can skip a file because its
// size and mtime match the recorded watermark.
func (s *State) FileUnchanged(path string, size, mtimeNano int64) bool {
	fs, ok := s.get(path)
	return ok && fs.Size == size && fs.ModTimeNano == mtimeNano
}

// CommitFile records the size/mtime watermark for a whole-file adapter, along
// with the record count read (bookkeeping only).
func (s *State) CommitFile(path string, size, mtimeNano int64, records int) {
	s.commit(path, size, mtimeNano, size, records)
}
