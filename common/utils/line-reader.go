package utils

import (
	"bufio"
	"bytes"
	"context"
	"os"

	"github.com/pkg/errors"
)

func RemoveBOM(raw []byte) []byte {
	if len(raw) > 3 {
		if raw[0] == '\xef' && raw[1] == '\xbb' && raw[2] == '\xbf' {
			return raw[3:]
		}
	}
	return raw
}

func RemoveBOMForString(raw string) string {
	if len(raw) > 3 {
		if raw[0] == '\xef' && raw[1] == '\xbb' && raw[2] == '\xbf' {
			return raw[3:]
		}
	}
	return raw
}

func FileLineReaderWithContext(file string, ctx context.Context) (chan []byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, errors.Errorf("failed to read file: %s", err)
	}

	reader := bufio.NewReader(f)
	outC := make(chan []byte)
	go func() {
		defer f.Close()
		defer close(outC)
		bomHandled := NewBool(false)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			lineRaw, err := reader.ReadBytes('\n')
			if err != nil && len(lineRaw) == 0 {
				break
			}
			raw := bytes.TrimSpace(lineRaw)
			if !bomHandled.IsSet() {
				raw = RemoveBOM(raw)
				bomHandled.Set()
			}
			outC <- raw
		}
	}()

	return outC, nil
}

func FileLineReader(file string) (chan []byte, error) {
	return FileLineReaderWithContext(file, context.Background())
}
