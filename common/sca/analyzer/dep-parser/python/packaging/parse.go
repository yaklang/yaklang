package packaging

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/pyrequire"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	"github.com/yaklang/yaklang/common/sca/model"
	"io"
	"net/textproto"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

// Parse parses egg and wheel metadata.
// e.g. .egg-info/PKG-INFO and dist-info/METADATA
func (*Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	ctx := types.ContextOf(r)
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	raw, err := textdecode.ReadContext(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	// MIME maps, continuation joins, fixed records and requirement tokens
	// are reserved before the stdlib header decoder allocates any of them.
	if err := budget.From(ctx).Working(8192 + 512*int64(len(raw))); err != nil {
		return nil, nil, err
	}
	rd := textproto.NewReader(bufio.NewReader(bytes.NewReader(raw)))
	var diagnostics []model.Diagnostic
	h, err := rd.ReadMIMEHeader()
	if e := textproto.ProtocolError(""); errors.As(err, &e) {
		// A MIME header may contain bytes in the key or value outside the set allowed by RFC 7230.
		// cf. https://cs.opensource.google/go/go/+/a6642e67e16b9d769a0c08e486ba08408064df19
		// However, our required key/value could have been correctly parsed,
		// so we continue with the subsequent process.
		diagnostics = append(diagnostics, model.Diagnostic{Code: "malformed_input", Stage: "parse", Reason: err.Error(), Incomplete: true})
	} else if err != nil && err != io.EOF {
		return nil, nil, fmt.Errorf("read MIME error: %w", err)
	}

	name, err := uniqueMIME(h, "Name")
	if err != nil {
		return nil, nil, err
	}
	version, err := uniqueMIME(h, "Version")
	if err != nil {
		return nil, nil, err
	}
	if name == "" || version == "" {
		return nil, nil, fmt.Errorf("malformed_input: metadata name or version")
	}

	// "License-Expression" takes precedence as "License" is deprecated.
	// cf. https://peps.python.org/pep-0639/#deprecate-license-field
	var license string
	if l := h.Get("License-Expression"); l != "" {
		license = l
	} else if l := h.Get("License"); l != "" {
		license = l
	} else {
		for _, classifier := range h.Values("Classifier") {
			if strings.HasPrefix(classifier, "License :: ") {
				values := strings.Split(classifier, " :: ")
				license = values[len(values)-1]
				break
			}
		}
	}
	if license == "" && h.Get("License-File") != "" {
		license = "file://" + h.Get("License-File")
	}

	for _, values := range h {
		for _, v := range values {
			if len(v) > budget.From(ctx).Limits.MaxFieldBytes {
				return nil, nil, fmt.Errorf("resource_limit: metadata field")
			}
		}
	}
	id := "metadata:" + name + "@" + version
	if err := budget.From(ctx).Result(budget.SizeOfPackage(name, version, license)); err != nil {
		return nil, nil, err
	}
	source := metadataSource(h)
	start, end := mimeHeaderSpan(raw)
	qs := []types.Requirement{}
	for _, req := range h.Values("Requires-Dist") {
		q, e := pyrequire.Declaration(req)
		if e != nil {
			return nil, nil, fmt.Errorf("malformed_input: Requires-Dist: %w", e)
		}
		if err := budget.From(ctx).Result(budget.SizeOfEdge() + budget.SizeOfString(q.Name) + budget.SizeOfString(q.Constraint)); err != nil {
			return nil, nil, err
		}
		qs = append(qs, types.Requirement{Target: q.Name, Constraint: q.Constraint, Condition: requireCondition(q)})
	}
	var deps []types.Dependency
	if len(qs) > 0 {
		deps = append(deps, types.Dependency{ID: id, Requirements: qs})
	}
	lib := types.Library{
		ID:          id,
		Name:        name,
		Version:     version,
		License:     license,
		Source:      source,
		Diagnostics: diagnostics,
		Locations:   []types.Location{{StartLine: start, EndLine: end}},
	}
	return []types.Library{lib}, deps, ctx.Err()
}

func uniqueMIME(h textproto.MIMEHeader, key string) (string, error) {
	vals := h.Values(key)
	if len(vals) == 0 {
		return "", nil
	}
	first := vals[0]
	for _, v := range vals[1:] {
		if v != first {
			return "", fmt.Errorf("malformed_input: conflicting %s", key)
		}
	}
	return first, nil
}

func metadataSource(h textproto.MIMEHeader) string {
	if v := h.Get("Home-page"); v != "" {
		return v
	}
	for _, u := range h.Values("Project-URL") {
		label, url, ok := strings.Cut(u, ",")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(label)) {
		case "homepage", "home-page", "home":
			return strings.TrimSpace(url)
		}
	}
	return ""
}

func requireCondition(q pyrequire.Record) string {
	var parts []string
	if q.Marker != "" {
		parts = append(parts, q.Marker)
	}
	if q.Extras != "" {
		parts = append(parts, "extras=["+q.Extras+"]")
	}
	if q.URL != "" {
		parts = append(parts, q.URL)
	}
	return strings.Join(parts, "; ")
}

func mimeHeaderSpan(raw []byte) (int, int) {
	text := string(raw)
	lines := strings.Split(text, "\n")
	end := 0
	for i, line := range lines {
		if strings.TrimRight(line, "\r") == "" {
			if i == 0 {
				continue
			}
			end = i
			break
		}
		end = i + 1
	}
	if end < 1 {
		end = 1
	}
	return 1, end
}
