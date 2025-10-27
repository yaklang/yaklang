package analyzer

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
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

func parseVersion(version string) ([]int, error) {
	parts := strings.Split(version, ".")
	if len(parts) < 3 {
		return nil, errors.New("not semver version format")
	}
	v := make([]int, 3)
	for i := 0; i < 3; i++ {
		t, err := strconv.Atoi(parts[i])
		if err != nil {
			return nil, err
		}
		v[i] = t
	}
	return v, nil
}

func handlerSemverVersionRange(semverRange string) string {
	if len(semverRange) == 0 {
		return ""
	}
	if semverRange[0] == '~' {
		v, err := parseVersion(semverRange[1:])
		if err != nil {
			return semverRange
		}
		return fmt.Sprintf(">= %d.%d.%d && < %d.%d.%d", v[0], v[1], v[2], v[0], v[1]+1, 0)
	}

	if semverRange[0] == '^' {
		v, err := parseVersion(semverRange[1:])
		if err != nil {
			return semverRange
		}
		return fmt.Sprintf(">= %d.%d.%d && < %d.%d.%d", v[0], v[1], v[2], v[0]+1, 0, 0)
	}

	return semverRange
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

func makePotentialPkgs(pkgs []*dxtypes.Package) []*dxtypes.Package {
	potentialPkgs := make([]*dxtypes.Package, 0)
	pkgMaps := make(map[string]*dxtypes.Package)
	for _, pkg := range pkgs {
		// and
		for andDepPkgName, andDepVersion := range pkg.DependsOn.And {
			id := andDepPkgName + andDepVersion
			potentialPkg, ok := pkgMaps[id]
			if !ok {
				potentialPkg = &dxtypes.Package{
					Name:           andDepPkgName,
					Version:        andDepVersion,
					IsVersionRange: strings.ContainsAny(andDepVersion, "><*"),
					Potential:      true,
				}
				potentialPkgs = append(potentialPkgs, potentialPkg)
				pkgMaps[id] = potentialPkg
			}
			pkg.LinkDepend(potentialPkg)
		}
		// or
		for _, orDepPkgMap := range pkg.DependsOn.Or {
			orDepName := lo.MapToSlice(orDepPkgMap, func(name, _ string) string {
				return name
			})
			sort.Strings(orDepName)
			potentialName := strings.Join(orDepName, "|") // potential package name, splited by "|";
			orDepVersion := lo.Map(orDepName, func(name string, index int) string {
				return orDepPkgMap[name]
			})
			potentialVersion := strings.Join(orDepVersion, "|")
			id := potentialName + potentialVersion
			potentialPkg, ok := pkgMaps[id]
			if !ok {
				potentialPkg = &dxtypes.Package{
					Name:           potentialName,
					Version:        strings.Join(orDepVersion, "|"), // potential package version, splited by "|",
					IsVersionRange: true,
					Potential:      true,
				}
				potentialPkgs = append(potentialPkgs, potentialPkg)
				pkgMaps[id] = potentialPkg
			}

			pkg.LinkDepend(potentialPkg)
		}
	}

	return append(pkgs, potentialPkgs...)
}

func MergePackages(pkgs []*dxtypes.Package) []*dxtypes.Package {
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
	ret := make([]*dxtypes.Package, 0, len(pkgs))
	//将orpkg切分为多个普通包
	for _, pkg := range orPkgs {
		match := false
		names := strings.Split(pkg.Name, "|")
		versions := strings.Split(pkg.Version, "|")
		for i, name := range names {
			version := versions[i]
			// 通过pkgMaps判断orPkgs中存在的包
			plist, ok := pkgMaps[name]
			if !ok {
				continue
			}
			// 如果存在 则创建一个potential包
			match = true
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
				downp.LinkDepend(p)
			}
			//加入同名pkg的数组中 期待进行合并
			plist = append(plist, p)
			pkgMaps[name] = plist
		}
		if match {
			//修正上下游关系
			for _, downp := range pkg.DownStreamPackages {
				delete(downp.UpStreamPackages, pkg.Identifier())
				delete(pkg.UpStreamPackages, downp.Identifier())
			}
		} else {
			// 如果没有命中则保存这个或包不进行合并。
			ret = append(ret, pkg)
		}
	}

	// handler pkg list of same name, merge package that can be merged.
	for _, list := range pkgMaps {
		if len(list) == 1 {
			ret = append(ret, list[0])
			continue
		}

		// check
		merge := make(map[*dxtypes.Package]*dxtypes.Package)
		// O(n) -- O(n * lg(N))
		for i := 0; i < len(list); i++ {
			p := list[i]
			mergeToOther := false // can return
			// skip pkg in merge
			if _, ok := merge[p]; ok {
				continue
			}
			for j := i + 1; j < len(list); j++ {
				p2 := list[j]
				// skip pkg in merge
				if _, ok := merge[p2]; ok {
					continue
				}
				// check can merge
				ret := dxtypes.CanMerge(p, p2)
				if ret == 0 {
					// don't merge
					// pass
				} else if ret == -1 {
					// merge p to p2
					merge[p] = p2
					// the p don't return! p is *merge to other*
					mergeToOther = true
				} else if ret == 1 {
					// merge p2 to p // p is *merge other*
					merge[p2] = p
				}
			}
			// only three type in pakcage of list:
			// 		* not merge  * merge to other  * merge other
			// we only return *not merge* and *merge other* packages
			if mergeToOther {
				continue
			}
			ret = append(ret, p)
		}

		for p2, p := range merge {
			p.Merge(p2)
		}
	}
	return ret
}
func DrawPackagesDOT(pkgs []*dxtypes.Package) {
	// g := dot.NewGraph(dot.Directed)
	// // rankdir=LR,splines=ortho,concentrate=true
	// g.Attr("rankdir", "LR")
	// g.Attr("concentrate", "true")
	// nodes := make(map[string]dot.Node, len(pkgs))
	// for _, pkg := range pkgs {
	// 	label := fmt.Sprintf("%s-%s", pkg.Name, html.EscapeString(pkg.Version))
	// 	// label += fmt.Sprintf(`<br/><FONT POINT-SIZE="10">License: %s</FONT>`, strings.Join(pkg.License, ", "))
	// 	// label += fmt.Sprintf(`<br/><FONT POINT-SIZE="10">Verification: %s</FONT>`, pkg.Verification)
	// 	label += fmt.Sprintf(`<br/><FONT POINT-SIZE="10">Potential: %v</FONT>`, pkg.Potential)
	// 	node := g.Node(pkg.Identifier()).Attr("label", dot.HTML(label)).Attr("shape", "box")

	// 	// node := g.Node(pkg.Identifier()).Attr("label", dot.HTML(label))
	// 	nodes[pkg.Identifier()] = node
	// }
	// edgeExistMap := make(map[string]struct{})
	// for _, pkg := range pkgs {
	// 	for _, upStreamPkg := range pkg.UpStreamPackages {
	// 		if _, ok := edgeExistMap[fmt.Sprintf("%s-%s", pkg.Identifier(), upStreamPkg.Identifier())]; ok {
	// 			continue
	// 		}
	// 		g.Edge(nodes[pkg.Identifier()], nodes[upStreamPkg.Identifier()])
	// 		edgeExistMap[fmt.Sprintf("%s-%s", pkg.Identifier(), upStreamPkg.Identifier())] = struct{}{}
	// 	}
	// 	for _, downStreamPkg := range pkg.DownStreamPackages {
	// 		if _, ok := edgeExistMap[fmt.Sprintf("%s-%s", downStreamPkg.Identifier(), pkg.Identifier())]; ok {
	// 			continue
	// 		}
	// 		g.Edge(nodes[downStreamPkg.Identifier()], nodes[pkg.Identifier()])
	// 		edgeExistMap[fmt.Sprintf("%s-%s", downStreamPkg.Identifier(), pkg.Identifier())] = struct{}{}
	// 	}
	// }
	// f, err := os.CreateTemp("", "temp-dot")
	// defer f.Close()
	// if err != nil {
	// 	return
	// }
	// _, err = io.Copy(f, strings.NewReader(g.String()))
	// if err != nil {
	// 	return
	// }
	// uuid, err := uuid.New()
	// if err != nil {
	// 	return
	// }
	// pngPath := filepath.Join(os.TempDir(), uuid.String()+".svg")
	// cmd := exec.Command("C:\\Users\\ad\\scoop\\shims\\dot.exe", "-T", "svg", fmt.Sprintf("-o%s", pngPath), f.Name())
	// err = cmd.Run()
	// if err != nil {
	// 	log.Errorf("dot: %v", err)
	// 	return
	// }
	// cmd = exec.Command("C:\\Windows\\explorer.exe", pngPath)
	// err = cmd.Run()
	// if err != nil {
	// 	log.Errorf("explorer: %v", err)
	// 	return
	// }
}
