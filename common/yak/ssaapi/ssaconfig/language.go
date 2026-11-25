package ssaconfig

import (
	"strings"

	"github.com/pkg/errors"
)

type Language string

func (l Language) String() string {
	return string(l)
}

const (
	Yak     Language = "yak"
	JS      Language = "js"
	PHP     Language = "php"
	JAVA    Language = "java"
	GO      Language = "golang"
	C       Language = "c"
	CPP     Language = "cpp"
	TS      Language = "ts"
	PYTHON  Language = "python"
	CSHARP  Language = "csharp"
	BASH    Language = "bash"
	LUA     Language = "lua"
	RUST    Language = "rust"
	KOTLIN  Language = "kotlin"
	SCALA   Language = "scala"
	General Language = "general"
)

func (l Language) GetFileExt() string {
	switch l {
	case Yak:
		return ".yak"
	case JAVA:
		return ".java"
	case GO:
		return ".go"
	case TS:
		return ".ts"
	case JS:
		return ".js"
	case C:
		return ".c"
	case CPP:
		return ".cpp"
	case PHP:
		return ".php"
	case PYTHON:
		return ".py"
	case CSHARP:
		return ".cs"
	case BASH:
		return ".sh"
	case LUA:
		return ".lua"
	case RUST:
		return ".rs"
	case KOTLIN:
		return ".kt"
	case SCALA:
		return ".scala"
	default:
		return ""
	}
}

// SameLanguage reports whether two language ids share one frontend. JavaScript
// has no frontend of its own: it compiles through the TypeScript builder, so a
// js rule and a ts program describe one language family.
func SameLanguage(a, b Language) bool {
	if a == b {
		return true
	}
	return isECMAScript(a) && isECMAScript(b)
}

func isECMAScript(l Language) bool {
	return l == JS || l == TS
}

func GetAllSupportedLanguages() []string {
	return []string{
		string(Yak),
		string(JS),
		string(PHP),
		string(JAVA),
		string(GO),
		string(C),
		string(CPP),
		string(TS),
		string(PYTHON),
		string(CSHARP),
		string(BASH),
		string(LUA),
		string(RUST),
		string(KOTLIN),
		string(SCALA),
	}
}

func ValidateLanguage(language string) (Language, error) {
	switch strings.TrimSpace(strings.ToLower(language)) {
	case "":
		return "", nil // empty language
	case "yak", "yaklang":
		return Yak, nil
	case "java":
		return JAVA, nil
	case "php":
		return PHP, nil
	case "js", "es", "javascript", "ecmascript", "nodejs", "node", "node.js":
		return JS, nil
	case "go", "golang":
		return GO, nil
	case "c", "clang":
		return C, nil
	case "cpp", "c++", "cplusplus", "cc", "cxx":
		return CPP, nil
	case "ts", "typescript":
		return TS, nil
	case "python", "py":
		return PYTHON, nil
	case "csharp", "c#", "cs", ".net", "dotnet":
		return CSHARP, nil
	case "bash", "sh", "shell":
		return BASH, nil
	case "lua":
		return LUA, nil
	case "rust", "rs":
		return RUST, nil
	case "kotlin", "kt", "kts":
		return KOTLIN, nil
	case "scala", "sc":
		return SCALA, nil
	case "general":
		return General, nil
	}
	return "", errors.Errorf("unsupported language: %s", language)
}
