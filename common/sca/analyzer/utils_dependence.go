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

func chargeAppend[T any](st *budget.State, s []T, v T, elem int64) ([]T, error) {
	grown, err := budget.Grow(st, s, 1, elem)
	if err != nil {
		return s, err
	}
	return append(grown, v), nil
}

func cloneStrings(st *budget.State, s []string) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	if err := st.Result(budget.SizeOfStrings(s)); err != nil {
		return nil, err
	}
	if len(s) == 0 {
		return []string{}, nil
	}
	return append([]string(nil), s...), nil
}

func appendClonedStrings(st *budget.State, dst, extra []string) ([]string, error) {
	for _, v := range extra {
		var err error
		dst, err = chargeAppend(st, dst, v, budget.SizeOfString(v))
		if err != nil {
			return dst, err
		}
	}
	return dst, nil
}

func chargeDetailsCopy(st *budget.State, d dxtypes.PackageDetails) error {
	nloc, err := budget.SizeMul(len(d.Locations), budget.SizeObject)
	if err != nil {
		return err
	}
	nreq, err := budget.SizeMul(len(d.Requirements), budget.SizeOfRecord())
	if err != nil {
		return err
	}
	ndiag, err := budget.SizeMul(len(d.Diagnostics), budget.SizeOfObservation())
	if err != nil {
		return err
	}
	return st.Result(budget.SizeObject + budget.SizeMap + nloc + nreq + ndiag + budget.SizeOfStrings(d.RawLicenses) + budget.SizeOfStrings(d.UnresolvedDependencies) + budget.SizeOfStrings(d.Provides))
}

func snapshotDetails(st *budget.State, src dxtypes.PackageDetails) (*dxtypes.PackageDetails, error) {
	if err := chargeDetailsCopy(st, src); err != nil {
		return nil, err
	}
	nd := src
	nd.Locations = append([]dxtypes.SourceRange(nil), src.Locations...)
	nd.Requirements = append(nd.Requirements[:0:0], src.Requirements...)
	nd.Diagnostics = append(nd.Diagnostics[:0:0], src.Diagnostics...)
	nd.RawLicenses = append([]string(nil), src.RawLicenses...)
	nd.UnresolvedDependencies = append([]string(nil), src.UnresolvedDependencies...)
	nd.Provides = append([]string(nil), src.Provides...)
	return &nd, nil
}

func mergeOwnedDetails(st *budget.State, dst *dxtypes.PackageDetails, src dxtypes.PackageDetails) error {
	if err := chargeDetailsCopy(st, src); err != nil {
		return err
	}
	holder := &dxtypes.Package{PackageDetails: dst}
	holder.MergeDetails(src)
	return nil
}

type mergeSlot struct {
	dst                       *dxtypes.Package
	license, fromFile, fromAn []string
	details                   *dxtypes.PackageDetails
}

func absorbPackage(st *budget.State, s *mergeSlot, p *dxtypes.Package, first bool) error {
	lic, err := cloneStrings(st, p.License)
	if err != nil {
		return err
	}
	files, err := cloneStrings(st, p.FromFile)
	if err != nil {
		return err
	}
	ans, err := cloneStrings(st, p.FromAnalyzer)
	if err != nil {
		return err
	}
	if first {
		s.license, s.fromFile, s.fromAn = lic, files, ans
	} else {
		if s.license, err = appendClonedStrings(st, s.license, lic); err != nil {
			return err
		}
		if s.fromFile, err = appendClonedStrings(st, s.fromFile, files); err != nil {
			return err
		}
		if s.fromAn, err = appendClonedStrings(st, s.fromAn, ans); err != nil {
			return err
		}
	}
	if p.PackageDetails == nil {
		return nil
	}
	src := p.Details()
	if s.details == nil {
		s.details, err = snapshotDetails(st, src)
		return err
	}
	return mergeOwnedDetails(st, s.details, src)
}

// MergePackagesBudget merges exact identities under st. Controllable merge
// working-set is charged before allocation. Evidence is copied onto independent
// backing so shared *PackageDetails or slice headers cannot be read after this
// round and appended again. Same *Package is visited once; each distinct
// instance contributes its original evidence once. On error, pkgs is unchanged.
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
	// Empty map headers only. Do not reserve len(pkgs) — nil/empty inputs must
	// not allocate per-index capacity.
	if err := st.Result(budget.SizeMap * 2); err != nil {
		return nil, err
	}
	seenPtr := map[*dxtypes.Package]struct{}{}
	index := map[identity]*mergeSlot{}
	type edge struct{ from, to *dxtypes.Package }
	var edges []edge
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
		key := keyOf(p)
		s := index[key]
		if s == nil {
			if err := st.Result(budget.SizeOfPackage(p.Name, p.Version, p.Verification) + budget.SizeObject + budget.SizePtr); err != nil {
				return nil, err
			}
			s = &mergeSlot{dst: p}
			index[key] = s
			if err := absorbPackage(st, s, p, true); err != nil {
				return nil, err
			}
		} else if err := absorbPackage(st, s, p, false); err != nil {
			return nil, err
		}
		for _, up := range p.UpStreamPackages {
			if up == nil {
				continue
			}
			var err error
			edges, err = chargeAppend(st, edges, edge{p, up}, budget.SizeOfEdge())
			if err != nil {
				return nil, err
			}
		}
	}
	nUnique := len(index)
	outBytes, err := budget.SizeMul(nUnique, budget.SizeObject+budget.SizePtr)
	if err != nil {
		return nil, err
	}
	graphBytes, err := budget.SizeMul(nUnique, budget.SizeMap)
	if err != nil {
		return nil, err
	}
	if err := st.Result(budget.SizeSlice*2 + outBytes + graphBytes); err != nil {
		return nil, err
	}
	for _, s := range index {
		s.dst.License = s.license
		s.dst.FromFile = s.fromFile
		s.dst.FromAnalyzer = s.fromAn
		s.dst.PackageDetails = s.details
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
		dstTo := e.to
		if to != nil {
			dstTo = to.dst
		}
		from.dst.LinkDepend(dstTo)
	}
	names := make([]identity, 0, nUnique)
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
	out := make([]*dxtypes.Package, 0, nUnique)
	for _, id := range names {
		out = append(out, index[id].dst)
	}
	return out, nil
}
