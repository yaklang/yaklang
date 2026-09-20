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

	name, version := h.Get("name"), h.Get("version")
	if name == "" || version == "" {
		return nil, nil, errors.New("name or version is empty")
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
	qs := []types.Requirement{}
	for _, raw := range h.Values("Requires-Dist") {
		q, e := pyrequire.Declaration(raw)
		if e != nil {
			return nil, nil, fmt.Errorf("malformed_input: Requires-Dist: %w", e)
		}
		if err := budget.From(ctx).Result(budget.SizeOfEdge() + budget.SizeOfString(q.Name) + budget.SizeOfString(q.Constraint)); err != nil {
			return nil, nil, err
		}
		qs = append(qs, types.Requirement{Target: q.Name, Constraint: q.Constraint, Condition: q.Marker})
	}
	var deps []types.Dependency
	if len(qs) > 0 {
		deps = append(deps, types.Dependency{ID: id, Requirements: qs})
	}
	return []types.Library{{ID: id, Name: name, Version: version, License: license, Diagnostics: diagnostics}}, deps, ctx.Err()
}
