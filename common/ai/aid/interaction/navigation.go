package interaction

// NormalizeNavigation turns tab content references into graph edges so that
// the same reachability, cycle and depth checks apply to every container.
func NormalizeNavigation(node *ComponentNode, diagnose func(string, string)) bool {
	if node.Component != "Tabs" {
		return true
	}
	items, ok := uiObjects(node.Props["items"])
	if !ok || len(items) == 0 || !uniqueUIField(items, "id") || !uniqueUIField(items, "content") {
		diagnose("props.items", "tab ids and content references must be unique")
		return false
	}
	children := make([]string, 0, len(items))
	defaultTab, hasDefault := node.Props["defaultTab"].(string)
	defaultExists := false
	for _, item := range items {
		children = append(children, item["content"].(string))
		defaultExists = defaultExists || item["id"] == defaultTab
	}
	if node.Children != nil {
		matches := len(node.Children) == len(children)
		for i, child := range node.Children {
			if i >= len(children) || children[i] != child {
				matches = false
				break
			}
		}
		if !matches {
			diagnose("children", "tab children are derived from items.content; corrected")
		}
	}
	node.Children = children
	if hasDefault && !defaultExists {
		delete(node.Props, "defaultTab")
		diagnose("props.defaultTab", "unknown default tab; use the first item")
	}
	return true
}
