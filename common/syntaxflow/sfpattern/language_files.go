package sfpattern

import (
	"path"
	"strings"
)

// sourceLanguageExtensions is the file ceiling for a source rule that declares
// a language. The ceiling applies only to the blanket file filter ("*"): a
// rule that names its own extensions keeps them, because a curated list such
// as "/\.(js|mjs|cjs|jsx)$/" is more precise than this table.
//
// A python rule must not read a .java file, even when the rule itself asks for
// "every file". Listing code plus the configuration formats that belong to the
// language keeps that promise without hand-editing 170 rules.
var sourceLanguageExtensions = map[string][]string{
	"java":       {".java", ".jsp", ".jspx", ".jspf", ".tag", ".properties", ".xml"},
	"python":     {".py", ".pyw", ".pyi", ".ipynb"},
	"js":         {".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".vue", ".svelte"},
	"ts":         {".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".vue", ".svelte"},
	"php":        {".php", ".phtml", ".php3", ".php4", ".php5", ".php7", ".inc", ".twig"},
	"csharp":     {".cs", ".cshtml", ".razor", ".aspx", ".ascx", ".asmx", ".config"},
	"golang":     {".go"},
	"c":          {".c", ".h"},
	"ruby":       {".rb", ".erb", ".rake", ".gemspec", ".ru"},
	"terraform":  {".tf", ".tfvars", ".hcl"},
	"solidity":   {".sol"},
	"swift":      {".swift"},
	"kotlin":     {".kt", ".kts"},
	"scala":      {".scala", ".sc"},
	"kubernetes": {".yaml", ".yml"},
	"yak":        {".yak"},
}

// sourceLanguageAliases maps the spellings rules use to the registry key.
var sourceLanguageAliases = map[string]string{
	"java":       "java",
	"python":     "python",
	"py":         "python",
	"js":         "js",
	"javascript": "js",
	"ecmascript": "js",
	"node":       "js",
	"nodejs":     "js",
	"ts":         "ts",
	"typescript": "ts",
	"php":        "php",
	"csharp":     "csharp",
	"cs":         "csharp",
	"golang":     "golang",
	"go":         "golang",
	"c":          "c",
	"ruby":       "ruby",
	"rb":         "ruby",
	"terraform":  "terraform",
	"solidity":   "solidity",
	"swift":      "swift",
	"kotlin":     "kotlin",
	"kt":         "kotlin",
	"scala":      "scala",
	"kubernetes": "kubernetes",
	"k8s":        "kubernetes",
	"yak":        "yak",
}

// languageAllowsPath reports whether a source rule of the given language may
// read file. An empty, "general" or unknown language allows every file: the
// ceiling only narrows languages this package knows.
func languageAllowsPath(language, file string) bool {
	key := normalizeSourceLanguage(language)
	if key == "" {
		return true
	}
	base := strings.ToLower(path.Base(strings.ReplaceAll(file, `\`, "/")))
	if key == "dockerfile" {
		return isDockerfileName(base)
	}
	for _, ext := range sourceLanguageExtensions[key] {
		if strings.HasSuffix(base, ext) {
			return true
		}
	}
	return false
}

func normalizeSourceLanguage(language string) string {
	lower := strings.ToLower(strings.TrimSpace(language))
	if lower == "" || lower == "general" {
		return ""
	}
	if lower == "dockerfile" || lower == "docker" {
		return "dockerfile"
	}
	if key, ok := sourceLanguageAliases[lower]; ok {
		return key
	}
	return ""
}

func isDockerfileName(base string) bool {
	if base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") {
		return true
	}
	return strings.HasSuffix(base, ".dockerfile")
}
