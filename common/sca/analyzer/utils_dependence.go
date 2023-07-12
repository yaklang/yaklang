package analyzer

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

func fastVersionCompare(old, new string) bool {
	if old == "*" {
		return true
	}
	if strings.ContainsAny(old, "><") && !strings.Contains(new, "><") {
		// old is version range, new is definite version
		return true
	}

	return false
}

func handleDependsOn(pkgs []*dxtypes.Package, provides map[string]*dxtypes.Package) {
	for _, pkg := range pkgs {
		// e.g. "libc.so.6()(64bit)" => "glibc-2.12-1.212.el6.x86_64"
		newAnd := make(map[string]string)
		for depName, depVersion := range pkg.DependsOn.And {
			if p, ok := provides[depName]; ok {
				newAnd[p.Name] = p.Version
			} else if oldVersion, ok := newAnd[depName]; !ok || fastVersionCompare(oldVersion, depVersion) {
				newAnd[depName] = depVersion
			}
		}

		pkg.DependsOn.And = newAnd

		if len(pkg.DependsOn.And) == 0 {
			pkg.DependsOn.And = nil
		}
	}
}

func linkUpSteamAndDownStream(pkgs []*dxtypes.Package) []*dxtypes.Package {
	potentialPkgs := make([]*dxtypes.Package, 0)

	pkgMap := lo.SliceToMap(pkgs, func(item *dxtypes.Package) (string, *dxtypes.Package) {
		return item.Name, item
	})

	for _, pkg := range pkgs {

		if pkg.UpStreamPackages == nil {
			pkg.UpStreamPackages = make(map[string]*dxtypes.Package)
		}
		// and
		for andDepPkgName, andDepVersion := range pkg.DependsOn.And {
			if andDepPkg, ok := pkgMap[andDepPkgName]; ok {
				// pkg.UpStreamPackages = append(pkg.UpStreamPackages, andDepPkg)
				pkg.UpStreamPackages[andDepPkgName] = andDepPkg

				if andDepPkg.DownStreamPackages == nil {
					andDepPkg.DownStreamPackages = make(map[string]*dxtypes.Package)
				}
				// andDepPkg.DownStreamPackages = append(andDepPkg.DownStreamPackages, pkg)
				andDepPkg.DownStreamPackages[pkg.Name] = pkg
			} else {
				// if not found, make a potential package
				potentialPkg := &dxtypes.Package{
					Name:           andDepPkgName,
					Version:        andDepVersion,
					IsVersionRange: true,
					DownStreamPackages: map[string]*dxtypes.Package{
						pkg.Name: pkg,
					},
					Potential: true,
				}
				potentialPkgs = append(potentialPkgs, potentialPkg)
				pkgMap[potentialPkg.Name] = potentialPkg
				pkg.UpStreamPackages[andDepPkgName] = potentialPkg
			}
		}
		// or
		for _, orDepPkgMap := range pkg.DependsOn.Or {
			exist := false
			for orDepPkgName := range orDepPkgMap {
				if orDepPkg, ok := pkgMap[orDepPkgName]; ok {
					// pkg.UpStreamPackages = append(pkg.UpStreamPackages, orDepPkg)
					pkg.UpStreamPackages[orDepPkgName] = orDepPkg

					if orDepPkg.DownStreamPackages == nil {
						orDepPkg.DownStreamPackages = make(map[string]*dxtypes.Package)
					}
					// orDepPkg.DownStreamPackages = append(orDepPkg.DownStreamPackages, pkg)
					orDepPkg.DownStreamPackages[pkg.Name] = pkg
					exist = true
					break
				}
			}

			if !exist {
				// if not found, make a potential package
				orDepName := ""
				orDepVersion := ""
				for name, version := range orDepPkgMap {
					orDepName += fmt.Sprintf("%s|", name)
					orDepVersion += fmt.Sprintf("%s|", version)
				}
				orDepName = strings.TrimSuffix(orDepName, "|")
				orDepVersion = strings.TrimSuffix(orDepVersion, "|")

				potentialPkg := &dxtypes.Package{
					Name:           orDepName,    // potential package name, splited by "|";
					Version:        orDepVersion, // potential package version, splited by "|",
					IsVersionRange: true,
					DownStreamPackages: map[string]*dxtypes.Package{
						pkg.Name: pkg,
					},
					Potential: true,
				}
				potentialPkgs = append(potentialPkgs, potentialPkg)
				pkgMap[potentialPkg.Name] = potentialPkg
				// pkg.UpStreamPackages = append(pkg.UpStreamPackages, potentialPkg)
				pkg.UpStreamPackages[potentialPkg.Name] = potentialPkg
			}
		}
	}

	// append potential packages
	return append(pkgs, potentialPkgs...)
}
