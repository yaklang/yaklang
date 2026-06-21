package loop_code_security_audit

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

// PreAnalysisResult 确定性预分析结果（不依赖 LLM）
type PreAnalysisResult struct {
	Language       string              // 主语言（go/java/python/javascript/php/rust/c/cpp）
	LanguageVer    string              // 语言版本（如 "1.21"）
	Frameworks     []string            // 框架列表（如 ["gin", "gorm", "jwt-go"]）
	DBLibs         []string            // 数据库相关库（如 ["gorm", "sqlx"]）
	HTTPFrameworks []string            // HTTP 框架（如 ["gin", "echo", "fiber"]）
	AuthLibs       []string            // 认证相关库（如 ["jwt-go", "bcrypt", "oauth2"]）
	CryptoLibs     []string            // 加密相关库
	FileOpsLibs    []string            // 文件操作相关库
	ExecLibs       []string            // 命令执行相关库
	TemplateLibs   []string            // 模板引擎相关库
	EntryPoints    []EntryPointInfo    // 入口点详情
	ProjectScale   ProjectScaleInfo    // 项目规模
	ConfigFiles    []string            // 关键配置文件路径
	DepFilePath    string              // 依赖文件路径（go.mod / package.json 等）
	RawDeps        map[string]string   // 原始依赖名→版本映射
}

// EntryPointInfo 入口点信息
type EntryPointInfo struct {
	File string // 文件绝对路径
	Line int    // 行号
	Type string // 类型：main / http_handler / grpc_service / cli_command
	Name string // 函数/结构体名
}

// ProjectScaleInfo 项目规模信息
type ProjectScaleInfo struct {
	TotalFiles int // 源代码文件总数
	TotalLines int // 代码行数估算
	TopDirs    int // 顶层目录数
}

// 安全相关的依赖关键词映射
var securityDepKeywords = map[string][]string{
	"db": {"gorm", "sqlx", "database/sql", "pg", "mongo", "redis", "bolt", "bbtd", "badger",
		"ent", "sqlc", "reform", "pop", "xorm", "go-pg", "upper/db",
		"sequelize", "knex", "prisma", "typeorm", "mongoose", "mikro-orm",
		"hibernate", "mybatis", "jpa", "ebean", "sqlalchemy", "django.db", "peewee", "tortoise"},
	"http": {"gin", "echo", "fiber", "chi", "mux", "httprouter", "iris", "fasthttp", "beego", "revel",
		"express", "fastify", "koa", "hapi", "nest", "next", "nuxt",
		"spring-boot", "spring-web", "jersey", "vert.x", "ktor", "http4s",
		"flask", "django", "fastapi", "tornado", "bottle", "aiohttp", "starlette",
		"laravel", "symfony", "slim", "guzzle"},
	"auth": {"jwt", "oauth", "session", "passport", "bcrypt", "argon2", "scrypt",
		"shiro", "spring-security", "pac4j",
		"django.contrib.auth", "flask-login", "flask-security",
		"jwt-go", "golang-jwt", "dgrijalva/jwt-go", "lestrrat-go/jwt", "square/go-jose",
		"jsonwebtoken", "jose", "passport-jwt",
		"firebase-auth", "supabase-auth"},
	"crypto": {"crypto", "aes", "rsa", "hmac", "nacl", "blowfish",
		"crypto/aes", "crypto/rsa", "crypto/sha256", "crypto/md5", "crypto/sha1",
		"bouncy-castle", "java.security", "javax.crypto",
		"cryptography", "pycryptodome", "hashlib",
		"crypto-js", "node-forge", "tweetnacl"},
	"exec": {"os/exec", "syscall", "os/exec.Command", "os.StartProcess",
		"Runtime.exec", "ProcessBuilder",
		"subprocess", "os.system", "os.popen", "commands",
		"child_process", "execa", "shelljs"},
	"file": {"os.Open", "os.Create", "os.ReadFile", "os.WriteFile", "ioutil.ReadFile", "ioutil.WriteFile",
		"io/fs", "filepath", "path/filepath",
		"java.io", "java.nio.file", "Files.",
		"open()", "os.path", "pathlib", "shutil",
		"fs.readFile", "fs.writeFile", "fs.createReadStream", "path.join"},
	"deser": {"encoding/gob", "encoding/json", "json.Unmarshal", "xml.Unmarshal",
		"ObjectInputStream", "XMLDecoder", "XStream", "Jackson", "Gson",
		"pickle", "yaml.load", "marshal", "shelve",
		"node-serialize", "js-yaml"},
	"template": {"html/template", "text/template", "pongo2", "jet", "amber",
		"thymeleaf", "freemarker", "velocity", "jsp",
		"jinja2", "mako", "chameleon", "tornado.template",
		"blade", "twig", "smarty"},
}

