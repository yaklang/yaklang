package reactloops

import (
	"io"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"
)

// Observe the provider streams before the transaction waits for completion.
// The existing consumer owns the reader; all bytes still reach it unchanged.
func observeActivityStreams(cfg *aispec.AIConfig, current func() *responseActivity, text bool) {
	if previous := cfg.ReasonStreamHandler; previous != nil {
		cfg.ReasonStreamHandler = func(reader io.Reader) {
			previous(pumpActivityStream(reader, current, true, !text))
		}
	}
	if previous := cfg.StreamHandler; previous != nil {
		cfg.StreamHandler = func(reader io.Reader) {
			previous(pumpActivityStream(reader, current, false, !text))
		}
	}
}

// AIResponse consumes channel readers serially. Drain the provider channels
// independently so an open Reason reader cannot hold back Content or its UI.
// The canonical response still receives every original byte in channel order.
func pumpActivityStream(reader io.Reader, current func() *responseActivity, reason, native bool) io.Reader {
	input, output := utils.NewBufPipe(nil)
	observed := &activityProviderReader{reader: reader, current: current, beforeHeader: current(), reason: reason, native: native}
	go func() {
		_, err := io.Copy(output, observed)
		_ = output.CloseWithError(err)
	}()
	return input
}

// Providers register readers before HTTP headers arrive. Bind on the first
// bytes, after the header callback has selected the concrete response, rather
// than retaining the already-retired pre-request activity.
type activityProviderReader struct {
	reader         io.Reader
	current        func() *responseActivity
	activity       *responseActivity
	beforeHeader   *responseActivity
	retired        bool
	textWriter     io.WriteCloser
	remaining      int
	reason, native bool
}

func (r *activityProviderReader) Read(data []byte) (int, error) {
	n, err := r.reader.Read(data)
	if n > 0 {
		if r.retired {
			return n, err
		}
		if r.activity == nil {
			r.activity = r.current()
			if r.beforeHeader != r.activity {
				r.beforeHeader.mu.Lock()
				next := r.beforeHeader.headerSuccessor
				r.beforeHeader.mu.Unlock()
				if next != r.activity {
					r.retired = true
					return n, err
				}
			}
			if !r.native && !r.reason {
				r.textWriter = r.activity.newTextMirror()
				r.remaining = 4 << 20
			}
		}
		if r.textWriter != nil && r.remaining > 0 {
			observed := min(n, r.remaining)
			_, _ = r.textWriter.Write(data[:observed])
			r.remaining -= observed
		}
		if r.reason {
			r.activity.reason(data[:n])
		} else if r.native {
			if strings.TrimSpace(string(data[:n])) != "" {
				r.activity.content()
			}
		} else {
			r.activity.envelope()
		}
		if r.native {
			r.activity.general.write(data[:n], r.reason)
		}
	}
	if (err != nil || r.remaining == 0) && r.textWriter != nil {
		_ = r.textWriter.Close()
	}
	return n, err
}

// The provider still feeds the canonical response/checkpoint consumers. This
// separate buffered display bridge delivers Reason and Markdown before the
// Config wrapper waits for callback completion, without replaying them later.
type liveGeneralOutput struct {
	mu             sync.Mutex
	callback       LoopGeneralOutputCallback
	output, reason io.WriteCloser
	done           chan struct{}
	err            error
	closed         bool
}

func (g *liveGeneralOutput) write(data []byte, reason bool) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return
	}
	if g.done == nil {
		outputReader, outputWriter := utils.NewBufPipe(nil)
		reasonReader, reasonWriter := utils.NewBufPipe(nil)
		g.output, g.reason, g.done = outputWriter, reasonWriter, make(chan struct{})
		go func() {
			err := invokeLoopGeneralOutputCallback(g.callback, outputReader, reasonReader)
			// A callback may consume only one channel or return immediately.
			var drains sync.WaitGroup
			drains.Add(2)
			go func() { defer drains.Done(); _, _ = io.Copy(io.Discard, outputReader) }()
			go func() { defer drains.Done(); _, _ = io.Copy(io.Discard, reasonReader) }()
			drains.Wait()
			g.mu.Lock()
			g.err = err
			g.mu.Unlock()
			close(g.done)
		}()
	}
	writer := g.output
	if reason {
		writer = g.reason
	}
	_, _ = writer.Write(data)
}

func (g *liveGeneralOutput) close() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closed = true
	if g.output != nil {
		_ = g.output.Close()
		_ = g.reason.Close()
	}
}

func (g *liveGeneralOutput) finish() (bool, error) {
	if g == nil {
		return false, nil
	}
	g.close()
	g.mu.Lock()
	done := g.done
	g.mu.Unlock()
	if done == nil {
		return false, nil
	}
	<-done
	g.mu.Lock()
	defer g.mu.Unlock()
	return true, g.err
}

func (g *liveGeneralOutput) started() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.done != nil
}

// This parser observes UI metadata only and never admits or executes an action.
// Buffering isolates the provider from UI/parser latency. A bounded mirror and
// explicit retirement avoid retaining abandoned streams on retries/cancel.
func (a *responseActivity) newTextMirror() io.WriteCloser {
	input, output := utils.NewBufPipe(nil)
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		_ = output.Close()
		return nil
	}
	a.streamClosers = append(a.streamClosers, output)
	a.mu.Unlock()
	go func() {
		options := []jsonextractor.CallbackOption{jsonextractor.WithFormatKeyValueCallback(func(key, value any, parents []string) {
			if key, ok := key.(string); ok {
				a.field("text", key, value, parents)
			}
		})}
		for _, field := range []string{"human_readable_thought", "cumulative_summary", "answer_payload", "answer", "summary"} {
			field := field
			options = append(options, jsonextractor.WithRegisterFieldStreamHandler(field, func(_ string, reader io.Reader, parents []string) {
				if len(parents) != 0 {
					_, _ = io.Copy(io.Discard, reader)
					return
				}
				observed := &displayFieldReader{Reader: utils.JSONStringReader(reader), onData: func() { a.displayField("text", field) }}
				_, _ = io.Copy(io.Discard, observed)
			}))
		}
		_ = jsonextractor.ExtractStructuredJSONFromStream(input, options...)
		_, _ = io.Copy(io.Discard, input)
	}()
	return output
}

func (a *responseActivity) hasReason() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.lastReason.IsZero()
}
