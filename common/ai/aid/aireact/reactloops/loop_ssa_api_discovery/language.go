package loop_ssa_api_discovery

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// LanguageReconcileResult captures how the effective SSA language was chosen.
type LanguageReconcileResult struct {
	Language ssaconfig.Language
	Detected ssaconfig.Language
	Hint     string
	Source   string // detected | hint
	Warnings []string
}

// DetectLanguage walks shallow markers under root to pick a SSA language.
func DetectLanguage(root string) (ssaconfig.Language, error) {
	if utils.FileExists(filepath.Join(root, "pom.xml")) ||
		utils.FileExists(filepath.Join(root, "build.gradle")) ||
		utils.FileExists(filepath.Join(root, "build.gradle.kts")) {
		return ssaconfig.JAVA, nil
	}
	if utils.FileExists(filepath.Join(root, "go.mod")) {
		return ssaconfig.GO, nil
	}
	if utils.FileExists(filepath.Join(root, "package.json")) {
		return ssaconfig.JS, nil
	}
	if utils.FileExists(filepath.Join(root, "composer.json")) {
		return ssaconfig.PHP, nil
	}
	if utils.FileExists(filepath.Join(root, "pyproject.toml")) ||
		utils.FileExists(filepath.Join(root, "setup.py")) ||
		utils.FileExists(filepath.Join(root, "requirements.txt")) {
		return ssaconfig.PYTHON, nil
	}
	if utils.FileExists(filepath.Join(root, "CMakeLists.txt")) {
		return ssaconfig.C, nil
	}
	matches, _ := filepath.Glob(filepath.Join(root, "*.yak"))
	if len(matches) > 0 {
		return ssaconfig.Yak, nil
	}
	return "", utils.Errorf("cannot detect language under %s; set Language: in user input", root)
}

// ReconcileLanguage prefers build markers (DetectLanguage) over user/AI hint when they conflict.
func ReconcileLanguage(root, hint string) (LanguageReconcileResult, error) {
	hint = strings.TrimSpace(utils.RemoveUnprintableChars(hint))
	detected, derr := DetectLanguage(root)
	if derr != nil {
		if hint == "" {
			return LanguageReconcileResult{}, derr
		}
		lang, err := ssaconfig.ValidateLanguage(hint)
		if err != nil {
			return LanguageReconcileResult{}, err
		}
		return LanguageReconcileResult{Language: lang, Hint: hint, Source: "hint"}, nil
	}
	if hint == "" {
		return LanguageReconcileResult{Language: detected, Detected: detected, Source: "detected"}, nil
	}
	hintLang, err := ssaconfig.ValidateLanguage(hint)
	if err != nil {
		return LanguageReconcileResult{}, err
	}
	if strings.EqualFold(string(hintLang), string(detected)) {
		return LanguageReconcileResult{
			Language: detected,
			Detected: detected,
			Hint:     hint,
			Source:   "detected",
		}, nil
	}
	warn := fmt.Sprintf(
		"language hint %q conflicts with build markers; using detected %q",
		hint, detected,
	)
	return LanguageReconcileResult{
		Language: detected,
		Detected: detected,
		Hint:     hint,
		Source:   "detected",
		Warnings: []string{warn},
	}, nil
}

// ResolveLanguage validates hint or falls back to detection (markers win on conflict).
func ResolveLanguage(root, hint string) (ssaconfig.Language, error) {
	rec, err := ReconcileLanguage(root, hint)
	if err != nil {
		return "", err
	}
	return rec.Language, nil
}