// PreAnalyzeProject 对项目进行确定性预分析（不依赖 LLM）。
// 通过读取依赖文件、扫描文件扩展名、搜索入口点模式等方式提取结构化信息。
func PreAnalyzeProject(projectPath string) *PreAnalysisResult {
	result := &PreAnalysisResult{
		RawDeps: make(map[string]string),
	}

	// 1. 检测主语言
	result.Language = detectLanguage(projectPath)
	log.Infof("[PreAnalyze] Detected language: %s", result.Language)

	// 2. 读取依赖文件
	depFile := findDepFile(projectPath, result.Language)
	if depFile != "" {
		result.DepFilePath = depFile
		parseDepFile(depFile, result)
		log.Infof("[PreAnalyze] Parsed dep file: %s, found %d deps", depFile, len(result.RawDeps))
	}

	// 3. 分类安全相关依赖
	classifyDeps(result)

	// 4. 扫描项目规模
	result.ProjectScale = scanProjectScale(projectPath, result.Language)
	log.Infof("[PreAnalyze] Project scale: %d files, ~%d lines", result.ProjectScale.TotalFiles, result.ProjectScale.TotalLines)

	// 5. 查找入口点
	result.EntryPoints = findEntryPoints(projectPath, result.Language)
	log.Infof("[PreAnalyze] Found %d entry points", len(result.EntryPoints))

	// 6. 查找关键配置文件
	result.ConfigFiles = findConfigFiles(projectPath, result.Language)

	return result
}

// detectLanguage 通过文件扩展名计数检测主语言
func detectLanguage(projectPath string) string {
	langCounts := map[string]int{}
	extToLang := map[string]string{
		".go":   "go",
		".java": "java",
		".py":   "python",
		".js":   "javascript",
		".ts":   "javascript",
		".php":  "php",
		".rb":   "ruby",
		".rs":   "rust",
		".c":    "c",
		".cpp":  "cpp",
		".cc":   "cpp",
		".cxx":  "cpp",
		".h":    "c",
		".cs":   "csharp",
		".kt":   "kotlin",
		".scala":"scala",
		".swift":"swift",
	}

	filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(projectPath, path)
		if rel == "." {
			return nil
		}
		// 跳过隐藏目录和 vendor/node_modules
		for _, skip := range []string{".", "vendor", "node_modules", ".git", "__pycache__", "testdata"} {
			if strings.HasPrefix(rel, skip+string(os.PathSeparator)) || strings.HasPrefix(rel, skip+"/") {
				return nil
			}
		}
		ext := strings.ToLower(filepath.Ext(path))
		if lang, ok := extToLang[ext]; ok {
			langCounts[lang]++
		}
		return nil
	})

	maxCount := 0
	detected := "unknown"
	for lang, count := range langCounts {
		if count > maxCount {
			maxCount = count
			detected = lang
		}
	}
	return detected
}

