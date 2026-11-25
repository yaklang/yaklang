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
	PHP     Language = "php"
	JAVA    Language = "java"
	GO      Language = "golang"
	C       Language = "c"
	CPP     Language = "cpp"
	TS      Language = "typescript"
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

// CanonicalLanguage resolves every accepted spelling to the single language
// value. It is the alias layer: javascript, js, ts and typescript all name the
// TypeScript frontend, the same way go and golang name the golang value.
func CanonicalLanguage(l Language) Language {
	if normalized, err := ValidateLanguage(string(l)); err == nil {
		return normalized
	}
	return l
}

// SameLanguage reports whether two language ids name the same language. Stored
// rules may still carry legacy spellings such as js or javascript, so both
// sides are canonicalized before they are compared.
func SameLanguage(a, b Language) bool {
	return CanonicalLanguage(a) == CanonicalLanguage(b)
}

func GetAllSupportedLanguages() []string {
	return []string{
		string(Yak),
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
	case "js", "es", "javascript", "ecmascript", "nodejs", "node", "node.js", "ts", "typescript":
		return TS, nil
	case "go", "golang":
		return GO, nil
	case "c", "clang":
		return C, nil
	case "cpp", "c++", "cplusplus", "cc", "cxx":
		return CPP, nil
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
