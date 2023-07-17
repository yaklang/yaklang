package analyzer

import (
	"bufio"
	"bytes"
)

// ReadBlock reads Analyzer data block from the underlying reader until Analyzer blank line is encountered.
func ReadBlock(r *bufio.Reader) ([]byte, error) {
	var block bytes.Buffer

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return block.Bytes(), err
		}

		if line == "\n" || line == "\r\n" {
			break
		}

		block.WriteString(line)
	}

	return block.Bytes(), nil
}
