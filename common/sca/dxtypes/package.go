package dxtypes

import "github.com/yaklang/yaklang/common/utils"

type Package struct {
	Name    string
	Version string

	// sha1://abc
	// md5://abc
	// sha256://abc
	// so...on
	Verification string

	License []string // Maybe...

	// Related
	UpStreamDependencies   []Package
	DownStreamDependencies []Package

	Indirect bool

	// 订正 CPE 和 强制关联 CVE
	AmendedCPE    []string
	AssociatedCVE []string

	/*
		// more go.mod or dependency batch files.
		// which times is the package used?
		Count int
	*/
	ExtraInfo []InfoPair
}

func (p *Package) Identifier() string {
	return utils.CalcSha1(p.Name, p.Version)
}
