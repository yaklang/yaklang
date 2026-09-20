package conan

import (
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/sca/core/jsonrecord"

	"slices"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type LockFile struct {
	GraphLock GraphLock `json:"graph_lock"`
}

type GraphLock struct {
	Nodes map[string]Node `json:"nodes"`
}

type Node struct {
	Ref       string   `json:"ref"`
	Requires  []string `json:"requires"`
	StartLine int
	EndLine   int
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	var lock LockFile
	input, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read canon lock file: %w", err)
	}
	nodes, err := jsonrecord.Decode(types.ContextOf(r), input, &lock)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode canon lock file: %w", err)
	}
	for k, v := range lock.GraphLock.Nodes {
		v.StartLine, v.EndLine = nodes.Get("graph_lock", "nodes", k).Lines()
		lock.GraphLock.Nodes[k] = v
	}

	// Get a list of direct dependencies
	var directDeps []string
	if root, ok := lock.GraphLock.Nodes["0"]; ok {
		directDeps = root.Requires
	}

	// Parse packages
	parsed := map[string]types.Library{}
	for i, node := range lock.GraphLock.Nodes {
		if node.Ref == "" {
			continue
		}
		lib, err := parseRef(node)
		if err != nil {
			return nil, nil, err
		}

		// Determine if the package is a direct dependency or not
		direct := slices.Contains(directDeps, i)
		lib.Indirect = !direct

		lib.ID = i
		lib.Source = node.Ref
		parsed[i] = lib
	}

	// Parse dependency graph
	var libs []types.Library
	var deps []types.Dependency
	for i, node := range lock.GraphLock.Nodes {
		lib, ok := parsed[i]
		if !ok {
			continue
		}

		var childDeps []string
		for _, req := range node.Requires {
			if child, ok := parsed[req]; ok {
				childDeps = append(childDeps, child.ID)
			} else {
				childDeps = append(childDeps, req)
			}
		}
		if len(childDeps) != 0 {
			deps = append(deps, types.Dependency{
				ID:        lib.ID,
				DependsOn: childDeps,
			})
		}

		libs = append(libs, lib)
	}
	return libs, deps, nil
}

func parseRef(node Node) (types.Library, error) {
	// full ref format: package/version@user/channel#rrev:package_id#prev
	// various examples:
	// 'pkga/0.1@user/testing'
	// 'pkgb/0.1.0'
	// 'pkgc/system'
	// 'pkgd/0.1.0#7dcb50c43a5a50d984c2e8fa5898bf18'
	ss := strings.Split(strings.Split(strings.Split(node.Ref, "@")[0], "#")[0], "/")
	if len(ss) != 2 {
		return types.Library{}, fmt.Errorf("Unable to determine conan dependency: %q", node.Ref)
	}
	return types.Library{
		ID:      fmt.Sprintf("%s/%s", ss[0], ss[1]),
		Name:    ss[0],
		Version: ss[1],
		Locations: []types.Location{
			{
				StartLine: node.StartLine,
				EndLine:   node.EndLine,
			},
		},
	}, nil
}
