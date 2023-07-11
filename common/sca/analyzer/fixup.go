package analyzer

import (
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

func consolidateDependencies(pkgs []dxtypes.Package) {
	potentialPkgs := make([]dxtypes.Package, 0)

	pkgMap := lo.SliceToMap(pkgs, func(item dxtypes.Package) (string, *dxtypes.Package) {
		return item.Name, &item
	})

	for i := range pkgs {
		pkg := &pkgs[i]

		if pkg.UpStreamPackages == nil {
			pkg.UpStreamPackages = make([]*dxtypes.Package, 0)
		}
		// and
		for andDepPkgName := range pkg.DependsOn.And {
			if andDepPkg, ok := pkgMap[andDepPkgName]; ok {
				pkg.UpStreamPackages = append(pkg.UpStreamPackages, andDepPkg)

				if andDepPkg.DownStreamPackages == nil {
					andDepPkg.DownStreamPackages = make([]*dxtypes.Package, 0)
				}
				andDepPkg.DownStreamPackages = append(andDepPkg.DownStreamPackages, pkg)
			} else {
				// if not found, make a potential package
				potentialPkg := &dxtypes.Package{
					Name:    andDepPkgName,
					Version: "*",
					DownStreamPackages: []*dxtypes.Package{
						pkg,
					},
					Potential: true,
				}
				potentialPkgs = append(potentialPkgs, *potentialPkg)
				pkg.UpStreamPackages = append(pkg.UpStreamPackages, potentialPkg)
			}
		}
		// or
		for _, orDepPkgMap := range pkg.DependsOn.Or {
			exist := false
			for orDepPkgName := range orDepPkgMap {
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
				potentialPkg := &dxtypes.Package{
					Name: strings.Join(
						lo.MapToSlice(orDepPkgMap, func(name string, _ string) string {
							return name
						}),
						"|"), // potential package name, splited by "|"
					Version: "*",
					DownStreamPackages: []*dxtypes.Package{
						pkg,
					},
					Potential: true,
				}
				potentialPkgs = append(potentialPkgs, *potentialPkg)
				pkg.UpStreamPackages = append(pkg.UpStreamPackages, potentialPkg)
			}
		}
	}

	// append potential packages
	pkgs = append(pkgs, potentialPkgs...)
}
