package conan

import (
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	"strings"

	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/jsonrecord"

	"slices"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/model"

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
	ctx := types.ContextOf(r)
	var lock LockFile
	input, err := textdecode.ReadRaw(ctx, r, 16<<20)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read canon lock file: %w", err)
	}
	nodes, err := jsonrecord.Decode(ctx, input, &lock)
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

	st := budget.From(ctx)
	parsed := map[string]types.Library{}
	for i, node := range lock.GraphLock.Nodes {
		if node.Ref == "" {
			continue
		}
		lib, err := parseRef(node)
		if err != nil {
			return nil, nil, err
		}

		direct := slices.Contains(directDeps, i)
		lib.Indirect = !direct

		if err := st.Result(budget.SizeOfPackage(lib.Name, lib.Version, node.Ref)); err != nil {
			return nil, nil, err
		}
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
		var reqs []types.Requirement
		seen := map[string]struct{}{}
		for _, req := range node.Requires {
			if _, dup := seen[req]; dup {
				continue
			}
			if err := st.Insert(req); err != nil {
				return nil, nil, err
			}
			seen[req] = struct{}{}
			if child, ok := parsed[req]; ok {
				q := types.Requirement{Target: child.Name, Constraint: child.Version, Condition: child.Source, Resolved: child.ID}
				if err := chargeConanEdge(st, q); err != nil {
					return nil, nil, err
				}
				childDeps = append(childDeps, child.ID)
				reqs = append(reqs, q)
				continue
			}
			q := types.Requirement{Condition: req}
			if err := chargeConanEdge(st, q); err != nil {
				return nil, nil, err
			}
			reqs = append(reqs, q)
			suffix := " is missing"
			if n, ok := lock.GraphLock.Nodes[req]; ok {
				suffix = " has empty ref"
				if n.Ref != "" {
					suffix = " is not a package"
				}
			}
			const prefix = "Conan node "
			need, err := budget.SizeAdd(budget.SizeString, int64(len(prefix)+len(req)+len(suffix)))
			if err != nil {
				return nil, nil, err
			}
			if err := st.Result(need); err != nil {
				return nil, nil, err
			}
			reason := prefix + req + suffix
			lib.Diagnostics = append(lib.Diagnostics, model.Diagnostic{Code: "evidence_insufficient", Stage: "conan", Reason: reason, Incomplete: true})
		}
		if len(childDeps) != 0 || len(reqs) != 0 {
			deps = append(deps, types.Dependency{
				ID:           lib.ID,
				DependsOn:    childDeps,
				Requirements: reqs,
			})
		}

		libs = append(libs, lib)
	}
	return libs, deps, nil
}

func chargeConanEdge(st *budget.State, q types.Requirement) error {
	need, err := budget.SizeAdd(budget.SizeOfEdge(), budget.SizeOfString(q.Target), budget.SizeOfString(q.Constraint), budget.SizeOfString(q.Condition))
	if err != nil {
		return err
	}
	return st.Result(need)
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
