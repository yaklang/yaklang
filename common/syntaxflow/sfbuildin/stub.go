//go:build no_syntaxflow
// +build no_syntaxflow

package sfbuildin

// Stub functions when SyntaxFlow support is excluded

func SyncEmbedRule(notifies ...func(process float64, ruleName string)) error {
	return nil
}

func ForceSyncEmbedRule(notifies ...func(process float64, ruleName string)) error {
	return nil
}

func SyncRuleFromFileSystem(fs interface{}, buildin bool, notifies ...func(float64, string)) error {
	return nil
}

func SyntaxFlowRuleHash() (string, error) {
	return "", nil
}
