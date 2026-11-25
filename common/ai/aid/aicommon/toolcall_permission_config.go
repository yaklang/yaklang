package aicommon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

const (
	// EnvToolCallPermissions accepts either one Claude Code permissions JSON
	// document or a JSON array of documents. It takes precedence over file
	// sources, which makes a one-off engine launch easy to configure.
	EnvToolCallPermissions = "YAK_AI_TOOL_PERMISSIONS"
	// EnvToolCallPermissionsFile is an OS path-list of permissions files.
	EnvToolCallPermissionsFile = "YAK_AI_TOOL_PERMISSIONS_FILE"

	toolCallPermissionsDirName  = "ai-tool-permissions"
	toolCallPermissionsFileName = "ai-tool-permissions.json"
)

// LoadClaudeCodePermissionConfigs reads the runtime permission sources:
//
//  1. YAK_AI_TOOL_PERMISSIONS (one document or a JSON array)
//  2. YAK_AI_TOOL_PERMISSIONS_FILE (OS path-list)
//  3. <YAKIT_HOME>/ai-tool-permissions.json
//  4. <YAKIT_HOME>/ai-tool-permissions/*.json, sorted by filename
//
// Explicit environment configuration replaces file discovery; the two file
// tiers are spliced together. Invalid documents are skipped by
// CompileClaudeCodePermissionsFromRuntime so one bad file cannot disable the
// built-in guard.
func LoadClaudeCodePermissionConfigs() []string {
	if raw := strings.TrimSpace(os.Getenv(EnvToolCallPermissions)); raw != "" {
		return splitRuntimePermissionConfig(raw)
	}

	var configs []string
	if rawFiles := strings.TrimSpace(os.Getenv(EnvToolCallPermissionsFile)); rawFiles != "" {
		for _, path := range filepath.SplitList(rawFiles) {
			path = strings.TrimSpace(strings.Trim(path, `"'`))
			if path == "" {
				continue
			}
			configs = append(configs, readPermissionConfigFile(path)...)
		}
	}

	base := strings.TrimSpace(consts.GetDefaultYakitBaseDir())
	if base == "" {
		return configs
	}
	configs = append(configs, readPermissionConfigFile(filepath.Join(base, toolCallPermissionsFileName))...)

	dir := filepath.Join(base, toolCallPermissionsDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warnf("read tool permission dir %s failed: %v", dir, err)
		}
		return configs
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		configs = append(configs, readPermissionConfigFile(filepath.Join(dir, name))...)
	}
	return configs
}

// runtimePermissionCache reuses the compiled gate while the raw documents are
// unchanged. Files are still read on every call, so edits apply immediately;
// only JSON parsing and rule compilation are skipped.
var runtimePermissionCache struct {
	mu   sync.Mutex
	key  string
	gate ToolCallRuleGate
}

// CompileClaudeCodePermissionsFromRuntime builds the optional runtime gate.
// A nil result means no configuration was found; callers should fall back to
// the built-in gate.
func CompileClaudeCodePermissionsFromRuntime() ToolCallRuleGate {
	configs := LoadClaudeCodePermissionConfigs()
	if len(configs) == 0 {
		return nil
	}
	key := strings.Join(configs, string(rune(0)))

	runtimePermissionCache.mu.Lock()
	defer runtimePermissionCache.mu.Unlock()
	if runtimePermissionCache.key == key {
		return runtimePermissionCache.gate
	}

	var rules []*ToolCallPolicyRule
	for i, raw := range configs {
		parsed, err := ParseClaudeCodePermissions(raw)
		if err != nil {
			log.Warnf("skip tool permission config %d: %v", i+1, err)
			continue
		}
		rules = append(rules, parsed...)
	}
	runtimePermissionCache.key = key
	runtimePermissionCache.gate = NewToolCallPolicyGate(rules)
	return runtimePermissionCache.gate
}

func splitRuntimePermissionConfig(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "[") {
		var documents []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &documents); err == nil {
			out := make([]string, 0, len(documents))
			for _, document := range documents {
				if value := strings.TrimSpace(string(document)); value != "" && value != "null" {
					out = append(out, value)
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	return []string{raw}
}

func readPermissionConfigFile(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warnf("read tool permission config %s failed: %v", path, err)
		}
		return nil
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return nil
	}
	return []string{raw}
}
