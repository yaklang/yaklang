package analyzer

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

func parseVersion(version string) ([]int, error) {
	parts := strings.Split(version, ".")
	if len(parts) < 3 {
		return nil, errors.New("not semver version format")
	}
	v := make([]int, 3)
	for i := 0; i < 3; i++ {
		t, err := strconv.Atoi(parts[i])
		if err != nil {
			return nil, err
		}
		v[i] = t
	}
	return v, nil
}

func handlerSemverVersionRange(semverRange string) string {
	if len(semverRange) == 0 {
		return ""
	}
	if semverRange[0] == '~' {
		v, err := parseVersion(semverRange[1:])
		if err != nil {
			return semverRange
		}
		return fmt.Sprintf(">= %d.%d.%d && < %d.%d.%d", v[0], v[1], v[2], v[0], v[1]+1, 0)
	}

	if semverRange[0] == '^' {
		v, err := parseVersion(semverRange[1:])
		if err != nil {
			return semverRange
		}
		if v[0] == 0 {
			if v[1] == 0 {
				return fmt.Sprintf(">= 0.0.%d && < 0.0.%d", v[2], v[2]+1)
			}
			return fmt.Sprintf(">= 0.%d.%d && < 0.%d.0", v[1], v[2], v[1]+1)
		}
		return fmt.Sprintf(">= %d.%d.%d && < %d.%d.%d", v[0], v[1], v[2], v[0]+1, 0, 0)
	}

	return semverRange
}

// MergePackages is the compatibility exact-identity adapter. It never parses
// alternatives, compares versions, or searches a same-name candidate list.
//
// Compatibility: the public signature stays ([]*Package) with no error. A
// default bounded State is used. Budget exhaustion must not look like an
// empty inventory, so the original slice is returned unchanged (non-silent:
// data is kept, not replaced with nil). License/from-file/graph fields are
// not mutated on that path, so a later successful merge cannot double-append.
// Callers that need the error should use MergePackagesBudget.
func MergePackages(pkgs []*dxtypes.Package) []*dxtypes.Package {
	return mergePackagesOrKeep(budget.From(context.Background()), pkgs)
}

func mergePackagesOrKeep(st *budget.State, pkgs []*dxtypes.Package) []*dxtypes.Package {
	out, err := MergePackagesBudget(st, pkgs)
	if err != nil {
		return pkgs
	}
	return out
}

func mergePackagesBudget(st *budget.State, pkgs []*dxtypes.Package) ([]*dxtypes.Package, error) {
	return MergePackagesBudget(st, pkgs)
}

// MergePackagesBudget merges exact identities under st. Controllable merge
// working-set (maps, extra evidence copies, edges, output index) is charged
// before any input mutation. Same pointers are visited once so already-merged
// fields cannot be read back and appended again. On error, pkgs and graphs
// are left unchanged.
func MergePackagesBudget(st *budget.State, pkgs []*dxtypes.Package) ([]*dxtypes.Package, error) {
	if st == nil {
		st = budget.From(context.Background())
	}
	type identity struct {
		Digest           [32]byte
		Potential, Range bool
		Evidence         string
	}
	keyOf := func(p *dxtypes.Package) identity {
		return identity{p.IdentityDigest(), p.Potential, p.HasVersionRange(), p.Details().Evidence}
	}
	// Precheck containers: pointer set, identity index, edge/order slices,
	// input-length sort scratch. Charge before allocating them.
	if err := st.Result(budget.SizeMap*2 + budget.SizeSlice*2 + budget.SizeOfSortIndex(len(pkgs))); err != nil {
		return nil, err
	}
	seenPtr := make(map[*dxtypes.Package]struct{}, len(pkgs))
	index := make(map[identity]*dxtypes.Package, len(pkgs))
	order := make([]*dxtypes.Package, 0, len(pkgs))
	type edge struct{ from, to *dxtypes.Package }
	edges := make([]edge, 0, len(pkgs))
	nUnique := 0
	for _, p := range pkgs {
		if p == nil {
			continue
		}
		if _, ok := seenPtr[p]; ok {
			continue
		}
		if err := st.Result(budget.SizePtr); err != nil {
			return nil, err
		}
		seenPtr[p] = struct{}{}
		if err := st.Result(budget.SizePtr); err != nil {
			return nil, err
		}
		order = append(order, p)
		key := keyOf(p)
		dst := index[key]
		if dst == nil {
			if err := st.Result(budget.SizeOfPackage(p.Name, p.Version, p.Verification) + budget.SizeObject + budget.SizePtr); err != nil {
				return nil, err
			}
			index[key] = p
			nUnique++
		} else if dst != p {
			extra := budget.SizeOfStrings(p.License) + budget.SizeOfStrings(p.FromFile) + budget.SizeOfStrings(p.FromAnalyzer)
			if p.PackageDetails != nil {
				d := p.Details()
				extra += budget.SizeMap + int64(len(d.Locations))*budget.SizeObject + int64(len(d.Requirements))*budget.SizeOfRecord() + int64(len(d.Diagnostics))*budget.SizeOfObservation()
				extra += budget.SizeOfStrings(d.RawLicenses) + budget.SizeOfStrings(d.UnresolvedDependencies) + budget.SizeOfStrings(d.Provides)
			}
			if err := st.Result(extra); err != nil {
				return nil, err
			}
		}
		for _, up := range p.UpStreamPackages {
			if up == nil {
				continue
			}
			if err := st.Result(budget.SizeOfEdge()); err != nil {
				return nil, err
			}
			edges = append(edges, edge{p, up})
		}
	}
	if err := st.Result(budget.SizeMap*int64(nUnique) + budget.SizeOfSortIndex(nUnique) + budget.SizeSlice + int64(nUnique)*budget.SizePtr); err != nil {
		return nil, err
	}
	for _, p := range order {
		key := keyOf(p)
		dst := index[key]
		if dst != p {
			if p.PackageDetails != nil {
				dst.MergeDetails(p.Details())
			}
			dst.License = append(dst.License, p.License...)
			dst.FromFile = append(dst.FromFile, p.FromFile...)
			dst.FromAnalyzer = append(dst.FromAnalyzer, p.FromAnalyzer...)
		}
	}
	for _, p := range pkgs {
		if p == nil {
			continue
		}
		p.UpStreamPackages = nil
		p.DownStreamPackages = nil
	}
	for _, e := range edges {
		from, to := index[keyOf(e.from)], index[keyOf(e.to)]
		if from == nil {
			continue
		}
		if to == nil {
			to = e.to
		}
		from.LinkDepend(to)
	}
	names := make([]identity, 0, len(index))
	for id := range index {
		names = append(names, id)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := names[i], names[j]
		if a.Digest != b.Digest {
			return string(a.Digest[:]) < string(b.Digest[:])
		}
		if a.Potential != b.Potential {
			return !a.Potential
		}
		if a.Evidence != b.Evidence {
			return a.Evidence < b.Evidence
		}
		return !a.Range && b.Range
	})
	out := make([]*dxtypes.Package, 0, len(index))
	for _, id := range names {
		out = append(out, index[id])
	}
	return out, nil
}
