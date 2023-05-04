package utils

import (
	"bufio"
	"bytes"
	"github.com/pkg/errors"
	"os"
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

func FileLineReader(file string) (chan []byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, errors.Errorf("failed to read file: %s", err)
	}

	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)
	outC := make(chan []byte)
	go func() {
		defer close(outC)
		bomHandled := NewBool(false)
		for scanner.Scan() {
			raw := bytes.TrimSpace(scanner.Bytes())
			if !bomHandled.IsSet() {
				raw = RemoveBOM(raw)
				bomHandled.Set()
			}
			outC <- raw
		}
	}()

	return outC, nil
}
