// Package pyrequire preserves dependency declarations without selecting an
// environment, executing Python, or fetching URLs and include files.
package pyrequire

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"regexp"
	"strings"
)

type Record struct {
	Name, Version, Constraint, Extras, Marker, URL string
	Hashes                                         []string
	StartLine, EndLine                             int
}

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*`)
var clausePattern = regexp.MustCompile(`^(===|~=|==|!=|<=|>=|<|>)[A-Za-z0-9][A-Za-z0-9.*+!_-]*$`)

func Parse(ctx context.Context, data []byte) ([]Record, error) {
	l := budget.From(ctx).Limits
	if int64(len(data)) > min(l.MaxFileBytes, 16<<20) {
		return nil, fmt.Errorf("resource_limit: Python requirements bytes")
	}
	if err := budget.From(ctx).Working(int64(len(data))); err != nil {
		return nil, err
	}
	s := bufio.NewScanner(bytes.NewReader(data))
	s.Buffer(make([]byte, 4096), l.MaxFieldBytes)
	var out []Record
	var line string
	start, n := 1, 0
	for s.Scan() {
		n++
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		raw := strings.TrimSpace(s.Text())
		if line == "" {
			start = n
		}
		if strings.HasSuffix(raw, `\`) {
			line += strings.TrimSuffix(raw, `\`) + " "
			if len(line) > l.MaxFieldBytes {
				return nil, fmt.Errorf("resource_limit: Python continuation")
			}
			continue
		}
		line += raw
		if line == "" || strings.HasPrefix(line, "#") {
			line = ""
			continue
		}
		r, e := Declaration(line)
		if e != nil {
			return out, fmt.Errorf("line %d: %w", start, e)
		}
		r.StartLine = start
		r.EndLine = n
		if err := budget.From(ctx).Result(budget.SizeOfRecord() + budget.SizeOfString(r.Name) + budget.SizeOfString(r.Constraint)); err != nil {
			return nil, err
		}
		out = append(out, r)
		line = ""
		if len(out) > l.MaxExpressionNodes {
			return nil, fmt.Errorf("resource_limit: Python requirement count")
		}
	}
	if e := s.Err(); e != nil {
		return nil, e
	}
	if line != "" {
		return out, fmt.Errorf("malformed_input: unfinished Python continuation")
	}
	return out, nil
}
func Declaration(s string) (Record, error) {
	var r Record
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "-") {
		return r, fmt.Errorf("unsupported_syntax: Python include or installer option")
	}
	// URI fragments are data; a hash outside a URI introduces a requirements comment.
	quote := byte(0)
	uri := strings.Contains(s, " @ ")
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == '#' && (!uri || i == 0 || s[i-1] == ' ') {
			s = s[:i]
			break
		}
	}
	name := namePattern.FindString(s)
	if name == "" {
		return r, fmt.Errorf("malformed_input: Python requirement name")
	}
	r.Name = name
	s = strings.TrimSpace(s[len(name):])
	if strings.HasPrefix(s, "[") {
		end := strings.IndexByte(s, ']')
		if end < 0 {
			return r, fmt.Errorf("malformed_input: Python extras")
		}
		r.Extras = strings.TrimSpace(s[1:end])
		for _, extra := range strings.Split(r.Extras, ",") {
			e := strings.TrimSpace(extra)
			if namePattern.FindString(e) != e || e == "" {
				return r, fmt.Errorf("malformed_input: Python extra")
			}
		}
		s = strings.TrimSpace(s[end+1:])
	}
	if i := strings.Index(s, "--hash="); i >= 0 {
		tail := strings.Fields(s[i:])
		for _, h := range tail {
			if !strings.HasPrefix(h, "--hash=") {
				return r, fmt.Errorf("unsupported_syntax: Python installer option")
			}
			r.Hashes = append(r.Hashes, strings.TrimPrefix(h, "--hash="))
		}
		s = strings.TrimSpace(s[:i])
	}
	if i := strings.IndexByte(s, ';'); i >= 0 {
		r.Marker = strings.TrimSpace(s[i+1:])
		s = strings.TrimSpace(s[:i])
		if r.Marker == "" {
			return r, fmt.Errorf("malformed_input: empty Python marker")
		}
		if e := marker(r.Marker); e != nil {
			return r, e
		}
	}
	if strings.HasPrefix(s, "@") {
		r.URL = strings.TrimSpace(s[1:])
		if r.URL == "" || strings.ContainsAny(r.URL, "\r\n\t ") {
			return r, fmt.Errorf("malformed_input: Python URL")
		}
		return r, nil
	}
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "("), ")"))
	r.Constraint = strings.ReplaceAll(s, " ", "")
	if r.Constraint != "" {
		for _, clause := range strings.Split(r.Constraint, ",") {
			if !clausePattern.MatchString(clause) {
				return r, fmt.Errorf("unsupported_syntax: Python version constraint %q", clause)
			}
		}
	}
	if strings.HasPrefix(r.Constraint, "==") && !strings.HasPrefix(r.Constraint, "===") && !strings.ContainsAny(r.Constraint, ",*") {
		r.Version = strings.TrimPrefix(r.Constraint, "==")
	}
	return r, nil
}
func marker(s string) error {
	quote := rune(0)
	depth := 0
	for _, c := range s {
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(':
			depth++
			if depth > 64 {
				return fmt.Errorf("resource_limit: Python marker depth")
			}
		case ')':
			depth--
			if depth < 0 {
				return fmt.Errorf("malformed_input: Python marker parentheses")
			}
		}
	}
	if quote != 0 || depth != 0 {
		return fmt.Errorf("malformed_input: Python marker quotes or parentheses")
	}
	return nil
}
