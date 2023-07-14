package dxtypes

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

type Package struct {
	id string // name + version

	Name           string
	Version        string
	IsVersionRange bool // Version is a version range

	FromFile     []string
	FromAnalyzer []string

	// Optional

	// sha1:abc
	// md5:abc
	// sha256:abc
	// ...
	Verification string

	License []string

	// Related
	// id -> package
	UpStreamPackages   map[string]*Package
	DownStreamPackages map[string]*Package

	DependsOn PackageRelationShip

	Potential bool

	// 订正 CPE 和 强制关联 CVE
	AmendedCPE    []string
	AssociatedCVE []string
}

type PackageRelationShip struct {
	And map[string]string   // key: package name, value: version range
	Or  []map[string]string // key: package name, value: version range
}

func (p *Package) Identifier() string {
	//if p.id == "" {
	//	p.id = utils.CalcSha1(p.Name, p.Version)
	//}
	//return p.id
	return utils.CalcSha1(p.Name, p.Version)
}

func (p Package) String() string {
	// 	license := "nil"
	// 	if len(p.License) > 0 {
	// 		license = fmt.Sprintf(`[]string{"%s"}`, strings.Join(p.License, `", "`))
	// 	}
	// 	return fmt.Sprintf(`{
	// 	Name:         "%s",
	// 	Version:      "%s",
	// 	Verification: "%s",
	// 	License:      %s,
	// 	Indirect:     %v,
	// 	Potential:    %v,
	// },`, p.Name, p.Version, p.Verification, license, p.Indirect, p.Potential)
	ret := fmt.Sprintf("%s-%s", p.Name, p.Version)
	ret += "\n\tupstream: "
	ret += strings.Join(
		lo.MapToSlice(p.UpStreamPackages, func(name string, pkg *Package) string {
			return fmt.Sprintf("%s-%s", name, pkg.Version)
		}),
		",",
	)
	ret += "\n\tdownstream: "
	ret += strings.Join(
		lo.MapToSlice(p.DownStreamPackages, func(name string, pkg *Package) string {
			return fmt.Sprintf("%s-%s", name, pkg.Version)
		}),
		",",
	)
	ret += "\n\tverfication: " + p.Verification
	ret += "\n\tlicense: " + strings.Join(p.License, ",")
	ret += fmt.Sprintf("\n\tindirect: %v", p.Indirect)
	ret += fmt.Sprintf("\n\tpotential: %v", p.Potential)
	ret += fmt.Sprintf("\n\tdependson: %v", p.DependsOn)
	ret += fmt.Sprintf("\n\tfromAnalyzer: %v", p.FromAnalyzer)
	ret += fmt.Sprintf("\n\tfromFile: %v", p.FromFile)
	return ret
}

func (p *Package) SetFrom(analyzer, file string) {
	if p.FromAnalyzer == nil {
		p.FromAnalyzer = make([]string, 0)
	}
	p.FromAnalyzer = append(p.FromAnalyzer, analyzer)
	if p.FromFile == nil {
		p.FromFile = make([]string, 0)
	}
	p.FromFile = append(p.FromFile, file)
}

func (p *Package) From() ([]string, []string) {
	return p.FromAnalyzer, p.FromFile
}
func (down *Package) LinkDepend(up *Package) {
	if up.DownStreamPackages == nil {
		up.DownStreamPackages = make(map[string]*Package)
	}
	up.DownStreamPackages[down.Identifier()] = down
	if down.UpStreamPackages == nil {
		down.UpStreamPackages = make(map[string]*Package)
	}
	down.UpStreamPackages[up.Identifier()] = up
}

// merge p2 to p1
func (p *Package) Merge(p2 *Package) *Package {
	p.Potential = false
	if p.License == nil {
		p.License = make([]string, 0)
	}
	p.License = lo.Uniq(append(p.License, p2.License...))
	if p.FromAnalyzer == nil {
		p.FromAnalyzer = make([]string, 0)
	}
	p.FromAnalyzer = append(p.FromAnalyzer, p2.FromAnalyzer...)
	if p.FromFile == nil {
		p.FromFile = make([]string, 0)
	}
	p.FromFile = append(p.FromFile, p2.FromFile...)

	for _, p2up := range p2.UpStreamPackages {
		p.LinkDepend(p2up)
		if p.Identifier() != p2.Identifier() {
			delete(p2up.DownStreamPackages, p2.Identifier())
		}
	}
	for _, p2down := range p2.DownStreamPackages {
		p2down.LinkDepend(p)
		if p.Identifier() != p2.Identifier() {
			delete(p2down.UpStreamPackages, p2.Identifier())
		}
	}
	return p
}

func (p *Package) CanMerge(p2 *Package) bool {
	if p.Verification != "" && p2.Verification != "" && p.Verification != p2.Verification {
		return false
	}
	if p.Name != p.Name {
		return false
	}
	// version
	if p2.IsVersionRange {
		if p2.Version == "*" {
			// pass
		} else {
			index := strings.IndexFunc(p2.Version, func(r rune) bool {
				return r >= '0' && r <= '9'
			})
			if index == -1 {
				return false
			}
			op := p2.Version[:index]
			version := p2.Version[index:]
			ret, err := utils.VersionCompare(p.Version, version)
			if err != nil {
				return false
			}
			if strings.Contains(op, "=") && ret == 0 {
				return true
			}
			if strings.Contains(op, ">") && ret > 0 {
				return true
			}
			if strings.Contains(op, "<") && ret < 0 {
				return true
			}
		}
	} else if p.Version == p2.Version {
		return true
	}

	return false
}
