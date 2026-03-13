package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

type goListError struct {
	Err string
}

type goPackage struct {
	Dir        string
	ImportPath string
	Standard   bool

	GoFiles      []string
	CgoFiles     []string
	CFiles       []string
	CXXFiles     []string
	MFiles       []string
	HFiles       []string
	FFiles       []string
	SFiles       []string
	SwigFiles    []string
	SwigCXXFiles []string
	SysoFiles    []string
	EmbedFiles   []string

	Error *goListError
}

func main() {
	var (
		pkg  = flag.String("pkg", "", "target package (e.g. ./path/to/pkg)")
		dst  = flag.String("dst", "", "destination directory (must be empty or non-existent)")
		tags = flag.String("tags", "", "optional build tags for go list/go build (comma-separated)")
	)
	flag.Parse()

	if strings.TrimSpace(*pkg) == "" {
		fmt.Fprintln(os.Stderr, "missing --pkg")
		os.Exit(2)
	}
	if strings.TrimSpace(*dst) == "" {
		fmt.Fprintln(os.Stderr, "missing --dst")
		os.Exit(2)
	}

	moduleRoot, err := moduleRootDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	dstDir := filepath.Clean(*dst)
	if err := ensureEmptyDir(dstDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := copyFile(filepath.Join(moduleRoot, "go.mod"), filepath.Join(dstDir, "go.mod")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := copyFile(filepath.Join(moduleRoot, "go.sum"), filepath.Join(dstDir, "go.sum")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	pkgs, err := goListDeps(moduleRoot, *pkg, *tags)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	type copyItem struct {
		src string
		dst string
	}
	seen := make(map[string]struct{}, 4096)
	var items []copyItem

	for _, p := range pkgs {
		if p == nil || p.Standard || p.Dir == "" {
			continue
		}
		pdir := filepath.Clean(p.Dir)
		if !strings.HasPrefix(pdir, moduleRoot+string(filepath.Separator)) && pdir != moduleRoot {
			continue
		}

		relDir, relErr := filepath.Rel(moduleRoot, pdir)
		if relErr != nil {
			fmt.Fprintln(os.Stderr, utils.Errorf("gomodsrc: rel %s -> %s failed: %v", moduleRoot, pdir, relErr))
			os.Exit(1)
		}

		files := collectPackageFiles(p)
		for _, f := range files {
			f = filepath.Clean(f)
			if f == "." || f == "" {
				continue
			}
			srcPath := filepath.Join(pdir, f)
			dstPath := filepath.Join(dstDir, relDir, f)
			if _, ok := seen[dstPath]; ok {
				continue
			}
			seen[dstPath] = struct{}{}
			items = append(items, copyItem{src: srcPath, dst: dstPath})
		}
	}

	// Make output deterministic for reproducibility.
	sort.Slice(items, func(i, j int) bool { return items[i].dst < items[j].dst })

	var copied int
	for _, it := range items {
		if err := copyFile(it.src, it.dst); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		copied++
	}

	fmt.Printf("gomodsrc: wrote %d files into %s\n", copied, dstDir)
}

func collectPackageFiles(p *goPackage) []string {
	var out []string
	out = append(out, p.GoFiles...)
	out = append(out, p.CgoFiles...)
	out = append(out, p.CFiles...)
	out = append(out, p.CXXFiles...)
	out = append(out, p.MFiles...)
	out = append(out, p.HFiles...)
	out = append(out, p.FFiles...)
	out = append(out, p.SFiles...)
	out = append(out, p.SwigFiles...)
	out = append(out, p.SwigCXXFiles...)
	out = append(out, p.SysoFiles...)
	out = append(out, p.EmbedFiles...)
	return out
}

func moduleRootDir() (string, error) {
	cmd := exec.Command("go", "env", "GOMOD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", utils.Errorf("gomodsrc: go env GOMOD failed: %v\n%s", err, out)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		return "", utils.Errorf("gomodsrc: not in a Go module (go env GOMOD=%q)", gomod)
	}
	return filepath.Dir(gomod), nil
}

func ensureEmptyDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return utils.Errorf("gomodsrc: mkdir %s failed: %v", dir, err)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return utils.Errorf("gomodsrc: readdir %s failed: %v", dir, err)
	}
	if len(ents) != 0 {
		return utils.Errorf("gomodsrc: dst is not empty: %s", dir)
	}
	return nil
}

func goListDeps(moduleRoot, pkg, tags string) ([]*goPackage, error) {
	args := []string{"list", "-deps", "-json"}
	if strings.TrimSpace(tags) != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, pkg)

	cmd := exec.Command("go", args...)
	cmd.Dir = moduleRoot

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, utils.Errorf("gomodsrc: go list failed: %v\n%s", err, stderr.String())
	}

	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	var out []*goPackage
	for {
		var p goPackage
		if err := dec.Decode(&p); err != nil {
			if err == io.EOF {
				break
			}
			return nil, utils.Errorf("gomodsrc: decode go list output failed: %v", err)
		}
		if p.Error != nil && strings.TrimSpace(p.Error.Err) != "" {
			return nil, utils.Errorf("gomodsrc: go list package %s failed: %s", p.ImportPath, p.Error.Err)
		}
		out = append(out, &p)
	}
	return out, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return utils.Errorf("gomodsrc: read %s failed: %v", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return utils.Errorf("gomodsrc: mkdir %s failed: %v", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return utils.Errorf("gomodsrc: write %s failed: %v", dst, err)
	}
	return nil
}
