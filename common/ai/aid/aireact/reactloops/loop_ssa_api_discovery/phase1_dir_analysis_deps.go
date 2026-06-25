package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reJSVersionComment = regexp.MustCompile(`(?i)(?:@version|version\s*[:=]\s*|pdf\.?js\s+v?)(\d+\.\d+(?:\.\d+)?(?:[-+][\w.]+)?)`)
	reSemVerLoose      = regexp.MustCompile(`\b(\d+\.\d+(?:\.\d+)?(?:[-+][\w.]+)?)\b`)
)

type npmPackageMeta struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// inferDependencyInfo extracts third-party name/group/version from directory evidence.
func inferDependencyInfo(node *DirectoryNode, codeRoot string) *DepInfo {
	if node == nil {
		return nil
	}
	name := filepath.Base(node.RelPath)
	desc := thirdPartyDescription(name)
	relLower := strings.ToLower(filepath.ToSlash(node.RelPath))

	info := &DepInfo{
		Name:        thirdPartyName(name),
		Description: desc,
	}

	dirPath := filepath.Join(codeRoot, filepath.FromSlash(node.RelPath))
	if meta := readPackageMeta(dirPath); meta != nil {
		if strings.TrimSpace(meta.Name) != "" {
			info.Name = meta.Name
		}
		if v := normalizeVersion(meta.Version); v != "" {
			info.Version = v
		}
	}

	if info.Version == "" {
		if v := scanDirVersionMarkers(dirPath); v != "" {
			info.Version = v
		}
	}
	if info.Version == "" {
		if v := scanAssetFileVersion(dirPath, node.FileNames); v != "" {
			info.Version = v
		}
	}

	if strings.Contains(relLower, "/src/main/java/") {
		if pkg := javaPackageFromDir(dirPath, node.FileNames); pkg != "" {
			info.Group = pkg
			if info.Version == "" {
				if v := readJavaDirVersion(dirPath, pkg); v != "" {
					info.Version = v
				}
			}
		}
	}

	if pluginRoot := findPluginRoot(dirPath); pluginRoot != "" && pluginRoot != dirPath {
		if meta := readPackageMeta(pluginRoot); meta != nil {
			if strings.TrimSpace(meta.Name) != "" && (info.Name == "" || strings.EqualFold(info.Name, thirdPartyName(name))) {
				info.Name = meta.Name
			}
			if info.Version == "" {
				info.Version = normalizeVersion(meta.Version)
			}
		}
		if info.Version == "" {
			info.Version = scanDirVersionMarkers(pluginRoot)
		}
	}

	info.Name = strings.TrimSpace(info.Name)
	info.Group = strings.TrimSpace(info.Group)
	info.Version = normalizeVersion(info.Version)
	if info.Name == "" {
		info.Name = thirdPartyName(name)
	}
	return info
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" || v == "unknown" || v == "n/a" {
		return ""
	}
	return strings.TrimSpace(v)
}

func readPackageMeta(dir string) *npmPackageMeta {
	for _, fn := range []string{"package.json", "bower.json"} {
		b, err := os.ReadFile(filepath.Join(dir, fn))
		if err != nil {
			continue
		}
		var meta npmPackageMeta
		if json.Unmarshal(b, &meta) == nil {
			return &meta
		}
	}
	return nil
}

func scanDirVersionMarkers(dir string) string {
	for _, fn := range []string{"version.txt", "VERSION", "version"} {
		b, err := os.ReadFile(filepath.Join(dir, fn))
		if err != nil {
			continue
		}
		if v := normalizeVersion(strings.TrimSpace(string(b))); v != "" {
			return v
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		lower := strings.ToLower(e.Name())
		if !strings.HasPrefix(lower, "changelog") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		lines := strings.Split(string(b), "\n")
		for _, line := range lines[:minInt(5, len(lines))] {
			if m := reSemVerLoose.FindStringSubmatch(line); len(m) > 1 {
				return normalizeVersion(m[1])
			}
		}
	}
	return ""
}

func scanAssetFileVersion(dir string, fileNames []string) string {
	candidates := append([]string(nil), fileNames...)
	if len(candidates) == 0 {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return ""
		}
		for _, e := range entries {
			if !e.IsDir() {
				candidates = append(candidates, e.Name())
			}
		}
	}
	prioritize := func(name string) int {
		lower := strings.ToLower(name)
		switch {
		case strings.Contains(lower, "pdf"):
			return 0
		case strings.HasSuffix(lower, ".min.js"):
			return 1
		case strings.HasSuffix(lower, ".js"):
			return 2
		case strings.HasSuffix(lower, ".css"):
			return 3
		default:
			return 9
		}
	}
	sortNames := append([]string(nil), candidates...)
	for i := 0; i < len(sortNames); i++ {
		for j := i + 1; j < len(sortNames); j++ {
			if prioritize(sortNames[j]) < prioritize(sortNames[i]) {
				sortNames[i], sortNames[j] = sortNames[j], sortNames[i]
			}
		}
	}
	for _, fn := range sortNames {
		lower := strings.ToLower(fn)
		if !strings.HasSuffix(lower, ".js") && !strings.HasSuffix(lower, ".css") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, fn))
		if err != nil {
			continue
		}
		head := string(b)
		if len(head) > 8192 {
			head = head[:8192]
		}
		if m := reJSVersionComment.FindStringSubmatch(head); len(m) > 1 {
			return normalizeVersion(m[1])
		}
	}
	return ""
}

func javaPackageFromDir(dir string, fileNames []string) string {
	for _, fn := range fileNames {
		if !strings.HasSuffix(strings.ToLower(fn), ".java") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, fn))
		if err != nil {
			continue
		}
		if m := reJavaPackageLine.FindStringSubmatch(string(b)); len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".java") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if m := reJavaPackageLine.FindStringSubmatch(string(b)); len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

func readJavaDirVersion(dir, pkg string) string {
	manifest := filepath.Join(dir, "META-INF", "MANIFEST.MF")
	if b, err := os.ReadFile(manifest); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(strings.ToLower(line), "implementation-version:") {
				return normalizeVersion(strings.TrimSpace(line[strings.Index(line, ":")+1:]))
			}
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".properties") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			lower := strings.ToLower(line)
			if strings.Contains(lower, "version") && strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					if v := normalizeVersion(parts[1]); v != "" {
						return v
					}
				}
			}
		}
	}
	_ = pkg
	return ""
}

func findPluginRoot(dir string) string {
	dir = filepath.Clean(dir)
	for i := 0; i < 8; i++ {
		if readPackageMeta(dir) != nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func enrichDepInfo(node *DirectoryNode, codeRoot string, info *DepInfo) *DepInfo {
	inferred := inferDependencyInfo(node, codeRoot)
	if inferred == nil {
		return info
	}
	if info == nil {
		return inferred
	}
	out := *info
	if strings.TrimSpace(out.Name) == "" || strings.EqualFold(out.Name, thirdPartyName(filepath.Base(node.RelPath))) {
		out.Name = inferred.Name
	}
	if strings.TrimSpace(out.Group) == "" {
		out.Group = inferred.Group
	}
	if normalizeVersion(out.Version) == "" {
		out.Version = inferred.Version
	} else {
		out.Version = normalizeVersion(out.Version)
	}
	if strings.TrimSpace(out.Description) == "" {
		out.Description = inferred.Description
	}
	return &out
}
