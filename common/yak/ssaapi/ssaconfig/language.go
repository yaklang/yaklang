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

func GetAllSupportedLanguages() []string {
	return []string{
		string(Yak),
		string(JS),
		string(PHP),
		string(JAVA),
		string(GO),
		string(C),
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
