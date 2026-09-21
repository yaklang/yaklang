package textdecode

import (
	"bufio"
	"context"
	"errors"
	"io"

	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

// Lines has ScanLines semantics with explicit reservation before reader,
// growing line backing, and token string allocations. It never executes input.
type Lines struct {
	ctx     context.Context
	state   *budget.State
	reader  *bufio.Reader
	buffer  []byte
	text    string
	err     error
	done    bool
	max     int
	total   int64
	records bool
}

func NewLines(ctx context.Context, r io.Reader) *Lines {
	ctx = budget.Ensure(ctx)
	s := &Lines{ctx: ctx, state: budget.From(ctx)}
	s.max = s.state.Limits.MaxFieldBytes
	if s.err = ctx.Err(); s.err != nil {
		return s
	}
	size := max(16, min(4096, s.max))
	if s.err = s.state.Working(int64(size) + budget.SizeObject); s.err != nil {
		return s
	}
	s.reader = bufio.NewReaderSize(r, size)
	return s
}

// NewRecordLines additionally reserves each finite Gradle/gemspec/requirements/
// properties record's conversion before exposing it to its parser. Parsers
// with their own per-node/per-container reservation use NewLines instead.
func NewRecordLines(ctx context.Context, r io.Reader) *Lines {
	s := NewLines(ctx, r)
	s.records = true
	return s
}

func (s *Lines) Scan() bool {
	s.text = ""
	if s.err != nil || s.done {
		return false
	}
	s.buffer = s.buffer[:0]
	for {
		if s.err = s.ctx.Err(); s.err != nil {
			return false
		}
		part, err := s.reader.ReadSlice('\n')
		if int64(len(part)) > s.state.Limits.MaxFileBytes-s.total {
			s.err = scanerr.New(scanerr.ResourceLimit, "line input bytes")
			return false
		}
		s.total += int64(len(part))
		if len(part) > s.max-len(s.buffer) {
			s.err = scanerr.New(scanerr.ResourceLimit, "line bytes")
			return false
		}
		// The common single-buffer line needs only its final immutable string.
		var line []byte
		if len(s.buffer) == 0 && !errors.Is(err, bufio.ErrBufferFull) {
			line = part
		} else {
			need := len(s.buffer) + len(part)
			if need > cap(s.buffer) {
				capacity := min(s.max, max(need, 2*cap(s.buffer)))
				if s.err = s.state.Working(budget.SizeOfBytes(capacity)); s.err != nil {
					return false
				}
				next := make([]byte, len(s.buffer), capacity)
				copy(next, s.buffer)
				s.buffer = next
			}
			s.buffer = append(s.buffer, part...)
			line = s.buffer
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err == io.EOF {
			s.done = true
		} else if err != nil {
			s.err = err
			return false
		}
		if len(line) == 0 && s.done {
			return false
		}
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		need := budget.SizeString + int64(len(line))
		if s.records {
			need = 4096 + 256*int64(len(line))
		}
		if s.err = s.state.Working(need); s.err != nil {
			return false
		}
		s.text = string(line)
		return true
	}
}
func (s *Lines) Text() string { return s.text }
func (s *Lines) Err() error   { return s.err }
