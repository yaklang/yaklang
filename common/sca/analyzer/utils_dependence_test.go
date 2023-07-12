package analyzer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/dxtypes"
)

var (
	pkgMaps = make(map[string][]*dxtypes.Package)
	pa1     = newPackage("pa1", "0.0.3", "pa")
	pa22    = newPackage("pa22", "0.0.3", "pa")
	pa21    = newPackage("pa21", "0.0.3", "pa")
	pa3     = newPackage("pa3", "0.0.3", "pa")
	pa3b    = newPackage("pb3", "0.0.2", "pa")

	pb1 = newPackage("pb1", "0.0.3", "pb")
	pb2 = newPackage("pb2", "0.0.3", "pb")
	pb3 = newPackage("pb3", "0.0.3", "pb")

	pc1  = newPackage("pc1", "0.0.3", "pc")
	pc2  = newPackage("pc2", "0.0.3", "pc")
	pc3  = newPackage("pc3", "0.0.3", "pc")
	pcor = newPackage("pa1|pb1|pb2", "0.0.3|0.0.3|0.0.3", "pc")

	pd1  = newPackage("pd1", "0.0.3", "pd")
	pd2  = newPackage("pd2", "0.0.5", "pd")
	pd3  = newPackage("pd3", "0.0.3", "pd")
	pdor = newPackage("pa1|pb1|pb2", ">0.0.2|>=0.0.3|<0.0.4", "pd")
	//pdor = newPackage("pa1|pb1|pb2", "0.0.2|0.0.3|0.0.4", "pd")
)

func newPackage(name, version, prefix string) *dxtypes.Package {
	p := &dxtypes.Package{
		Name:               name,
		Version:            version,
		IsVersionRange:     strings.ContainsAny(version, "<>="),
		FromFile:           []string{fmt.Sprintf("/path/%s/file", prefix)},
		FromAnalyzer:       []string{fmt.Sprintf("%s-analyzer", prefix)},
		Verification:       "",
		License:            nil,
		UpStreamPackages:   make(map[string]*dxtypes.Package),
		DownStreamPackages: make(map[string]*dxtypes.Package),
	}
	//pkgs = append(pkgs, p)
	list, ok := pkgMaps[prefix]
	if !ok {
		list = make([]*dxtypes.Package, 0)
	}
	list = append(list, p)
	pkgMaps[prefix] = list
	return p
}

//	func DrawPackagesDOT(pkgs []*dxtypes.Package, name string) {
//		g := dot.NewGraph(dot.Directed)
//		nodes := make(map[string]dot.Node, len(pkgs))
//		for _, pkg := range pkgs {
//			label := fmt.Sprintf("%s-%s", pkg.Name, html.EscapeString(pkg.Version))
//			label += fmt.Sprintf(`<br/><FONT POINT-SIZE="10">License: %s</FONT>`, strings.Join(pkg.License, ", "))
//			label += fmt.Sprintf(`<br/><FONT POINT-SIZE="10">Verification: %s</FONT>`, pkg.Verification)
//			label += fmt.Sprintf(`<br/><FONT POINT-SIZE="10">Indirect: %v</FONT>`, pkg.Indirect)
//			label += fmt.Sprintf(`<br/><FONT POINT-SIZE="10">Potential: %v</FONT>`, pkg.Potential)
//			node := g.Node(pkg.Identifier()).Attr("label", dot.HTML(label))
//			nodes[pkg.Identifier()] = node
//		}
//		edgeExistMap := make(map[string]struct{})
//		for _, pkg := range pkgs {
//			for _, upStreamPkg := range pkg.UpStreamPackages {
//				if _, ok := edgeExistMap[fmt.Sprintf("%s-%s", pkg.Identifier(), upStreamPkg.Identifier())]; ok {
//					continue
//				}
//				g.Edge(nodes[pkg.Identifier()], nodes[upStreamPkg.Identifier()])
//				edgeExistMap[fmt.Sprintf("%s-%s", pkg.Identifier(), upStreamPkg.Identifier())] = struct{}{}
//			}
//			for _, downStreamPkg := range pkg.DownStreamPackages {
//				if _, ok := edgeExistMap[fmt.Sprintf("%s-%s", downStreamPkg.Identifier(), pkg.Identifier())]; ok {
//					continue
//				}
//				g.Edge(nodes[downStreamPkg.Identifier()], nodes[pkg.Identifier()])
//				edgeExistMap[fmt.Sprintf("%s-%s", downStreamPkg.Identifier(), pkg.Identifier())] = struct{}{}
//			}
//		}
//		f, err := os.CreateTemp("", "temp-dot")
//		defer f.Close()
//		if err != nil {
//			return
//		}
//		_, err = io.Copy(f, strings.NewReader(g.String()))
//		if err != nil {
//			return
//		}
//		pngPath := filepath.Join(os.TempDir(), name)
//		cmd := exec.Command("C:\\Users\\ad\\scoop\\shims\\dot.exe", "-T", "png", fmt.Sprintf("-o%s", pngPath), f.Name())
//		err = cmd.Run()
//		if err != nil {
//			log.Errorf("dot: %v", err)
//			return
//		}
//		cmd = exec.Command("explorer.exe", pngPath)
//		err = cmd.Run()
//		if err != nil {
//			log.Errorf("explorer: %v", err)
//			return
//		}
//	}
func init() {
	// pa1 -> pa22 -> pa3
	//     -> pa21
	linkStream(pa1, pa22)
	linkStream(pa1, pa21)
	linkStream(pa22, pa3)
	linkStream(pa22, pa3b)

	// pb1 -> pb2 -> pb3
	linkStream(pb1, pb2)
	linkStream(pb2, pb3)

	// pc1 -> pc2 -> pc3
	//     -> pa1|pc4|pb2
	linkStream(pc1, pc2)
	linkStream(pc2, pc3)
	linkStream(pc1, pcor)

	// pd1 -> pd2 -> pd3
	//     -> pa1|pb1|pb2
	linkStream(pd1, pd2)
	linkStream(pd2, pd3)
	linkStream(pd1, pdor)
}

func TestMergePackagesNormal(t *testing.T) {
	pkgs := make([]*dxtypes.Package, 0)
	pkgs = append(pkgs, pkgMaps["pa"]...)
	pkgs = append(pkgs, pkgMaps["pb"]...)
	// DrawPackagesDOT(pkgs, "org.png")

	// pb2 == pa22
	pb2.Name = pa22.Name
	ret := mergePackages(pkgs)
	// DrawPackagesDOT(ret, "ret.png")
	_ = ret
}

// TODO:
func TestMergePackagesVErsionRange(t *testing.T) {

}

func TestMergePackagesOrPackage(t *testing.T) {
	pkgs := make([]*dxtypes.Package, 0)
	pkgs = append(pkgs, pkgMaps["pa"]...)
	pkgs = append(pkgs, pkgMaps["pb"]...)
	pkgs = append(pkgs, pkgMaps["pc"]...)
	// DrawPackagesDOT(pkgs, "org.png")
	ret := mergePackages(pkgs)
	_ = ret
	// DrawPackagesDOT(ret, "ret.png")
}

func TestMergePackagesOrPackageVersionRange(t *testing.T) {
	pkgs := make([]*dxtypes.Package, 0)
	pkgs = append(pkgs, pkgMaps["pa"]...)
	pkgs = append(pkgs, pkgMaps["pb"]...)
	pkgs = append(pkgs, pkgMaps["pd"]...)
	// DrawPackagesDOT(pkgs, "org.png")
	ret := mergePackages(pkgs)
	// DrawPackagesDOT(ret, "ret.png")
	_ = ret
}
