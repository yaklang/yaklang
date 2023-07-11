package dxtypes

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

type Package struct {
	id string // name + version

	Name    string
	Version string

	fromFile     []string
	fromAnalyzer []string

	// Optional

	// sha1:abc
	// md5:abc
	// sha256:abc
	// ...
	Verification string

	License []string

	// Related
	UpStreamPackages   map[string]*Package
	DownStreamPackages map[string]*Package

	DependsOn PackageRelationShip

	Indirect  bool
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
	if p.id == "" {
		p.id = utils.CalcSha1(p.Name, p.Version)
	}
	return p.id
}

func (p Package) String() string {
	ret := fmt.Sprintf("%s-%s", p.Name, p.Version)
	// for _, pkg:=p.UpStreamPackages{
	// 	ret += fmt.Sprintf("%s-%s,",)
	// }
	ret += "\n\tupstream: "
	ret += strings.Join(
		// lo.Map(p.UpStreamPackages, func(pkg *Package, _ int) string {
		// 	return fmt.Sprintf("%s-%s", pkg.Name, pkg.Version)
		// }),
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

	ret += fmt.Sprintf("\n\tdependson: %v", p.DependsOn)
	// ret += fmt.Sprintf("\n\tindirect: %v", p.Potential)
	return ret
}

// merge p1 to p1
func PackageMerge(p1 *Package, p2 *Package) *Package {
	p1.fromAnalyzer = append(p1.fromAnalyzer, p2.fromAnalyzer...)
	p1.fromFile = append(p1.fromFile, p2.fromFile...)
	for _, p := range p2.UpStreamPackages {
		p1.UpStreamPackages[p.Name] = p
		p.DownStreamPackages[p1.Name] = p1
	}
	for _, p := range p2.DownStreamPackages {
		p1.DownStreamPackages[p.Name] = p
		p.UpStreamPackages[p1.Name] = p1
	}
	return p1
}
