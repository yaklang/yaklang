package stream_parser

import (
	"bytes"
	"fmt"
	"unicode/utf8"
)

// A stateless identification candidate must validate the entire first line.
// Native connection decoding additionally handles pre-identification lines and
// guarded compatibility evidence; this rule alone cannot infer that context.
func validateSSHIdentification(raw []byte) error {
	if len(raw) == 0 || len(raw) > 255 || raw[len(raw)-1] != '\n' {
		return fmt.Errorf("ssh: identification length or termination is invalid")
	}
	line := bytes.TrimSuffix(raw[:len(raw)-1], []byte{'\r'})
	var software []byte
	switch {
	case bytes.HasPrefix(line, []byte("SSH-2.0-")):
		software = line[8:]
	case bytes.HasPrefix(line, []byte("SSH-1.99-")):
		software = line[9:]
	default:
		return fmt.Errorf("ssh: identification version is unsupported")
	}
	if at := bytes.IndexByte(software, ' '); at >= 0 {
		software = software[:at]
	}
	if len(software) == 0 {
		return fmt.Errorf("ssh: software version is empty")
	}
	for _, b := range software {
		if b <= 32 || b >= 127 || b == '-' {
			return fmt.Errorf("ssh: software version is invalid")
		}
	}
	for _, b := range line {
		if b < 32 || b == 127 {
			return fmt.Errorf("ssh: identification contains control bytes")
		}
	}
	if !utf8.Valid(line) {
		return fmt.Errorf("ssh: identification is not UTF-8")
	}
	return nil
}