// findDepFile 查找项目的依赖声明文件
func findDepFile(projectPath, language string) string {
	candidates := map[string][]string{
		"go":         {"go.mod"},
		"java":       {"pom.xml", "build.gradle", "build.gradle.kts"},
		"python":     {"requirements.txt", "pyproject.toml", "setup.py", "Pipfile", "poetry.lock"},
		"javascript": {"package.json"},
		"php":        {"composer.json"},
		"ruby":       {"Gemfile"},
		"rust":       {"Cargo.toml"},
		"c":          {"CMakeLists.txt", "Makefile", "configure.ac", "meson.build"},
		"cpp":        {"CMakeLists.txt", "Makefile", "conanfile.txt", "vcpkg.json"},
	}

	langs := []string{language}
	if language == "c" || language == "cpp" {
		langs = []string{"c", "cpp"}
	}

	for _, lang := range langs {
		for _, name := range candidates[lang] {
			path := filepath.Join(projectPath, name)
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}

	// 通用回退
	for _, name := range []string{"go.mod", "package.json", "pom.xml", "requirements.txt", "Cargo.toml", "composer.json"} {
		path := filepath.Join(projectPath, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// parseDepFile 解析依赖文件，提取依赖名和版本
func parseDepFile(depFile string, result *PreAnalysisResult) {
	f, err := os.Open(depFile)
	if err != nil {
		return
	}
	defer f.Close()

	baseName := filepath.Base(depFile)
	scanner := bufio.NewScanner(f)

	switch baseName {
	case "go.mod":
		parseGoMod(scanner, result)
	case "package.json":
		parsePackageJSON(depFile, result)
	case "pom.xml":
		parsePomXML(depFile, result)
	case "requirements.txt":
		parseRequirementsTxt(scanner, result)
	case "Cargo.toml":
		parseCargoToml(scanner, result)
	case "composer.json":
		parseComposerJSON(depFile, result)
	}
}

// parseGoMod 解析 go.mod 文件
func parseGoMod(scanner *bufio.Scanner, result *PreAnalysisResult) {
	inRequire := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "go ") {
			result.LanguageVer = strings.TrimPrefix(line, "go ")
			continue
		}
		if line == "require (" {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}
		if inRequire || strings.HasPrefix(line, "require ") {
			dep := strings.TrimPrefix(line, "require ")
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			parts := strings.Fields(dep)
			if len(parts) >= 1 {
				name := parts[0]
				ver := ""
				if len(parts) >= 2 {
					ver = parts[1]
				}
				result.RawDeps[name] = ver
			}
		}
	}
}

// parsePackageJSON 解析 package.json（简单文本提取，不引入 JSON 解析依赖）
func parsePackageJSON(depFile string, result *PreAnalysisResult) {
	data, err := os.ReadFile(depFile)
	if err != nil {
		return
	}
	content := string(data)
	// 提取依赖名：匹配 "name": "version" 模式
	re := regexp.MustCompile(`"([^"]+)"\s*:\s*"[^"]*"`)
	matches := re.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		name := m[1]
		// 过滤掉非依赖字段
		if name == "name" || name == "version" || name == "description" ||
			name == "main" || name == "license" || name == "author" ||
			name == "scripts" || name == "private" {
			continue
		}
		result.RawDeps[name] = ""
	}
}

// parsePomXML 简单提取 pom.xml 中的 groupId:artifactId
func parsePomXML(depFile string, result *PreAnalysisResult) {
	data, err := os.ReadFile(depFile)
	if err != nil {
		return
	}
	content := string(data)
	// 提取 artifactId
	re := regexp.MustCompile(`<artifactId>([^<]+)</artifactId>`)
	matches := re.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		result.RawDeps[m[1]] = ""
	}
}

// parseRequirementsTxt 解析 requirements.txt
func parseRequirementsTxt(scanner *bufio.Scanner, result *PreAnalysisResult) {
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		// 格式: package==version 或 package>=version
		re := regexp.MustCompile(`^([a-zA-Z0-9_-]+)\s*([><=!~]+\s*[\d.]+)?`)
		if m := re.FindStringSubmatch(line); m != nil {
			ver := ""
			if len(m) > 2 {
				ver = m[2]
			}
			result.RawDeps[strings.ToLower(m[1])] = ver
		}
	}
}

// parseCargoToml 简单提取 Cargo.toml 依赖
func parseCargoToml(scanner *bufio.Scanner, result *PreAnalysisResult) {
	inDeps := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "[dependencies]" || line == "[dev-dependencies]" {
			inDeps = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inDeps = false
			continue
		}
		if inDeps && line != "" && !strings.HasPrefix(line, "#") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				name := strings.TrimSpace(parts[0])
				result.RawDeps[name] = ""
			}
		}
	}
}

// parseComposerJSON 简单提取 composer.json 依赖
func parseComposerJSON(depFile string, result *PreAnalysisResult) {
	data, err := os.ReadFile(depFile)
	if err != nil {
		return
	}
	content := string(data)
	re := regexp.MustCompile(`"([a-zA-Z0-9/-]+)"\s*:\s*"[^"]*"`)
	matches := re.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		name := m[1]
		if name == "name" || name == "type" || name == "description" || name == "license" {
			continue
		}
		result.RawDeps[name] = ""
	}
}

