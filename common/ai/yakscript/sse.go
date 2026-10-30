package yakscript

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ReadSSE reads UTF-8 SSE events, independent of HTTP chunk boundaries. Returning
// io.EOF from the callback stops successfully (e.g. an adapter's done event).
// It does not parse JSON, recognize business completion, expose id/retry fields,
// or reconnect. EOF with a pending event returns io.ErrUnexpectedEOF; callers
// must also validate their own terminal markers after a clean event boundary.
func ReadSSE(reader io.Reader, callback func(event, data string) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 8<<20)
	scanner.Split(splitSSELine)
	event := ""
	var data strings.Builder
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			line = strings.TrimPrefix(line, "\ufeff")
			first = false
		}
		if line == "" {
			if data.Len() > 0 {
				if event == "" {
					event = "message"
				}
				err := callback(event, strings.TrimSuffix(data.String(), "\n"))
				if err == io.EOF {
					return nil
				}
				if err != nil {
					return err
				}
			}
			event = ""
			data.Reset()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			event = value
		case "data":
			if data.Len()+len(value)+1 > 8<<20 {
				return fmt.Errorf("SSE event exceeds 8 MiB")
			}
			data.WriteString(value)
			data.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if data.Len() > 0 || event != "" {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func splitSSELine(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' {
			return i + 1, data[:i], nil
		}
		if b == '\r' {
			if i+1 == len(data) && !atEOF {
				return 0, nil, nil
			}
			if i+1 < len(data) && data[i+1] == '\n' {
				return i + 2, data[:i], nil
			}
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
