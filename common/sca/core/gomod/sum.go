package gomod

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
)

type SumKey struct {
	Path, Version string
	GoMod         bool
}

// ParseSum returns integrity evidence keyed by the complete module identity and
// content kind. A sum entry alone is never promoted to a dependency component.
func ParseSum(ctx context.Context, data []byte) (map[SumKey]string, error) {
	if len(data) > 16<<20 {
		return nil, fmt.Errorf("resource_limit: go.sum bytes")
	}
	out := map[SumKey]string{}
	s := bufio.NewScanner(bytes.NewReader(data))
	s.Buffer(make([]byte, 4096), 1<<20)
	line := 0
	for s.Scan() {
		line++
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if line > 400000 {
			return nil, fmt.Errorf("resource_limit: go.sum lines")
		}
		parts := strings.Fields(s.Text())
		if len(parts) == 0 {
			continue
		}
		if len(parts) != 3 {
			return nil, fmt.Errorf("malformed_input: go.sum line %d", line)
		}
		k := SumKey{Path: parts[0], Version: strings.TrimSuffix(parts[1], "/go.mod"), GoMod: strings.HasSuffix(parts[1], "/go.mod")}
		if !modulePath(k.Path) || canonicalVersion(k.Version) == "" {
			return nil, fmt.Errorf("malformed_input: go.sum identity at %d", line)
		}
		if !strings.HasPrefix(parts[2], "h1:") {
			return nil, fmt.Errorf("unsupported_syntax: go.sum hash at %d", line)
		}
		hash, err := base64.StdEncoding.DecodeString(parts[2][3:])
		if err != nil || len(hash) != 32 {
			return nil, fmt.Errorf("malformed_input: go.sum digest at %d", line)
		}
		if old, ok := out[k]; ok && old != parts[2] {
			return nil, fmt.Errorf("malformed_input: conflicting go.sum evidence at %d", line)
		}
		out[k] = parts[2]
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
