package utils

import (
	"bufio"
	"io"
)

func ReadWithLen(r io.Reader, length int) ([]byte, int) {
	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanBytes)

	var (
		output []byte
		count  int
	)
	for scanner.Scan() {
		count += 1
		output = append(output, scanner.Bytes()...)

		if count >= length {
			break
		}
	}

	return output, len(output)
}
