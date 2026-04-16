package scannode

import "strings"

const capabilityKeySSARuleSyncExport = "ssa.rule_sync.export"

func normalizeScanNodeCapabilityKeys(input []string) []string {
	result := make([]string, 0, len(input)+len(compiledScanNodeCapabilityKeys()))
	seen := make(map[string]struct{}, len(input)+len(compiledScanNodeCapabilityKeys()))

	appendKey := func(key string) {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			return
		}
		if _, exists := seen[trimmed]; exists {
			return
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}

	for _, key := range compiledScanNodeCapabilityKeys() {
		appendKey(key)
	}
	for _, key := range input {
		appendKey(key)
	}
	return result
}
