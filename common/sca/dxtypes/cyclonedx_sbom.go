package dxtypes

import (
	"bytes"
	"fmt"
	cdx "github.com/CycloneDX/cyclonedx-go"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/go-funk"
	"strings"
)

func normalCyloneDXHashType(i string) (cdx.HashAlgorithm, bool) {
	switch strings.ToLower(i) {
	case "md5":
		return cdx.HashAlgoMD5, true
	case "sha1", "sha-1":
		return cdx.HashAlgoSHA1, true
	case "sha256", "sha-256":
		return cdx.HashAlgoSHA256, true
	case "sha384", "sha-384":
		return cdx.HashAlgoSHA384, true
	case "sha512", "sha-512":
		return cdx.HashAlgoSHA512, true
	case "sha3-256", "sha3_256":
		return cdx.HashAlgoSHA3_256, true
	case "sha3-384", "sha3_384":
		return cdx.HashAlgoSHA3_384, true
	case "sha3-512", "sha3_512":
		return cdx.HashAlgoSHA3_512, true
	case "blake2b-256", "blake2b_256":
		return cdx.HashAlgoBlake2b_256, true
	case "blake2b-384", "blake2b_384":
		return cdx.HashAlgoBlake2b_384, true
	case "blake2b-512", "blake2b_512":
		return cdx.HashAlgoBlake2b_512, true
	case "blake3":
		return cdx.HashAlgoBlake3, true
	}
	return "", false
}

func dxPackagesToCycloneDXComponent(pkgFilter *filter.StringFilter, pkgs []*Package) []cdx.Component {
	var ret = make([]cdx.Component, 0, len(pkgs))
	for _, pkg := range pkgs {
		id := fmt.Sprintf("%v-%v", pkg.Name, pkg.Version)
		if pkgFilter.Exist(id) {
			continue
		}
		pkgFilter.Insert(id)

		lis := cdx.Licenses(funk.Map(pkg.License, func(s string) cdx.LicenseChoice {
			return cdx.LicenseChoice{
				License: &cdx.License{
					BOMRef:     "",
					ID:         "",
					Name:       s,
					Text:       nil,
					URL:        "",
					Licensing:  nil,
					Properties: nil,
				},
			}
		}).([]cdx.LicenseChoice))
		var cpe string
		if len(pkg.AmendedCPE) > 0 {
			cpe = pkg.AmendedCPE[0]
		}
		var sub []cdx.Component
		if pkg.DownStreamPackages != nil && len(pkg.DownStreamPackages) > 0 {
			var downstream = make([]*Package, 0, len(pkg.DownStreamPackages))
			for _, v := range pkg.DownStreamPackages {
				downstream = append(downstream, v)
			}
			sub = dxPackagesToCycloneDXComponent(pkgFilter, downstream)
		}

		var hashes []cdx.Hash
		if pkg.Verification != "" {
			schema, code, _ := strings.Cut(pkg.Verification, ":")
			if ret, ok := normalCyloneDXHashType(schema); ok {
				hashes = []cdx.Hash{
					{
						Algorithm: ret,
						Value:     code,
					},
				}
			}
		}
		ret = append(ret, cdx.Component{
			Name:       pkg.Name,
			Version:    pkg.Version,
			Hashes:     &hashes, // pkg.Verification
			Licenses:   &lis,
			CPE:        cpe,
			Components: &sub,
		})
	}
	return ret
}

func CreateCycloneDXSBOMByDXPackages(pkgs []*Package) *cdx.BOM {
	bom := cdx.NewBOM()
	filter := filter.NewFilter()
	ret := dxPackagesToCycloneDXComponent(filter, pkgs)
	bom.Components = &ret
	return bom
}

func MarshalCycloneDXBomToJSON(bom *cdx.BOM) ([]byte, error) {
	var buf bytes.Buffer
	err := cdx.NewBOMEncoder(&buf, cdx.BOMFileFormatJSON).Encode(bom)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
