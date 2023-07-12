package dxtypes

import (
	"sort"
	"strings"
	"testing"

	"golang.org/x/exp/slices"
)

func TestPackageMerge(t *testing.T) {
	pa := Package{
		Name:    "pa",
		Version: "0.0.1",
		fromFile: []string{
			"/path/pa/file",
		},
		fromAnalyzer: []string{
			"pa-analyzer",
		},
		Verification:       "",
		License:            []string{},
		UpStreamPackages:   map[string]*Package{},
		DownStreamPackages: map[string]*Package{},
	}

	pa_down := Package{
		Name:    "pa-down",
		Version: "0.0.2",
		fromFile: []string{
			"/path/padown/file",
		},
		fromAnalyzer: []string{
			"pa-analyzer",
		},
		Verification:       "",
		License:            []string{},
		UpStreamPackages:   map[string]*Package{},
		DownStreamPackages: map[string]*Package{},
	}
	pa.DownStreamPackages[pa_down.Name] = &pa_down
	pa_down.UpStreamPackages[pa.Name] = &pa

	pb := Package{
		Name:    "pa",
		Version: "0.0.1",
		fromFile: []string{
			"/path/pb/file",
		},
		fromAnalyzer: []string{
			"pb-analyzer",
		},
		Verification:       "",
		License:            []string{},
		UpStreamPackages:   map[string]*Package{},
		DownStreamPackages: map[string]*Package{},
	}
	pb_down := Package{
		Name:    "pb-down",
		Version: "0.0.3",
		fromFile: []string{
			"/path/pbdown/file",
		},
		fromAnalyzer: []string{
			"pb-analyzer",
		},
		Verification:       "",
		License:            []string{},
		UpStreamPackages:   map[string]*Package{},
		DownStreamPackages: map[string]*Package{},
	}
	pb.DownStreamPackages[pb_down.Name] = &pb_down
	pb_down.UpStreamPackages[pb.Name] = &pb

	// PackageMerge(&pa, &pb)
	pa.PackageMerge(&pb)
	// fmt.Printf("%s", pa)
	if len(pa.fromAnalyzer) != 2 {
		t.Fatal("fromAnalyzer len error")
	}
	if slices.CompareFunc(pa.fromAnalyzer, []string{"pa-analyzer", "pb-analyzer"}, strings.Compare) != 0 {
		t.Fatalf("fromAnalyzer error: %v", pa.fromAnalyzer)
	}
	if len(pa.fromFile) != 2 {
		t.Fatal("fromFile len error")
	}
	if slices.CompareFunc(pa.fromFile, []string{"/path/pa/file", "/path/pb/file"}, strings.Compare) != 0 {
		t.Fatalf("fromFile error: %v", pa.fromFile)
	}

	pkgname := []string{}
	for _, pkg := range pa.DownStreamPackages {
		if _, ok := pkg.UpStreamPackages[pa.Name]; !ok {
			t.Fatalf("downSteram not link to pa %s", pkg)
		}
		pkgname = append(pkgname, pkg.Name)
	}

	sort.Strings(pkgname)
	if len(pkgname) != 2 {
		t.Fatal("fromFile len error")
	}
	if slices.CompareFunc(pkgname, []string{"pa-down", "pb-down"}, strings.Compare) != 0 {
		t.Fatalf("pkgname error: %v", pkgname)
	}
}
