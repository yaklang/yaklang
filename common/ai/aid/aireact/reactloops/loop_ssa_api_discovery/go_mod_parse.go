package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/yaklang/yaklang/common/utils"
)

const maxGoModRequiresInReport = 64

type goModSummary struct {
	ModulePath string   `json:"module_path"`
	GoVersion  string   `json:"go_version,omitempty"`
	Requires   []string `json:"requires,omitempty"`
}

func parseGoModSummary(modPath string) (*goModSummary, error) {
	data, err := os.ReadFile(modPath)
	if err != nil {
		return nil, err
	}
	parsed, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, err
	}
	out := &goModSummary{}
	if parsed.Module != nil {
		out.ModulePath = strings.TrimSpace(parsed.Module.Mod.Path)
	}
	if parsed.Go != nil {
		out.GoVersion = strings.TrimSpace(parsed.Go.Version)
	}
	for _, req := range parsed.Require {
		if req == nil || req.Mod.Path == "" {
			continue
		}
		out.Requires = append(out.Requires, req.Mod.Path)
		if len(out.Requires) >= maxGoModRequiresInReport {
			break
		}
	}
	return out, nil
}

func enrichPreanalysisFromGoMod(rep *APIPreanalysisReport, root string) {
	if rep == nil || root == "" {
		return
	}
	modPath := filepath.Join(root, "go.mod")
	if _, err := os.Stat(modPath); err != nil {
		return
	}
	summary, err := parseGoModSummary(modPath)
	if err != nil {
		rep.Warnings = append(rep.Warnings, "go.mod parse: "+utils.ShrinkString(err.Error(), 200))
		return
	}
	if summary.ModulePath != "" {
		rep.Modules = append(rep.Modules, struct {
			Name   string `json:"name"`
			RelDir string `json:"rel_dir"`
			Kind   string `json:"kind,omitempty"`
		}{Name: summary.ModulePath, RelDir: ".", Kind: "go_module"})
	}
	for _, req := range summary.Requires {
		rep.Modules = append(rep.Modules, struct {
			Name   string `json:"name"`
			RelDir string `json:"rel_dir"`
			Kind   string `json:"kind,omitempty"`
		}{Name: req, RelDir: ".", Kind: "go_require"})
	}
}

func buildDependenciesPayloadFromRoot(root string) map[string]any {
	manifests := []map[string]string{}
	for _, name := range []string{"go.mod", "package.json", "pom.xml", "composer.json", "requirements.txt"} {
		p := filepath.Join(root, name)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		row := map[string]string{"file": name, "ecosystem": name}
		if name == "go.mod" {
			if summary, err := parseGoModSummary(p); err == nil && summary != nil {
				if summary.ModulePath != "" {
					row["module"] = summary.ModulePath
				}
				if summary.GoVersion != "" {
					row["go_version"] = summary.GoVersion
				}
				if len(summary.Requires) > 0 {
					row["require_count"] = utils.InterfaceToString(len(summary.Requires))
				}
			}
		}
		manifests = append(manifests, row)
	}
	return map[string]any{"manifests": manifests}
}
