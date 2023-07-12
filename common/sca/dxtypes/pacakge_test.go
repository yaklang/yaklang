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
		FromFile: []string{
			"/path/pa/file",
		},
		FromAnalyzer: []string{
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
		FromFile: []string{
			"/path/padown/file",
		},
		FromAnalyzer: []string{
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
		FromFile: []string{
			"/path/pb/file",
		},
		FromAnalyzer: []string{
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
		FromFile: []string{
			"/path/pbdown/file",
		},
		FromAnalyzer: []string{
			"pb-analyzer",
		},
		Verification:       "",
		License:            []string{},
		UpStreamPackages:   map[string]*Package{},
		DownStreamPackages: map[string]*Package{},
	}
	pb.DownStreamPackages[pb_down.Name] = &pb_down
	pb_down.UpStreamPackages[pb.Name] = &pb

	// Merge(&pa, &pb)
	pa.Merge(&pb)
	// fmt.Printf("%s", pa)
	if len(pa.FromAnalyzer) != 2 {
		t.Fatalf("fromAnalyzer len error: %v", pa.FromAnalyzer)
	}
	if slices.CompareFunc(pa.FromAnalyzer, []string{"pa-analyzer", "pb-analyzer"}, strings.Compare) != 0 {
		t.Fatalf("fromAnalyzer error: %v", pa.FromAnalyzer)
	}
	if len(pa.FromFile) != 2 {
		t.Fatal("fromFile len error")
	}
	if slices.CompareFunc(pa.FromFile, []string{"/path/pa/file", "/path/pb/file"}, strings.Compare) != 0 {
		t.Fatalf("fromFile error: %v", pa.FromFile)
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
