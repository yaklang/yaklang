package loop_ssa_api_discovery

import (
	"path/filepath"
	"strings"
)

const (
	fileCategoryCode     = "code"
	fileCategoryResource = "resource"
	fileCategoryConfig   = "config"
	fileCategoryBuild    = "build"
	fileCategoryOther    = "other"
)

var (
	codeExtensions = map[string]struct{}{
		".java": {}, ".go": {}, ".py": {}, ".php": {}, ".rb": {},
		".js": {}, ".ts": {}, ".jsx": {}, ".tsx": {}, ".vue": {},
		".kt": {}, ".scala": {}, ".groovy": {}, ".cs": {},
	}
	resourceExtensions = map[string]struct{}{
		".html": {}, ".htm": {}, ".css": {}, ".scss": {}, ".less": {},
		".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".svg": {}, ".ico": {}, ".webp": {},
		".woff": {}, ".woff2": {}, ".ttf": {}, ".eot": {},
		".mp3": {}, ".mp4": {}, ".wav": {}, ".pdf": {},
		".map": {},
	}
	configExtensions = map[string]struct{}{
		".yml": {}, ".yaml": {}, ".properties": {}, ".xml": {}, ".json": {}, ".toml": {}, ".ini": {}, ".env": {},
	}
	buildBasenames = map[string]struct{}{
		"pom.xml": {}, "build.gradle": {}, "build.gradle.kts": {}, "go.mod": {}, "go.sum": {},
		"package.json": {}, "package-lock.json": {}, "yarn.lock": {}, "composer.json": {},
		"pyproject.toml": {}, "requirements.txt": {}, "setup.py": {}, "Cargo.toml": {},
	}
	resourceDirTokens = []string{
		"static", "assets", "public", "resources", "webapp", "templates", "template",
		"dist", "build", "target", "out", "node_modules", "vendor", ".git", ".idea",
	}
)

// classifyFileCategory assigns code|resource|config|build|other from path and optional content sniff.
func classifyFileCategory(relPath string, contentHead []byte) string {
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	if isResourceDirPath(relPath) {
		return fileCategoryResource
	}
	base := strings.ToLower(filepath.Base(relPath))
	ext := strings.ToLower(filepath.Ext(relPath))

	if _, ok := buildBasenames[base]; ok {
		return fileCategoryBuild
	}
	if _, ok := configExtensions[ext]; ok {
		if isLikelyConfigPath(relPath) {
			return fileCategoryConfig
		}
	}
	if _, ok := resourceExtensions[ext]; ok {
		return fileCategoryResource
	}
	if _, ok := codeExtensions[ext]; ok {
		return fileCategoryCode
	}
	if len(contentHead) > 0 {
		s := strings.ToLower(string(contentHead))
		if strings.Contains(s, "@controller") || strings.Contains(s, "@restcontroller") ||
			strings.Contains(s, "http.handlefunc") || strings.Contains(s, "@app.route") {
			return fileCategoryCode
		}
	}
	if isResourceDirPath(relPath) {
		return fileCategoryResource
	}
	return fileCategoryOther
}

func isLikelyConfigPath(rel string) bool {
	lower := strings.ToLower(rel)
	if strings.Contains(lower, "/config/") || strings.Contains(lower, "/resources/") ||
		strings.Contains(lower, "/web-inf/") || strings.HasPrefix(lower, "config/") {
		return true
	}
	switch strings.ToLower(filepath.Base(rel)) {
	case "application.yml", "application.yaml", "application.properties",
		"bootstrap.yml", "bootstrap.yaml", "web.xml", "nginx.conf":
		return true
	}
	return strings.HasPrefix(strings.ToLower(filepath.Base(rel)), "application")
}

func isResourceDirPath(rel string) bool {
	lower := strings.ToLower(filepath.ToSlash(rel))
	for _, tok := range resourceDirTokens {
		if strings.Contains(lower, "/"+tok+"/") || strings.HasPrefix(lower, tok+"/") {
			return true
		}
	}
	return false
}

// shouldSkipPathForCodeHarvest returns true when path is classified as resource/build artifact dir.
func shouldSkipPathForCodeHarvest(relPath string) bool {
	cat := classifyFileCategory(relPath, nil)
	return cat == fileCategoryResource || isResourceDirPath(relPath)
}
