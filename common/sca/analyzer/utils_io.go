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
	var block bytes.Buffer
	l := budget.From(ctx).Limits
	lineBytes, lines := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		part, err := r.ReadSlice('\n')
		if len(part) > l.MaxFieldBytes-lineBytes || int64(len(part)) > l.MaxFileBytes-int64(block.Len()) {
			return nil, fmt.Errorf("resource_limit: text block or field")
		}
		blank := lineBytes == 0 && (bytes.Equal(part, []byte("\n")) || bytes.Equal(part, []byte("\r\n")))
		lineBytes += len(part)
		if !blank {
			if err := budget.From(ctx).Result(int64(len(part))); err != nil {
				return nil, err
			}
			block.Write(part)
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
			return block.Bytes(), err
		}
		if blank && block.Len() != 0 {
			return block.Bytes(), nil
		}
	}
}
