package lockjson

import (
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/jsonrecord"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type Swift struct{}
type swiftLock struct {
	Version    int     `json:"version"`
	OriginHash *string `json:"originHash"`
	Object     struct {
		Pins []swiftPin `json:"pins"`
	} `json:"object"`
	Pins []swiftPin `json:"pins"`
}
type swiftPin struct {
	Package       string                                      `json:"package"`
	RepositoryURL string                                      `json:"repositoryURL"`
	Identity      string                                      `json:"identity"`
	Kind          string                                      `json:"kind"`
	Location      string                                      `json:"location"`
	State         struct{ Version, Branch, Revision *string } `json:"state"`
}

func value(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (Swift) Parse(_ fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	ctx := types.ContextOf(r)
	raw, err := textdecode.ReadRaw(ctx, r, 16<<20)
	if err != nil {
		return nil, nil, err
	}
	var lock swiftLock
	nodes, err := jsonrecord.Decode(ctx, raw, &lock)
	if err != nil {
		return nil, nil, err
	}
	if nodes.Get("version") == nil || lock.Version < 1 {
		return nil, nil, malformed("missing Swift resolved version")
	}
	if lock.Version > 3 {
		return nil, nil, scanerr.New(scanerr.UnsupportedSyntax, "Swift resolved version %d", lock.Version)
	}
	pins, pn := lock.Pins, nodes.Get("pins")
	if lock.Version == 1 {
		pins, pn = lock.Object.Pins, nodes.Get("object", "pins")
	}
	if !array(pn) {
		return nil, nil, malformed("Swift pins must be an array")
	}
	if err := budget.From(ctx).Working(int64(len(pins)) * 2048); err != nil {
		return nil, nil, err
	}
	libs := make([]types.Library, 0, len(pins))
	seen := make(map[string]bool, len(pins))
	for i, p := range pins {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		n := pn.Elements()[i]
		if !object(n) || !object(n.Get("state")) {
			return nil, nil, malformed("invalid Swift pin or state")
		}
		name, source, kind := p.Identity, p.Location, p.Kind
		if lock.Version > 1 {
			for _, key := range []string{"identity", "location", "kind"} {
				field := n.Get(key)
				if field == nil || len(field.Raw) == 0 || field.Raw[0] != '"' {
					return nil, nil, malformed("Swift pin requires string " + key)
				}
			}
		}

		if lock.Version == 1 {
			source = p.RepositoryURL
			kind = "sourceControl"
			// SwiftPM v1 derives identity from the last repository path component,
			// not the optional display name. Do not contact the repository.
			trimmed := strings.TrimRight(source, "/")
			name = trimmed[strings.LastIndex(trimmed, "/")+1:]
			name = strings.TrimSuffix(name, ".git")
		}
		name = strings.ToLower(name)
		if strings.TrimSpace(name) == "" {
			return nil, nil, malformed("empty Swift package identity")
		}
		switch kind {
		case "sourceControl", "remoteSourceControl", "localSourceControl":
			if source == "" {
				return nil, nil, malformed("Swift source-control pin has no location")
			}
		case "registry":
			if value(p.State.Version) == "" {
				return nil, nil, malformed("Swift registry pin has no version")
			}
		default:
			return nil, nil, scanerr.New(scanerr.UnsupportedSyntax, "Swift pin kind %q", kind)
		}
		version, branch, revision := value(p.State.Version), value(p.State.Branch), value(p.State.Revision)
		for _, field := range []*string{p.State.Version, p.State.Branch, p.State.Revision} {
			if field != nil && strings.TrimSpace(*field) == "" {
				return nil, nil, malformed("empty Swift pin state field")
			}
		}

		if version == "" && revision == "" || branch != "" && revision == "" || version != "" && branch != "" {
			return nil, nil, malformed("invalid Swift pin state")
		}
		if kind != "registry" && revision == "" {
			return nil, nil, malformed("Swift source-control pin has no revision")
		}
		if seen[name] {
			return nil, nil, malformed("duplicate Swift package identity")
		}
		seen[name] = true
		// Revision/branch are identity evidence, not a fabricated semantic version
		// or package content hash. Registry origin is not present in this file.
		variant := native(kind, branch, revision)
		libs = append(libs, types.Library{ID: native(name, source, variant, p.Package), Name: name, Version: version, Source: source, Variant: variant, Scope: "directness:unknown", Evidence: "locked", Locations: location(n)})
	}
	sort.Sort(types.Libraries(libs))
	// Package.resolved contains pins only; it cannot establish parent-child edges.
	return libs, nil, nil
}
