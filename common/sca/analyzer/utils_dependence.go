package analyzer

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

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

// MergePackages is a compatibility exact-identity adapter. It never parses
// alternatives, compares versions, or searches a same-name candidate list.
func MergePackages(pkgs []*dxtypes.Package) []*dxtypes.Package {
	type identity struct {
		Digest           [32]byte
		Potential, Range bool
		Evidence         string
	}
	keyOf := func(p *dxtypes.Package) identity {
		return identity{p.IdentityDigest(), p.Potential, p.HasVersionRange(), p.Details().Evidence}
	}
	index := make(map[identity]*dxtypes.Package, len(pkgs))
	type edge struct{ from, to *dxtypes.Package }
	var edges []edge
	for _, p := range pkgs {
		if p == nil {
			continue
		}
		key := keyOf(p)
		dst := index[key]
		if dst == nil {
			dst = p
			index[key] = dst
		}
		if dst != p {
			if p.PackageDetails != nil {
				dst.MergeDetails(p.Details())
			}
			dst.License = append(dst.License, p.License...)
			dst.FromFile = append(dst.FromFile, p.FromFile...)
			dst.FromAnalyzer = append(dst.FromAnalyzer, p.FromAnalyzer...)
		}
		for _, up := range p.UpStreamPackages {
			if up != nil {
				edges = append(edges, edge{p, up})
			}
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
	return out
}
