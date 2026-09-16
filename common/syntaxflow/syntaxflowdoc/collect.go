package syntaxflowdoc

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

var (
	libNameQuotedRe = regexp.MustCompile(`(?i)\blib\s*:\s*"([^"]+)"`)
	libNameSingleRe = regexp.MustCompile(`(?i)\blib\s*:\s*'([^']+)'`)
	titleQuotedRe   = regexp.MustCompile(`(?i)\btitle\s*:\s*"([^"]+)"`)
	langQuotedRe    = regexp.MustCompile(`(?i)\blanguage\s*:\s*"([^"]+)"`)
)

// NativeCallInfo is a serializable NativeCall documentation row (no function pointer).
type NativeCallInfo struct {
	Name        string
	Description string
}

// CollectNativeCalls adds NativeCall rows into the helper.
func CollectNativeCalls(h *DocumentHelper, items []NativeCallInfo) {
	if h == nil {
		return
	}
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		desc := strings.TrimSpace(item.Description)
		if desc == "" {
			desc = "SyntaxFlow NativeCall <" + name + ">"
		}
		h.NativeCalls[name] = &DocEntry{
			Category:    CategoryNativeCall,
			Name:        name,
			Title:       "<" + name + ">",
			Description: desc,
			Example:     "$v<" + name + "> as $out",
		}
	}
}

// CollectBuiltinLibsFromDir scans a directory tree of .sf files for desc(lib: "...").
func CollectBuiltinLibsFromDir(h *DocumentHelper, root string) error {
	if h == nil {
		return utils.Error("document helper is nil")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return utils.Error("builtin lib root is empty")
	}
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return utils.Errorf("builtin lib root is not a directory: %s", root)
	}

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "testdata" || name == "buildin-rule-test" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".sf") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		content := string(raw)
		libName := extractLibName(content)
		if libName == "" {
			return nil
		}
		rel := path
		if r, relErr := filepath.Rel(root, path); relErr == nil {
			rel = filepath.ToSlash(r)
		}
		title := extractFirst(titleQuotedRe, content)
		lang := extractFirst(langQuotedRe, content)
		desc := "Builtin SyntaxFlow include library."
		if title != "" {
			desc = title
		}
		if _, exists := h.BuiltinLibs[libName]; exists {
			return nil
		}
		h.BuiltinLibs[libName] = &DocEntry{
			Category:    CategoryBuiltinLib,
			Name:        libName,
			Title:       libName,
			Description: desc,
			Example:     "<include('" + libName + "')> as $lib",
			Path:        rel,
			Language:    lang,
		}
		return nil
	})
}

func extractLibName(content string) string {
	if m := libNameQuotedRe.FindStringSubmatch(content); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	if m := libNameSingleRe.FindStringSubmatch(content); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func extractFirst(re *regexp.Regexp, content string) string {
	if m := re.FindStringSubmatch(content); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// DefaultBuiltinRulesDir returns common/syntaxflow/sfbuildin/buildin under the repo.
func DefaultBuiltinRulesDir() string {
	root := GetProjectPath()
	if root == "" {
		return ""
	}
	return filepath.Join(root, "common", "syntaxflow", "sfbuildin", "buildin")
}
