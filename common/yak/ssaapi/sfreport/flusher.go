package sfreport

import (
	"io"
	"sync"
)

// reportFlusher serializes report documents in the background.
//
// Snapshots are complete documents, so a newer one supersedes an older one
// that has not been written yet; the channel keeps only the latest. Save()
// stops the worker and writes the final document itself, which is why an
// explicit save always leaves a complete file behind.
type reportFlusher struct {
	writer io.Writer

	mu      sync.Mutex
	ch      chan []byte
	stop    chan struct{}
	wg      sync.WaitGroup
	started bool
}

func newReportFlusher(writer io.Writer) *reportFlusher {
	return &reportFlusher{writer: writer}
}

// submit hands a snapshot to the worker, replacing a pending one.
func (f *reportFlusher) submit(data []byte) {
	if f == nil || f.writer == nil || len(data) == 0 {
		return
	}
	f.mu.Lock()
	if !f.started {
		f.ch = make(chan []byte, 1)
		f.stop = make(chan struct{})
		f.started = true
		f.wg.Add(1)
		go f.worker()
	}
	select {
	case f.ch <- data:
	default:
		// A snapshot is already waiting; the newest document wins.
		select {
		case <-f.ch:
		default:
		}
		select {
		case f.ch <- data:
		default:
		}
	}
	f.mu.Unlock()
}

func (f *reportFlusher) worker() {
	defer f.wg.Done()
	for {
		select {
		case <-f.stop:
			// Write whatever is still queued before leaving, otherwise a
			// stop racing a submit would silently drop the newest snapshot.
			for {
				select {
				case data := <-f.ch:
					if err := writeReportSnapshot(f.writer, data); err != nil {
						log.Errorf("write report snapshot failed: %v", err)
					}
				default:
					return
				}
			}
		case data := <-f.ch:
			if err := writeReportSnapshot(f.writer, data); err != nil {
				log.Errorf("write report snapshot failed: %v", err)
			}
		}
	}
}

// stopAndWait lets a synchronous save take over the writer.
func (f *reportFlusher) stopAndWait() {
	if f == nil {
		return
	}
	f.mu.Lock()
	if !f.started {
		f.mu.Unlock()
		return
	}
	close(f.stop)
	f.started = false
	f.mu.Unlock()
	f.wg.Wait()
}
