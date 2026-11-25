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
	"github.com/yaklang/yaklang/common/schema"
)

const (
	// EnvToolCallPermissions accepts one shell 规则权限策略 JSON document or a
	// JSON array of documents. Only Bash(...) rows are applied.
	EnvToolCallPermissions = "YAK_AI_TOOL_PERMISSIONS"
	// EnvToolCallPermissionsFile is an OS path-list of permission files.
	EnvToolCallPermissionsFile = "YAK_AI_TOOL_PERMISSIONS_FILE"

	toolCallPermissionsDirName  = "ai-tool-permissions"
	toolCallPermissionsFileName = "ai-tool-permissions.json"
)

// LoadRulePermissionConfigs reads runtime shell 规则权限策略 sources.
func LoadRulePermissionConfigs() []string {
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

var runtimeBashPermissionCache struct {
	mu    sync.Mutex
	key   string
	rules []*schema.SyntaxFlowRule
}

func loadRuntimeBashPermissionSyntaxFlowRules() []*schema.SyntaxFlowRule {
	configs := LoadRulePermissionConfigs()
	key := strings.Join(configs, string(rune(0)))

	runtimeBashPermissionCache.mu.Lock()
	defer runtimeBashPermissionCache.mu.Unlock()
	if runtimeBashPermissionCache.key == key {
		return runtimeBashPermissionCache.rules
	}

	var parsed []*RulePermissionRule
	for i, raw := range configs {
		rules, err := ParseRulePermissions(raw)
		if err != nil {
			log.Warnf("skip shell rule permission config %d: %v", i+1, err)
			continue
		}
		parsed = append(parsed, rules...)
	}
	runtimeBashPermissionCache.key = key
	runtimeBashPermissionCache.rules = CompileBashPermissionSyntaxFlowRules(parsed)
	return runtimeBashPermissionCache.rules
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
