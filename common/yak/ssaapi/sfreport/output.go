package sfreport

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

// reportFlushEvery is how many findings the SARIF report collects before it
// rewrites its snapshot. The IRify report decides from its own change flag
// instead: it writes a whole document, so it never measures by events.
const reportFlushEvery = 500

// reportSnapshotFunc writes one complete report document to a destination.
//
// A document is never assembled in memory: the writer receives it in pieces.
// That is what keeps a 408MB snapshot from costing 408MB of resident memory
// (plus the marshal's own scratch space) on every checkpoint.
type reportSnapshotFunc func(io.Writer) error

// writeReportSnapshotBytes publishes an already encoded report document.
func writeReportSnapshotBytes(w io.Writer, data []byte) error {
	return writeReportSnapshot(w, func(out io.Writer) error {
		_, err := out.Write(data)
		return err
	})
}

// writeReportSnapshot publishes one complete report document.
//
// A file destination is replaced atomically: the snapshot goes to a temporary
// file in the same directory, is synced, and then renamed over the target, so
// a reader never sees a half-written document and a failed snapshot leaves the
// previous one intact. Other destinations are rewound and overwritten (see
// rewindReportOutput).
func writeReportSnapshot(w io.Writer, produce reportSnapshotFunc) error {
	if w == nil {
		return nil
	}
	if named, ok := w.(interface{ Name() string }); ok {
		if path := named.Name(); path != "" && path != "/dev/stdout" && path != "stdout" {
			if _, err := os.Stat(filepath.Dir(path)); err == nil {
				return writeFileSnapshot(path, produce)
			}
		}
	}
	if err := rewindReportOutput(w); err != nil {
		return err
	}
	if err := produce(w); err != nil {
		// A half-written document must never be published: drop it so the
		// destination keeps whatever complete snapshot it had before.
		if resetErr := rewindReportOutput(w); resetErr != nil {
			return utils.Wrapf(err, "reset partial report snapshot: %v", resetErr)
		}
		return err
	}
	return nil
}

// writeFileSnapshot streams the document next to path and renames it in place.
//
// The stream is synced before the rename, and the temporary file is removed on
// any failure, so path always holds a complete document.
func writeFileSnapshot(path string, produce reportSnapshotFunc) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return utils.Wrapf(err, "create report snapshot failed")
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := produce(tmp); err != nil {
		tmp.Close()
		return utils.Wrapf(err, "write report snapshot failed")
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return utils.Wrapf(err, "sync report snapshot failed")
	}
	if err := tmp.Close(); err != nil {
		return utils.Wrapf(err, "close report snapshot failed")
	}
	if err := os.Rename(tmpName, path); err != nil {
		return utils.Wrapf(err, "publish report snapshot failed")
	}
	return nil
}

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

// reportFlushMinInterval bounds how often an intermediate snapshot may be
// published. A snapshot is the whole document (408MB on javacms-core), and
// MarshalIndent allocates about 3.5x its size, so publishing one every 30s
// still cost 55GB of allocation and 15GB of writes on a 19-minute scan. Stage
// boundaries and the final Save() always publish a complete document, so a
// two-minute interval keeps the crash-resilience guarantee at a fraction of
// the cost. Overridable in tests; 0 disables the throttle.
var reportFlushMinIntervalValue = 2 * time.Minute

func reportFlushMinInterval() time.Duration { return reportFlushMinIntervalValue }
