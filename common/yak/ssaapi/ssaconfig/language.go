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
	case "general":
		return General, nil
	}
	return "", errors.Errorf("unsupported language: %s", language)
}
