package yakit

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaproject"
)

func isRemoteCodeSourceURL(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "git@") || strings.HasPrefix(s, "ssh://")
}

func firstLocalCodeSourcePath(candidates ...string) string {
	for _, p := range candidates {
		p = strings.TrimSpace(p)
		if p == "" || isRemoteCodeSourceURL(p) {
			continue
		}
		return p
	}
	return ""
}

func localPathFromCompileConfigJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	cfg, err := ssaconfig.New(ssaconfig.ModeAll, ssaconfig.WithJsonRawConfig([]byte(raw)))
	if err != nil || cfg == nil {
		return ""
	}
	return firstLocalCodeSourcePath(cfg.GetCodeSourceLocalFile(), cfg.GetCodeSourceLocalFileOrURL())
}

func localPathFromSSAProjectID(projectID uint64) string {
	if projectID == 0 {
		return ""
	}
	proj, err := ssaproject.LoadSSAProjectByID(uint(projectID))
	if err != nil || proj == nil {
		return ""
	}
	if p := firstLocalCodeSourcePath(proj.URL); p != "" {
		return p
	}
	cfg, err := proj.GetConfig()
	if err != nil || cfg == nil {
		return ""
	}
	return firstLocalCodeSourcePath(cfg.GetCodeSourceLocalFile(), cfg.GetCodeSourceLocalFileOrURL())
}

func localPathFromSSAProjectName(projectName string) string {
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return ""
	}
	proj, err := ssaproject.LoadSSAProjectByName(projectName)
	if err != nil || proj == nil {
		return ""
	}
	if p := firstLocalCodeSourcePath(proj.URL); p != "" {
		return p
	}
	cfg, err := proj.GetConfig()
	if err != nil || cfg == nil {
		return ""
	}
	return firstLocalCodeSourcePath(cfg.GetCodeSourceLocalFile(), cfg.GetCodeSourceLocalFileOrURL())
}

// ResolveCodeSourceLocalPath maps an SSA program_name (or a path whose last
// component is a program_name) to the project's local source directory.
// IRify "获取文件项目" / file_monitor often pass program_name as a file:// path.
func ResolveCodeSourceLocalPath(nameOrPath string) string {
	nameOrPath = strings.TrimSpace(nameOrPath)
	if nameOrPath == "" {
		return ""
	}

	candidates := []string{nameOrPath}
	base := filepath.Base(filepath.Clean(nameOrPath))
	if base != "" && base != "." && base != nameOrPath {
		candidates = append(candidates, base)
	}

	db := consts.GetGormSSAProjectDataBase()
	for _, name := range candidates {
		if db != nil {
			if prog, err := GetSSAProgramByName(db, name); err == nil && prog != nil {
				if p := localPathFromCompileConfigJSON(prog.ConfigInput); p != "" {
					return p
				}
				if p := localPathFromSSAProjectID(prog.ProjectID); p != "" {
					return p
				}
			}
		}
		projectName := ssaproject.BaseProjectNameFromProgramName(name)
		if projectName != "" && projectName != name {
			if p := localPathFromSSAProjectName(projectName); p != "" {
				return p
			}
		}
	}
	return ""
}