// classifyDeps 根据关键词分类依赖到安全相关类别
func classifyDeps(result *PreAnalysisResult) {
	classified := make(map[string]bool) // 避免重复分类

	for depName := range result.RawDeps {
		depLower := strings.ToLower(depName)

		for category, keywords := range securityDepKeywords {
			for _, kw := range keywords {
				if strings.Contains(depLower, strings.ToLower(kw)) {
					if classified[category+":"+depName] {
						continue
					}
					classified[category+":"+depName] = true

					switch category {
					case "db":
						result.DBLibs = append(result.DBLibs, depName)
					case "http":
						result.HTTPFrameworks = append(result.HTTPFrameworks, depName)
					case "auth":
						result.AuthLibs = append(result.AuthLibs, depName)
					case "crypto":
						result.CryptoLibs = append(result.CryptoLibs, depName)
					case "exec":
						result.ExecLibs = append(result.ExecLibs, depName)
					case "file":
						result.FileOpsLibs = append(result.FileOpsLibs, depName)
				case "deser":
					result.DBLibs = append(result.DBLibs, depName) // 归入 db 类，反序列化
				case "template":
					result.TemplateLibs = append(result.TemplateLibs, depName)
					}
					break
				}
			}
		}
	}

	// 补充 HTTP 框架到 Frameworks
	for _, fw := range result.HTTPFrameworks {
		result.Frameworks = append(result.Frameworks, fw)
	}
}

// scanProjectScale 扫描项目规模
func scanProjectScale(projectPath, language string) ProjectScaleInfo {
	scale := ProjectScaleInfo{}
	topDirs := make(map[string]bool)

	langExts := map[string][]string{
		"go":         {".go"},
		"java":       {".java", ".kt", ".scala"},
		"python":     {".py"},
		"javascript": {".js", ".ts", ".jsx", ".tsx"},
		"php":        {".php"},
		"ruby":       {".rb"},
		"rust":       {".rs"},
		"c":          {".c", ".h"},
		"cpp":        {".cpp", ".cc", ".cxx", ".h", ".hpp"},
	}

	exts := langExts[language]
	if exts == nil {
		exts = []string{".go", ".java", ".py", ".js", ".ts", ".php", ".rb", ".rs", ".c", ".cpp"}
	}

	filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(projectPath, path)
		if rel == "." {
			return nil
		}
		// 跳过隐藏目录和 vendor/node_modules（只对目录生效）
		for _, skip := range []string{"vendor", "node_modules", ".git", "__pycache__", "testdata"} {
			if info.IsDir() && (strings.HasPrefix(rel, skip+string(os.PathSeparator)) || rel == skip) {
				return filepath.SkipDir
			}
		}
		// 跳过测试文件
		if !info.IsDir() && (strings.Contains(rel, "_test.") || strings.Contains(rel, "/test/")) {
			return nil
		}

		if info.IsDir() {
			// 记录顶层目录
			depth := strings.Count(rel, string(os.PathSeparator))
			if depth == 0 {
				topDirs[rel] = true
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		for _, e := range exts {
			if ext == e {
				scale.TotalFiles++
				scale.TotalLines += 50 // 粗略估算每文件 50 行
				break
			}
		}
		return nil
	})

	scale.TopDirs = len(topDirs)
	return scale
}

// findEntryPoints 查找项目入口点
func findEntryPoints(projectPath, language string) []EntryPointInfo {
	var entries []EntryPointInfo

	patterns := map[string][]struct {
		Pattern string
		Type    string
		Name    string
	}{
		"go": {
			{`func main()`, "main", "main"},
			{`func main(`, "main", "main"},
		},
		"java": {
			{`public static void main(`, "main", "main"},
			{`@SpringBootApplication`, "main", "SpringBootApp"},
			{`@RestController`, "http_handler", "RestController"},
			{`@Controller`, "http_handler", "Controller"},
		},
		"python": {
			{`if __name__`, "main", "__main__"},
			{`app = Flask(`, "http_handler", "FlaskApp"},
			{`app = FastAPI(`, "http_handler", "FastAPIApp"},
		},
		"javascript": {
			{`app.listen(`, "http_handler", "Server"},
			{`createServer(`, "http_handler", "Server"},
			{`module.exports`, "main", "ModuleExport"},
		},
		"php": {
			{`Route::`, "http_handler", "LaravelRoute"},
			{`$app->`, "http_handler", "SlimRoute"},
		},
		"rust": {
			{`fn main()`, "main", "main"},
			{`#[tokio::main]`, "main", "async_main"},
			{`#[actix_web::main]`, "main", "actix_main"},
		},
	}

	langExts := map[string][]string{
		"go":         {".go"},
		"java":       {".java"},
		"python":     {".py"},
		"javascript": {".js", ".ts"},
		"php":        {".php"},
		"rust":       {".rs"},
	}

	exts := langExts[language]
	pats := patterns[language]
	if exts == nil || pats == nil {
		return entries
	}

	filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(projectPath, path)
		if rel == "." {
			return nil
		}
		// 跳过隐藏目录和 vendor/node_modules
		for _, skip := range []string{".", "vendor", "node_modules", ".git", "__pycache__", "testdata"} {
			if strings.HasPrefix(rel, skip+string(os.PathSeparator)) || strings.HasPrefix(rel, skip+"/") {
				return nil
			}
		}
		if strings.Contains(rel, "_test.") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		matched := false
		for _, e := range exts {
			if ext == e {
				matched = true
				break
			}
		}
		if !matched {
			return nil
		}

		// 读取文件搜索入口模式
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			for _, pat := range pats {
				if strings.Contains(line, pat.Pattern) {
					entries = append(entries, EntryPointInfo{
						File: path,
						Line: lineNo,
						Type: pat.Type,
						Name: pat.Name,
					})
				}
			}
		}
		return nil
	})

	return entries
}

