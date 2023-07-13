package analyzer

import (
	"sort"
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

func linkStream(down, up *dxtypes.Package) {
	if up.DownStreamPackages == nil {
		up.DownStreamPackages = make(map[string]*dxtypes.Package)
	}
	up.DownStreamPackages[down.Identifier()] = down
	if down.UpStreamPackages == nil {
		down.UpStreamPackages = make(map[string]*dxtypes.Package)
	}
	down.UpStreamPackages[up.Identifier()] = up
}

func linkPackages(pkgs []*dxtypes.Package) []*dxtypes.Package {
	potentialPkgs := make([]*dxtypes.Package, 0)

	pkgMap := lo.SliceToMap(pkgs, func(item *dxtypes.Package) (string, *dxtypes.Package) {
		return item.Name, item
	})

	for _, pkg := range pkgs {

		// and
		for andDepPkgName, andDepVersion := range pkg.DependsOn.And {
			if andDepPkg, ok := pkgMap[andDepPkgName]; ok {
				linkStream(pkg, andDepPkg)
			} else {
				// if not found, make a potential package
				potentialPkg := &dxtypes.Package{
					Name:           andDepPkgName,
					Version:        andDepVersion,
					IsVersionRange: true,
					Potential:      true,
				}
				potentialPkgs = append(potentialPkgs, potentialPkg)
				pkgMap[potentialPkg.Name] = potentialPkg
				linkStream(pkg, potentialPkg)
			}
		}
		// or
		for _, orDepPkgMap := range pkg.DependsOn.Or {
			exist := false
			for orDepPkgName := range orDepPkgMap {
				if orDepPkg, ok := pkgMap[orDepPkgName]; ok {
					linkStream(pkg, orDepPkg)
					exist = true
					break
				}
			}

			if !exist {
				// if not found, make a potential package
				orDepName := make([]string, 0, len(orDepPkgMap))
				for name := range orDepPkgMap {
					orDepName = append(orDepName, name)
				}
				sort.Strings(orDepName)
				orDepVersion := lo.Map(orDepName, func(name string, index int) string {
					return orDepPkgMap[name]
				})

				potentialPkg := &dxtypes.Package{
					Name:           strings.Join(orDepName, "|"),    // potential package name, splited by "|";
					Version:        strings.Join(orDepVersion, "|"), // potential package version, splited by "|",
					IsVersionRange: true,
					Potential:      true,
				}
				potentialPkgs = append(potentialPkgs, potentialPkg)
				pkgMap[potentialPkg.Name] = potentialPkg
				linkStream(pkg, potentialPkg)
			}
		}
	}

	// append potential packages
	return append(pkgs, potentialPkgs...)
}

func mergePackages(pkgs []*dxtypes.Package) []*dxtypes.Package {
	pkgMaps := make(map[string][]*dxtypes.Package) // name -> []packages
	orPkgs := make([]*dxtypes.Package, 0)
	for _, pkg := range pkgs {
		if strings.Contains(pkg.Name, "|") {
			orPkgs = append(orPkgs, pkg)
			continue
		}
		plist, ok := pkgMaps[pkg.Name]
		if !ok {
			plist = make([]*dxtypes.Package, 0)
		}
		plist = append(plist, pkg)
		pkgMaps[pkg.Name] = plist
	}
	//将orpkg切分为多个普通包
	for _, pkg := range orPkgs {
		names := strings.Split(pkg.Name, "|")
		versions := strings.Split(pkg.Version, "|")
		for i, name := range names {
			version := versions[i]
			// 通过pkgMaps判断orPkgs中存在的包
			plist, ok := pkgMaps[name]
			if !ok {
				continue
			}
			p := &dxtypes.Package{
				Name:           name,
				Version:        version,
				IsVersionRange: strings.ContainsAny(version, "><*"),
				FromFile:       pkg.FromFile,
				FromAnalyzer:   pkg.FromAnalyzer,
				Potential:      true,
			}
			//修正上下游关系
			for _, downp := range pkg.DownStreamPackages {
				linkStream(downp, p)
			}
			//加入同名pkg的数组中
			plist = append(plist, p)
			pkgMaps[name] = plist
		}
		//修正上下游关系
		for _, downp := range pkg.DownStreamPackages {
			delete(downp.UpStreamPackages, pkg.Identifier())
			delete(pkg.UpStreamPackages, downp.Identifier())
		}
	}

	ret := make([]*dxtypes.Package, 0, len(pkgs))
	// handler pkg list of same name, merge package that can be merged.
	for _, list := range pkgMaps {
		if len(list) == 1 {
			ret = append(ret, list[0])
			continue
		}

		p := list[0]
		for _, p2 := range list {
			if p2 == p {
				continue
			}
			// match
			if p.CanMerge(p2) {
				p.Merge(p2)
			} else {
				ret = append(ret, p2)
			}
		}
		ret = append(ret, p)
	}
	return ret
}
