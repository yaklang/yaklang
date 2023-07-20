package lowhttp

import (
	"fmt"
	"strings"
)

// CurlCommand contains exec.Command compatible slice + helpers
type CurlCommand struct {
	slice []string
}

// append appends a string to the CurlCommand
func (c *CurlCommand) append(newSlice ...string) {
	c.slice = append(c.slice, newSlice...)
}

// String returns a ready to copy/paste command
func (c *CurlCommand) String() string {
	return strings.Join(c.slice, " ")
}

func bashEscape(str string) string {
	return `'` + strings.Replace(str, `'`, `'\''`, -1) + `'`
}

func (nopCloser) Close() error { return nil }

// GetCurlCommand returns a CurlCommand corresponding to an http.Request
func GetCurlCommand(isHttps bool, req []byte) (*CurlCommand, error) {
	command := CurlCommand{}

	command.append("curl")
	u, err := ExtractURLFromHTTPRequestRaw(req, isHttps)
	if err != nil {
		return nil, err
	}
	_, body := SplitHTTPPacket(req, func(method string, requestUri string, proto string) error {
		if method != "GET" {
			command.append("-X", method)
		}
		return nil
	}, func(proto string, code int, codeMsg string) error {
		return nil
	}, func(line string) string {
		k, v := SplitHTTPHeader(line)
		switch strings.ToLower(k) {
		case "content-length", "host":
			return line
		}
		command.append("-H", bashEscape(fmt.Sprintf("%s: %s", k, v)))
		return line
	})
	if string(body) != "" {
		bodyEscaped := bashEscape(string(body))
		command.append("-d", bodyEscaped)
	}
	command.append(bashEscape(u.String()))
	return &command, nil
}
