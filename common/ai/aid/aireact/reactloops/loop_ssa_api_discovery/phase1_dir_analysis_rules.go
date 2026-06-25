package loop_ssa_api_discovery

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var thirdPartyNamePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^(apache|google|guava|fastjson|jackson|lombok|spring|hibernate|mybatis|slf4j|log4j|commons|gson|protobuf|grpc|junit|mockito|assertj|hutool|druid|hikaricp|poi|xmlbeans|sfntly|bitmap|font|pdfjs|tinymce|codemirror|spectrum)$`),
}

var staticAssetExtensions = map[string]struct{}{
	".ftl": {}, ".min.js": {}, ".js": {}, ".css": {}, ".html": {}, ".htm": {},
	".json": {}, ".map": {}, ".svg": {}, ".png": {}, ".gif": {}, ".jpg": {},
	".jpeg": {}, ".webp": {}, ".woff": {}, ".woff2": {}, ".ttf": {}, ".eot": {},
	".ico": {}, ".xml": {}, ".txt": {}, ".properties": {}, ".md": {},
}

var defaultThirdPartyPathPatterns = []string{
	"**/.mvn/**",
	"**/gradle/wrapper/**",
	"**/target/**",
	"**/build/**",
	"**/node_modules/**",
	"**/webapp/resource/plugins/**",
	"**/resource/plugins/**",
	"**/src/main/webapp/**",
	"**/webapp/**",
	"**/pdfjs/locale/**",
	"**/locale/**",
	"**/locales/**",
	"**/i18n/**",
	"**/l10n/**",
}

func pathMatchesGlobPattern(rel, pattern string) bool {
	rel = strings.TrimSpace(filepath.ToSlash(rel))
	pattern = strings.TrimSpace(filepath.ToSlash(pattern))
	if rel == "" || pattern == "" {
		return false
	}
	if pattern == "**" || pattern == "*" {
		return true
	}
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		rest := rel
		for i, part := range parts {
			part = strings.Trim(part, "/")
			if part == "" {
				continue
			}
			idx := strings.Index(rest, part)
			if idx < 0 {
				return false
			}
			if i == 0 && !strings.HasPrefix(pattern, "**") && idx != 0 {
				return false
			}
			rest = rest[idx+len(part):]
		}
		return true
	}
	if strings.Contains(pattern, "*") {
		if ok, _ := filepath.Match(pattern, rel); ok {
			return true
		}
		for _, seg := range strings.Split(rel, "/") {
			if ok, _ := filepath.Match(pattern, seg); ok {
				return true
			}
		}
		return false
	}
	return rel == pattern || strings.HasPrefix(rel, pattern+"/") || strings.HasSuffix("/"+rel, "/"+pattern)
}

func matchesAnyPathPattern(rel string, patterns []string) bool {
	for _, p := range patterns {
		if pathMatchesGlobPattern(rel, p) {
			return true
		}
	}
	return false
}

func (ctx *ProjectContextSummaryV1) matchesFirstPartyPath(rel string) bool {
	if ctx == nil {
		return isLikelyFirstPartyContainerPath(rel)
	}
	rel = filepath.ToSlash(rel)
	for _, root := range ctx.FirstPartyBoundary.ModuleRoots {
		root = strings.TrimSpace(filepath.ToSlash(root))
		if root == "" || root == "." {
			if rel == "" {
				return true
			}
			continue
		}
		if rel == root || strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	if matchesAnyPathPattern(rel, ctx.FirstPartyBoundary.PathPatterns) {
		return true
	}
	pkgHint := strings.ReplaceAll(strings.ToLower(rel), "/", ".")
	for _, root := range ctx.FirstPartyBoundary.PackageRoots {
		root = strings.TrimSpace(strings.ToLower(root))
		if root != "" && (pkgHint == root || strings.HasPrefix(pkgHint, root+".")) {
			return true
		}
	}
	for _, mod := range ctx.BusinessModules {
		hint := strings.TrimSpace(filepath.ToSlash(mod.JavaPathHint))
		if hint != "" && (rel == hint || strings.HasPrefix(rel, hint+"/")) {
			return true
		}
		if modRoot := strings.TrimSpace(filepath.ToSlash(mod.ModuleRoot)); modRoot != "" {
			if rel == modRoot || strings.HasPrefix(rel, modRoot+"/") {
				return true
			}
		}
	}
	return isLikelyFirstPartyContainerPath(rel)
}

func (ctx *ProjectContextSummaryV1) matchesThirdPartyPath(rel string) bool {
	rel = filepath.ToSlash(rel)
	if ctx != nil {
		if matchesAnyPathPattern(rel, ctx.ThirdPartyBoundary.PathPatterns) {
			return true
		}
		pkgHint := strings.ReplaceAll(strings.ToLower(rel), "/", ".")
		for _, prefix := range ctx.ThirdPartyBoundary.PackagePrefixes {
			prefix = strings.ToLower(strings.TrimSpace(prefix))
			if prefix != "" && strings.HasPrefix(pkgHint, prefix) {
				return true
			}
		}
	}
	return matchesAnyPathPattern(rel, defaultThirdPartyPathPatterns)
}

func isLikelyFirstPartyContainerPath(rel string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" {
		return true
	}
	if strings.Contains(rel, "/src/main/java/") || strings.HasSuffix(rel, "/src/main/java") {
		return true
	}
	if strings.Contains(rel, "/src/main/resources/") || strings.HasSuffix(rel, "/src/main/resources") {
		return true
	}
	return false
}

func isBuildModuleContainer(node *DirectoryNode) bool {
	if node == nil {
		return false
	}
	if node.RelPath == "" {
		return true
	}
	hasBuildFile := false
	for _, fn := range node.FileNames {
		lower := strings.ToLower(fn)
		if lower == "pom.xml" || lower == "build.gradle" || lower == "build.gradle.kts" {
			hasBuildFile = true
		}
		if strings.HasSuffix(lower, ".java") {
			return false
		}
	}
	if hasBuildFile {
		return true
	}
	if node.FileCount == 0 && node.TotalSizeKB > node.DirectSizeKB {
		return true
	}
	return false
}

func isKnownThirdPartyDir(name, rel string) bool {
	if matchesAnyPathPattern(rel, defaultThirdPartyPathPatterns) {
		return true
	}
	lower := strings.ToLower(name)
	relLower := strings.ToLower(filepath.ToSlash(rel))

	thirdPartyDirNames := []string{
		"generated", "target", "build", "third_party", "thirdparty", "third-party",
		"node_modules", ".git", "font", "sfntly", "bitmap",
		"locale", "locales", "plugins", "plugin", "i18n", "l10n", "lang", "langs",
		"pdfjs", "tinymce", "themes", "theme", "skins", "skin", "codemirror", "spectrum",
	}
	for _, n := range thirdPartyDirNames {
		if lower == n || strings.HasSuffix(relLower, "/"+n) {
			return true
		}
	}

	knownPrefixes := []string{
		"org.apache.commons", "com.google.guava", "com.alibaba.fastjson",
		"com.fasterxml.jackson", "org.projectlombok", "org.springframework",
		"org.hibernate", "org.mybatis", "org.slf4j", "org.apache.logging",
		"org.junit", "org.mockito", "com.google.protobuf", "io.grpc",
		"cn.hutool", "com.alibaba.druid", "com.zaxxer.hikari",
		"org.apache.poi", "org.apache.xmlbeans",
	}
	pkgHint := strings.ReplaceAll(relLower, "/", ".")
	for _, p := range knownPrefixes {
		if strings.HasPrefix(pkgHint, p) {
			return true
		}
	}

	if isUtilityOnlyDir(name, rel) {
		return true
	}

	return false
}

func isStaticAssetFileName(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".min.js") {
		return true
	}
	ext := filepath.Ext(lower)
	if ext == "" {
		return false
	}
	_, ok := staticAssetExtensions[ext]
	return ok
}

func isStaticAssetOnlyDir(node *DirectoryNode) bool {
	if node == nil || len(node.FileNames) == 0 {
		return false
	}
	for _, fn := range node.FileNames {
		lower := strings.ToLower(fn)
		if strings.HasSuffix(lower, ".java") {
			return false
		}
		if !isStaticAssetFileName(fn) {
			return false
		}
	}
	return true
}

func shouldTreatAsThirdPartyDir(node *DirectoryNode, ctx *ProjectContextSummaryV1) bool {
	if node == nil {
		return false
	}
	if ctx != nil && ctx.matchesFirstPartyPath(node.RelPath) {
		if isBuildModuleContainer(node) || strings.Contains(node.RelPath, "/src/main/java/") {
			return false
		}
	}
	if ctx != nil && ctx.matchesThirdPartyPath(node.RelPath) {
		return true
	}
	name := filepath.Base(node.RelPath)
	if isKnownThirdPartyDir(name, node.RelPath) {
		if strings.Contains(node.RelPath, "/src/main/java/") {
			return false
		}
		return true
	}
	return isStaticAssetOnlyDir(node)
}

func buildModuleContainerDirAnalysis(node *DirectoryNode, ctx *ProjectContextSummaryV1) *DirAnalysis {
	name := filepath.Base(node.RelPath)
	if name == "." {
		name = "project_root"
	}
	desc := fmt.Sprintf("项目模块/容器目录 %s，向下包含业务源码", name)
	if ctx != nil {
		for _, mod := range ctx.BusinessModules {
			if mod.ModuleRoot == node.RelPath || strings.HasPrefix(node.RelPath, mod.ModuleRoot+"/") {
				if strings.TrimSpace(mod.Role) != "" {
					desc = mod.Role
					break
				}
			}
		}
		if node.RelPath == "" && strings.TrimSpace(ctx.Summary) != "" {
			desc = ctx.Summary
		}
	}
	return &DirAnalysis{
		FunctionDesc: desc,
		TechLayers:   []string{"tech:config"},
		BizDomains:   []string{},
		DbFeatures:   []string{"db:none"},
		BfsControl:   BfsControlContinue,
		IsBusiness:   true,
		IsHttpEntry:  false,
		HasDB:        false,
	}
}

func buildThirdPartyDirAnalysis(node *DirectoryNode, ctx *ProjectContextSummaryV1, codeRoot string) *DirAnalysis {
	if node == nil {
		return nil
	}
	name := filepath.Base(node.RelPath)
	desc := thirdPartyDescription(name)
	relLower := strings.ToLower(filepath.ToSlash(node.RelPath))
	if ctx != nil && strings.TrimSpace(ctx.ThirdPartyBoundary.Description) != "" &&
		(strings.Contains(relLower, "/plugins/") || strings.Contains(relLower, "/locale/")) {
		desc = ctx.ThirdPartyBoundary.Description
	}
	switch {
	case strings.Contains(relLower, "/locale/") || strings.Contains(relLower, "/locales/"):
		desc = "国际化/语言包资源目录，无 Java 业务逻辑"
	case strings.Contains(relLower, "/plugins/") || strings.Contains(relLower, "/tinymce/"):
		desc = "第三方前端插件静态资源，无 Java 业务逻辑"
	case isStaticAssetOnlyDir(node):
		desc = fmt.Sprintf("静态资源目录 (%s)，无 Java 业务逻辑", name)
	case strings.Contains(relLower, "/webapp"):
		desc = "Web 静态资源目录，无 Java 业务逻辑"
	}
	dep := enrichDepInfo(node, codeRoot, &DepInfo{
		Name:        thirdPartyName(name),
		Description: desc,
	})
	return &DirAnalysis{
		FunctionDesc: desc,
		TechLayers:   []string{"tech:third_party"},
		BizDomains:   []string{},
		DbFeatures:   []string{"db:none"},
		BfsControl:   BfsControlStop,
		IsBusiness:   false,
		IsHttpEntry:  false,
		HasDB:        false,
		DepInfo:      dep,
	}
}

func tryProgrammaticDirAnalysis(node *DirectoryNode, ctx *ProjectContextSummaryV1, codeRoot string) (*DirAnalysis, bool) {
	if node == nil {
		return nil, false
	}
	hasJava := false
	for _, fn := range node.FileNames {
		if strings.HasSuffix(strings.ToLower(fn), ".java") {
			hasJava = true
			break
		}
	}

	if isBuildModuleContainer(node) || (ctx != nil && ctx.matchesFirstPartyPath(node.RelPath) && !hasJava && !isStaticAssetOnlyDir(node)) {
		return buildModuleContainerDirAnalysis(node, ctx), true
	}
	if shouldTreatAsThirdPartyDir(node, ctx) {
		return buildThirdPartyDirAnalysis(node, ctx, codeRoot), true
	}
	if !hasJava && isStaticAssetOnlyDir(node) {
		return buildThirdPartyDirAnalysis(node, ctx, codeRoot), true
	}
	return nil, false
}

func formatProjectContextForDirPrompt(ctx *ProjectContextSummaryV1) string {
	if ctx == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Project context (use as boundary reference)\n")
	if s := strings.TrimSpace(ctx.Summary); s != "" {
		sb.WriteString("- Summary: " + s + "\n")
	}
	if s := strings.TrimSpace(ctx.FirstPartyBoundary.Description); s != "" {
		sb.WriteString("- First-party: " + s + "\n")
	}
	if len(ctx.FirstPartyBoundary.ModuleRoots) > 0 {
		sb.WriteString("- Business modules: " + strings.Join(ctx.FirstPartyBoundary.ModuleRoots, ", ") + "\n")
	}
	if s := strings.TrimSpace(ctx.ThirdPartyBoundary.Description); s != "" {
		sb.WriteString("- Third-party/static: " + s + "\n")
	}
	if len(ctx.ThirdPartyBoundary.Examples) > 0 {
		sb.WriteString("- Third-party examples: " + strings.Join(ctx.ThirdPartyBoundary.Examples, "; ") + "\n")
	}
	return sb.String()
}

func thirdPartyName(name string) string {
	base := strings.ToLower(name)
	for _, re := range thirdPartyNamePatterns {
		if re.MatchString(base) {
			return strings.Title(base)
		}
	}
	return strings.Title(base)
}

func thirdPartyDescription(name string) string {
	switch strings.ToLower(name) {
	case "guava":
		return "Google Guava core libraries"
	case "fastjson":
		return "Alibaba fastjson JSON processor"
	case "jackson":
		return "Jackson JSON processor"
	case "lombok":
		return "Lombok code generation"
	case "commons":
		return "Apache Commons utilities"
	case "slf4j":
		return "SLF4J logging facade"
	case "log4j":
		return "Log4j logging framework"
	case "spring":
		return "Spring framework modules"
	case "hibernate":
		return "Hibernate ORM"
	case "mybatis":
		return "MyBatis persistence framework"
	case "junit":
		return "JUnit test framework"
	case "mockito":
		return "Mockito mocking framework"
	case "gson":
		return "Google Gson JSON library"
	case "protobuf":
		return "Protocol Buffers"
	case "grpc":
		return "gRPC framework"
	case "hutool":
		return "Hutool Java utility library"
	case "druid":
		return "Alibaba Druid database connection pool"
	case "hikaricp":
		return "HikariCP database connection pool"
	case "poi":
		return "Apache POI Office document library"
	case "xmlbeans":
		return "Apache XMLBeans"
	case "sfntly":
		return "SFNTLY font library"
	case "bitmap":
		return "Bitmap utilities"
	case "font":
		return "Font handling library"
	case "pdfjs":
		return "Mozilla PDF.js PDF rendering library"
	case "tinymce":
		return "TinyMCE rich text editor"
	case "locale", "locales", "i18n", "l10n", "lang", "langs":
		return "Internationalization / locale resource files"
	case "plugins", "plugin":
		return "Third-party frontend plugin static assets"
	case "codemirror":
		return "CodeMirror editor library"
	case "spectrum":
		return "Spectrum color picker library"
	}
	return fmt.Sprintf("Third-party library: %s", name)
}

func isUtilityOnlyDir(name, rel string) bool {
	relLower := strings.ToLower(filepath.ToSlash(rel))
	if strings.Contains(relLower, "/src/main/java/") {
		return false
	}
	lower := strings.ToLower(name)
	utilityNames := []string{"util", "utils", "helper", "helpers", "factory", "factories", "constants", "constant", "common", "base"}
	for _, u := range utilityNames {
		if lower == u || strings.HasSuffix(lower, "_"+u) || strings.HasSuffix(lower, "-"+u) {
			return true
		}
	}
	return false
}
