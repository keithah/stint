package collector

import (
	"bufio"
	"errors"
	"io"
	"os"
)

const maxJSONLLineBytes = 4 * 1024 * 1024

// scanJSONLIncremental reads the unconsumed tail of a newline-delimited file and
// invokes perLine for each complete (newline-terminated) line. It owns the
// resume/offset/partial-line/commit bookkeeping shared by every byte-offset
// JSONL adapter:
//
//   - resume to the recorded byte offset (falling back to 0 if the Seek fails),
//   - read line by line, treating only newline-terminated lines as complete so a
//     trailing partial line (file still being written) is left for the next scan,
//   - advance the consumed byte offset and line counter per complete line,
//   - commit the new watermark to state when finished.
//
// perLine receives the raw line bytes (including the trailing newline) and the
// 1-based line number within the file. It is only called for complete lines.
// Stat/Open failures are reported via report.Errors and the function returns
// without calling perLine.
func scanJSONLIncremental(path string, state *State, report *ScanReport, perLine func(line []byte, lineNo int)) {
	info, err := os.Stat(path)
	if err != nil {
		report.Errors++
		return
	}
	f, err := os.Open(path)
	if err != nil {
		report.Errors++
		return
	}
	defer f.Close()

	size := info.Size()
	mtime := info.ModTime().UnixNano()
	offset, lineNo := state.ByteOffset(path, size)
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			offset, lineNo = 0, 0
			f.Seek(0, io.SeekStart)
		}
	}

	reader := bufio.NewReaderSize(f, 64*1024)
	consumed := offset
	pendingBytes := int64(0)
	lineTooLong := false
	line := make([]byte, 0, 16*1024)

	for {
		fragment, err := reader.ReadSlice('\n')
		// Only treat a line as complete when terminated by '\n'; a trailing
		// partial line (no newline, file still being written) is left for the
		// next scan and not committed to the offset.
		if len(fragment) > 0 {
			pendingBytes += int64(len(fragment))
			if pendingBytes > maxJSONLLineBytes {
				if !lineTooLong {
					report.Errors++
				}
				lineTooLong = true
			}
			if !lineTooLong {
				line = append(line, fragment...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if len(fragment) > 0 && err == nil {
			consumed += pendingBytes
			lineNo++
			if !lineTooLong {
				perLine(line, lineNo)
			}
			pendingBytes = 0
			lineTooLong = false
			line = line[:0]
		}
		if err != nil {
			break
		}
	}

	state.CommitBytes(path, size, mtime, consumed, lineNo)
}
