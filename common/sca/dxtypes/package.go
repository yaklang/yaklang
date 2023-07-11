package dxtypes

import "github.com/yaklang/yaklang/common/utils"

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
