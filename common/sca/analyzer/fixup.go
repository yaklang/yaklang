package analyzer

import (
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

func consolidateDependencies(pkgs []dxtypes.Package) {
	pkgMap := lo.SliceToMap(pkgs, func(item dxtypes.Package) (string, *dxtypes.Package) {
		return item.Name, &item
	})

	for i := range pkgs {
		pkg := &pkgs[i]

		if pkg.UpStreamPackages == nil {
			pkg.UpStreamPackages = make([]*dxtypes.Package, 0)
		}
		// and
		for _, andDepPkgName := range pkg.DependsOn.And {
			if andDepPkg, ok := pkgMap[andDepPkgName]; ok {
				pkg.UpStreamPackages = append(pkg.UpStreamPackages, andDepPkg)

				if andDepPkg.DownStreamPackages == nil {
					andDepPkg.DownStreamPackages = make([]*dxtypes.Package, 0)
				}
				andDepPkg.DownStreamPackages = append(andDepPkg.DownStreamPackages, pkg)
			} else {
				// if not found, make a potential package
				pkg.UpStreamPackages = append(pkg.UpStreamPackages, &dxtypes.Package{
					Name:    andDepPkgName,
					Version: "*",
					DownStreamPackages: []*dxtypes.Package{
						pkg,
					},
					Potential: true,
				})
			}
		}
		// or
		for _, orDepPkgs := range pkg.DependsOn.Or {
			exist := false
			for _, orDepPkgName := range orDepPkgs {
				if orDepPkg, ok := pkgMap[orDepPkgName]; ok {
					pkg.UpStreamPackages = append(pkg.UpStreamPackages, orDepPkg)

					if orDepPkg.DownStreamPackages == nil {
						orDepPkg.DownStreamPackages = make([]*dxtypes.Package, 0)
					}
					orDepPkg.DownStreamPackages = append(orDepPkg.DownStreamPackages, pkg)
					exist = true
					break
				}
			}

			if !exist {
				// if not found, make a potential package
				pkg.UpStreamPackages = append(pkg.UpStreamPackages, &dxtypes.Package{
					Name:    strings.Join(orDepPkgs, "|"), // potential package name, splited by "|"
					Version: "*",
					DownStreamPackages: []*dxtypes.Package{
						pkg,
					},
					Potential: true,
				})
			}
		}
	}
}
