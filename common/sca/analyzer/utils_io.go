package analyzer

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/sca/core/budget"
)

// ReadBlock reads Analyzer data block from the underlying reader until Analyzer blank line is encountered.
func ReadBlock(r *bufio.Reader) ([]byte, error) {
	return readBlock(context.Background(), r)
}

func readBlock(ctx context.Context, r *bufio.Reader) ([]byte, error) {
	ctx = budget.Ensure(ctx)
	var block []byte
	st := budget.From(ctx)
	l := budget.From(ctx).Limits
	lineBytes, lines := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		part, err := r.ReadSlice('\n')
		if len(part) > l.MaxFieldBytes-lineBytes || int64(len(part)) > l.MaxFileBytes-int64(len(block)) {
			return nil, fmt.Errorf("resource_limit: text block or field")
		}
		blank := lineBytes == 0 && (bytes.Equal(part, []byte("\n")) || bytes.Equal(part, []byte("\r\n")))
		lineBytes += len(part)
		if !blank {
			next, e := budget.Grow(st, block, len(part), 1)
			if e != nil {
				return nil, e
			}
			block = append(next, part...)
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		lines++
		if lines > l.MaxExpressionNodes {
			return nil, fmt.Errorf("resource_limit: text block fields")
		}
		lineBytes = 0
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			return block, err
		}
		if blank && len(block) != 0 {
			return block, nil
		}
	}
}
