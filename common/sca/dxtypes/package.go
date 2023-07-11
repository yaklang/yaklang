package dxtypes

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
)

type Package struct {
	id   string // name + version
	from string // analyzer name

	Name    string
	Version string

	// Optional

	// sha1:abc
	// md5:abc
	// sha256:abc
	// ...
	Verification string

	License []string

	// Related
	UpStreamPackages   []*Package
	DownStreamPackages []*Package

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
		lo.Map(p.UpStreamPackages, func(pkg *Package, _ int) string {
			return fmt.Sprintf("%s-%s", pkg.Name, pkg.Version)
		}),
		",",
	)
	ret += "\n\tdownstream: "
	ret += strings.Join(
		lo.Map(p.DownStreamPackages, func(pkg *Package, _ int) string {
			return fmt.Sprintf("%s-%s", pkg.Name, pkg.Version)
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