// findConfigFiles 查找关键配置文件
func findConfigFiles(projectPath, language string) []string {
	var configs []string
	configNames := []string{
		"Dockerfile", "docker-compose.yml", "docker-compose.yaml",
		".env", ".env.example", ".env.local",
		"Makefile", "Taskfile.yml",
		".gitignore", ".gitleaks.toml",
		"config.yaml", "config.yml", "config.json", "config.toml",
		"settings.py", "application.yml", "application.properties",
	}

	for _, name := range configNames {
		path := filepath.Join(projectPath, name)
		if _, err := os.Stat(path); err == nil {
			configs = append(configs, path)
		}
	}
	return configs
}

// FormatPreAnalysisForPrompt 将预分析结果格式化为可读文本，注入 prompt
func FormatPreAnalysisForPrompt(r *PreAnalysisResult) string {
	var sb strings.Builder

	sb.WriteString("## 确定性预分析结果\n\n")

	sb.WriteString(fmt.Sprintf("- **主语言**: %s", r.Language))
	if r.LanguageVer != "" {
		sb.WriteString(fmt.Sprintf(" %s", r.LanguageVer))
	}
	sb.WriteString("\n")

	if len(r.Frameworks) > 0 {
		sb.WriteString(fmt.Sprintf("- **框架/库**: %s\n", strings.Join(uniqueStrings(r.Frameworks), ", ")))
	}
	if len(r.HTTPFrameworks) > 0 {
		sb.WriteString(fmt.Sprintf("- **HTTP 框架**: %s\n", strings.Join(uniqueStrings(r.HTTPFrameworks), ", ")))
	}
	if len(r.DBLibs) > 0 {
		sb.WriteString(fmt.Sprintf("- **数据库库**: %s\n", strings.Join(uniqueStrings(r.DBLibs), ", ")))
	}
	if len(r.AuthLibs) > 0 {
		sb.WriteString(fmt.Sprintf("- **认证库**: %s\n", strings.Join(uniqueStrings(r.AuthLibs), ", ")))
	}
	if len(r.CryptoLibs) > 0 {
		sb.WriteString(fmt.Sprintf("- **加密库**: %s\n", strings.Join(uniqueStrings(r.CryptoLibs), ", ")))
	}
	if len(r.ExecLibs) > 0 {
		sb.WriteString(fmt.Sprintf("- **命令执行库**: %s\n", strings.Join(uniqueStrings(r.ExecLibs), ", ")))
	}

	sb.WriteString(fmt.Sprintf("- **项目规模**: %d 文件, ~%d 行, %d 顶层目录\n",
		r.ProjectScale.TotalFiles, r.ProjectScale.TotalLines, r.ProjectScale.TopDirs))

	if len(r.EntryPoints) > 0 {
		sb.WriteString("- **入口点**:\n")
		for _, ep := range r.EntryPoints {
			sb.WriteString(fmt.Sprintf("  - `%s:%d` (%s) %s\n", ep.File, ep.Line, ep.Type, ep.Name))
		}
	}

	if len(r.ConfigFiles) > 0 {
		sb.WriteString(fmt.Sprintf("- **配置文件**: %s\n", strings.Join(r.ConfigFiles, ", ")))
	}

	if r.DepFilePath != "" {
		sb.WriteString(fmt.Sprintf("- **依赖文件**: %s (%d 个依赖)\n", r.DepFilePath, len(r.RawDeps)))
	}

	return sb.String()
}

func uniqueStrings(input []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range input {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
