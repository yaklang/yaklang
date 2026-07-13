package loop_yaklangcode

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

var yakCliDeclPattern = regexp.MustCompile(`cli\.(\w+)\(\s*"([^"]+)"`)

// yakCliParamDeclarators are cli.* functions that declare a script parameter (not cli.setDefault etc.).
var yakCliParamDeclarators = map[string]struct{}{
	"String": {}, "Bool": {}, "Have": {},
	"Int": {}, "Integer": {}, "Float": {}, "Double": {},
	"StringSlice": {}, "IntSlice": {},
	"Url": {}, "Urls": {},
	"Host": {}, "Hosts": {}, "Network": {}, "Net": {},
	"Port": {}, "Ports": {},
	"File": {}, "FileNames": {}, "FolderName": {}, "FileOrContent": {}, "LineDict": {},
	"HTTPPacket": {}, "YakCode": {}, "Text": {}, "Json": {}, "YakitPlugin": {},
}

// selfTestCLIAlwaysOverride forces safe values for self-test even when the script has setDefault.
var selfTestCLIAlwaysOverride = map[string]string{
	"sqli-enable":  "false",
	"brute-enable": "false",
	"scan":         "false",
	"enable-scan":  "false",
	"enable-fuzz":  "false",
}

// selfTestCLINameMocks provides mock values by common parameter names.
var selfTestCLINameMocks = map[string]string{
	"target":      "http://127.0.0.1/",
	"url":         "http://127.0.0.1/",
	"host":        "127.0.0.1",
	"hosts":       "127.0.0.1",
	"network":     "127.0.0.1/32",
	"net":         "127.0.0.1/32",
	"sign-key":    "yaklang-self-test-mock-key",
	"token":       "yaklang-self-test-mock-token",
	"secret":      "yaklang-self-test-mock-secret",
	"method":      "POST",
	"body-params": "username=admin&password=123456",
}

// buildSelfTestCLIArgs derives subprocess CLI flags so cli.check() passes without live network.
func buildSelfTestCLIArgs(code string) []string {
	hasCliCheck := strings.Contains(code, "cli.check()") || strings.Contains(code, "cli.Check()")

	seen := map[string]bool{}
	var args []string
	add := func(name, value string) {
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" || value == "" || seen[name] {
			return
		}
		seen[name] = true
		args = append(args, "--"+name, value)
	}

	if hasCliCheck {
		for name, value := range selfTestCLIAlwaysOverride {
			add(name, value)
		}
	}

	for _, loc := range yakCliDeclPattern.FindAllStringSubmatchIndex(code, -1) {
		if len(loc) < 6 {
			continue
		}
		cliType := code[loc[2]:loc[3]]
		if _, ok := yakCliParamDeclarators[cliType]; !ok {
			continue
		}
		paramName := code[loc[4]:loc[5]]
		callStart := loc[0]
		opts := extractYakCliCallOptions(code, callStart)
		required := strings.Contains(opts, "setRequired(true)")
		hasDefault := strings.Contains(opts, "setDefault(")

		if override, ok := selfTestCLIAlwaysOverride[paramName]; ok {
			add(paramName, override)
			continue
		}
		if hasDefault && !required {
			continue
		}
		if !required && !hasCliCheck {
			continue
		}
		add(paramName, mockCLIValueForParam(cliType, paramName, required))
	}

	if hasCliCheck && !seen["target"] {
		add("target", selfTestCLINameMocks["target"])
	}
	return args
}

func extractYakCliCallOptions(code string, callStart int) string {
	open := strings.Index(code[callStart:], "(")
	if open < 0 {
		return ""
	}
	open += callStart
	close := findMatchingParen(code, open)
	if close < 0 || close <= open+1 {
		return ""
	}
	return code[open+1 : close]
}

func findMatchingParen(s string, open int) int {
	if open < 0 || open >= len(s) || s[open] != '(' {
		return -1
	}
	depth := 0
	inStr := false
	esc := false
	for i := open; i < len(s); i++ {
		ch := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if ch == '\\' {
				esc = true
				continue
			}
			if ch == '"' {
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func mockCLIValueForParam(cliType, paramName string, required bool) string {
	if v, ok := selfTestCLINameMocks[paramName]; ok {
		return v
	}
	switch strings.ToLower(cliType) {
	case "bool":
		return "false"
	case "int", "integer", "float", "double", "port", "ports":
		return "0"
	case "url", "urls":
		return "http://127.0.0.1/"
	case "host", "hosts", "network", "net":
		return "127.0.0.1"
	case "file", "filename", "foldernames":
		return createSelfTestTempFile()
	case "filenames", "linedict":
		if required {
			return createSelfTestTempFile()
		}
		return ""
	default:
		return fmt.Sprintf("yaklang-self-test-%s", paramName)
	}
}

func createSelfTestTempFile() string {
	f, err := os.CreateTemp("", "yaklang-selftest-cli-*.txt")
	if err != nil {
		return filepath.Join(os.TempDir(), "yaklang-selftest-mock.txt")
	}
	_, _ = f.WriteString("mock\n")
	_ = f.Close()
	return f.Name()
}

func resolveYakEngineBinary() (string, error) {
	if p := strings.TrimSpace(os.Getenv("YAKLANG_ENGINE_BINARY")); p != "" {
		if !utils.IsFile(p) {
			return "", utils.Errorf("YAKLANG_ENGINE_BINARY not found: %s", p)
		}
		return p, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", utils.Errorf("resolve yak engine binary: %v", err)
	}
	base := strings.ToLower(filepath.Base(exe))
	if base == "yak" || base == "yak.exe" {
		return exe, nil
	}
	return "", utils.Errorf("current binary is not yak engine (%s); set YAKLANG_ENGINE_BINARY", base)
}
