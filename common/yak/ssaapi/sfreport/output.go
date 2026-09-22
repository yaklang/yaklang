package sfreport

import (
	"bytes"
	"io"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// rewindReportOutput makes a report writer ready to receive the whole document
// from the beginning, so a save replaces the previous snapshot instead of
// appending to it.
//
// A scan saves its report at every stage boundary and once more when it ends
// (see ScanProject). Each save contains every finding collected so far, so the
// writer has to be rewound: appending produced several concatenated JSON
// documents, which no JSON parser and therefore no SARIF upload accepts.
func rewindReportOutput(w io.Writer) error {
	if w == nil {
		return nil
	}
	switch out := w.(type) {
	case interface {
		Truncate(size int64) error
		Seek(offset int64, whence int) (int64, error)
	}:
		// Files opened by the CLI.
		if err := out.Truncate(0); err != nil {
			return utils.Wrapf(err, "truncate report output failed")
		}
		if _, err := out.Seek(0, io.SeekStart); err != nil {
			return utils.Wrapf(err, "rewind report output failed")
		}
	case interface{ Reset() }:
		// In-memory sinks such as bytes.Buffer or SnapshotBuffer.
		out.Reset()
	}
	// A forward-only stream cannot be rewound. SnapshotBuffer is what the CLI
	// wraps stdout in so this case does not corrupt the output.
	return nil
}

// SnapshotBuffer adapts a forward-only destination (stdout, a pipe) to the
// save-replaces-output contract of a report.
//
// Snapshots accumulate in memory and only the latest one reaches the
// underlying writer, on Flush. Intermediate saves therefore cost a marshal and
// nothing else, while the stream still receives exactly one document.
type SnapshotBuffer struct {
	mu  sync.Mutex
	dst io.Writer
	buf bytes.Buffer
}

func NewSnapshotBuffer(dst io.Writer) *SnapshotBuffer {
	return &SnapshotBuffer{dst: dst}
}

// Write accepts a snapshot; it is buffered until Flush.
func (b *SnapshotBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// Reset discards the previous snapshot, which is what makes a later save
// replace it.
func (b *SnapshotBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

// Flush publishes the latest snapshot to the destination. A report with no
// snapshot yet writes nothing.
func (b *SnapshotBuffer) Flush() error {
	if b == nil || b.dst == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.buf.Len() == 0 {
		return nil
	}
	data := b.buf.Bytes()
	// The report body has no trailing newline; add one so whatever the process
	// logs next does not end up glued to the closing brace.
	if !bytes.HasSuffix(data, []byte("\n")) {
		data = append(append([]byte{}, data...), '\n')
	}
	_, err := b.dst.Write(data)
	return err
}
