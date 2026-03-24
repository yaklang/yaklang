package browser

import (
	"fmt"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

var interactiveRoles = map[string]bool{
	"button":           true,
	"link":             true,
	"textbox":          true,
	"combobox":         true,
	"checkbox":         true,
	"radio":            true,
	"switch":           true,
	"slider":           true,
	"spinbutton":       true,
	"tab":              true,
	"menuitem":         true,
	"option":           true,
	"treeitem":         true,
	"searchbox":        true,
	"menuitemcheckbox": true,
	"menuitemradio":    true,
}

type SnapshotResult struct {
	Text      string
	RefMap    *RefMap
	NodeCount int
}

type snapshotNode struct {
	nodeID        string
	role          string
	name          string
	backendNodeID int
	ignored       bool
	children      []*snapshotNode
	properties    map[string]string
}

func takeSnapshot(page *rod.Page, refMap *RefMap) (*SnapshotResult, error) {
	//err := proto.AccessibilityEnable{}.Call(page)
	//if err != nil {
	//	return nil, fmt.Errorf("enable accessibility domain: %w", err)
	//}
	//
	//result, err := proto.AccessibilityGetFullAXTree{}.Call(page)
	//if err != nil {
	//	return nil, fmt.Errorf("get full AX tree: %w", err)
	//}

	result, err := accessibilityGetPageFullAXTree(page)
	if err != nil {
		return nil, err
	}

	if len(result.Nodes) == 0 {
		return &SnapshotResult{Text: "(empty page)", RefMap: refMap}, nil
	}

	nodeMap := make(map[string]*proto.AccessibilityAXNode, len(result.Nodes))
	for _, node := range result.Nodes {
		nodeMap[string(node.NodeID)] = node
	}

	var rootNode *proto.AccessibilityAXNode
	for _, node := range result.Nodes {
		if node.ParentID == "" {
			rootNode = node
			break
		}
	}
	if rootNode == nil && len(result.Nodes) > 0 {
		rootNode = result.Nodes[0]
	}

	refMap.Reset()

	tree := buildSnapshotTree(rootNode, nodeMap)
	if tree == nil {
		return &SnapshotResult{Text: "(empty page)", RefMap: refMap}, nil
	}

	assignRefs(tree, refMap)

	var sb strings.Builder
	renderTree(tree, 0, &sb)

	return &SnapshotResult{
		Text:      sb.String(),
		RefMap:    refMap,
		NodeCount: len(result.Nodes),
	}, nil
}

func buildSnapshotTree(node *proto.AccessibilityAXNode, nodeMap map[string]*proto.AccessibilityAXNode) *snapshotNode {
	if node == nil {
		return nil
	}

	sn := &snapshotNode{
		nodeID:        string(node.NodeID),
		ignored:       node.Ignored,
		backendNodeID: int(node.BackendDOMNodeID),
		properties:    make(map[string]string),
	}

	if node.Role != nil {
		sn.role = axValueString(node.Role)
	}
	if node.Name != nil {
		sn.name = axValueString(node.Name)
	}

	for _, prop := range node.Properties {
		propName := string(prop.Name)
		propVal := axValueString(prop.Value)
		switch propName {
		case "checked":
			if propVal == "true" {
				sn.properties["checked"] = "true"
			}
		case "expanded":
			sn.properties["expanded"] = propVal
		case "level":
			sn.properties["level"] = propVal
		case "disabled":
			if propVal == "true" {
				sn.properties["disabled"] = "true"
			}
		case "required":
			if propVal == "true" {
				sn.properties["required"] = "true"
			}
		case "selected":
			if propVal == "true" {
				sn.properties["selected"] = "true"
			}
		case "readonly":
			if propVal == "true" {
				sn.properties["readonly"] = "true"
			}
		}
	}

	if node.Value != nil {
		val := axValueString(node.Value)
		if val != "" {
			sn.properties["value"] = val
		}
	}

	for _, childID := range node.ChildIDs {
		childNode, ok := nodeMap[string(childID)]
		if !ok {
			continue
		}
		child := buildSnapshotTree(childNode, nodeMap)
		if child != nil {
			sn.children = append(sn.children, child)
		}
	}

	return sn
}

func assignRefs(node *snapshotNode, refMap *RefMap) {
	if node.ignored {
		for _, child := range node.children {
			assignRefs(child, refMap)
		}
		return
	}

	if interactiveRoles[node.role] && node.backendNodeID > 0 {
		ref := refMap.Assign(&RefEntry{
			BackendNodeID: node.backendNodeID,
			Role:          node.role,
			Name:          node.name,
		})
		node.properties["ref"] = ref
	}

	for _, child := range node.children {
		assignRefs(child, refMap)
	}
}

func renderTree(node *snapshotNode, depth int, sb *strings.Builder) {
	if node.ignored {
		for _, child := range node.children {
			renderTree(child, depth, sb)
		}
		return
	}

	role := node.role
	if role == "" || role == "none" || role == "generic" {
		hasVisibleChildren := false
		for _, child := range node.children {
			if !child.ignored {
				hasVisibleChildren = true
				break
			}
		}
		if !hasVisibleChildren && node.name == "" {
			return
		}
		if role == "" || role == "none" {
			for _, child := range node.children {
				renderTree(child, depth, sb)
			}
			return
		}
	}

	indent := strings.Repeat("  ", depth)
	sb.WriteString(indent)
	sb.WriteString("- ")
	sb.WriteString(role)

	if node.name != "" {
		sb.WriteString(fmt.Sprintf(" %q", node.name))
	}

	attrs := renderAttributes(node.properties)
	if attrs != "" {
		sb.WriteString(" ")
		sb.WriteString(attrs)
	}

	sb.WriteString("\n")

	for _, child := range node.children {
		renderTree(child, depth+1, sb)
	}
}

func renderAttributes(props map[string]string) string {
	if len(props) == 0 {
		return ""
	}

	var parts []string

	if ref, ok := props["ref"]; ok {
		parts = append(parts, fmt.Sprintf("ref=%s", ref))
	}

	orderedKeys := []string{"level", "checked", "selected", "expanded", "disabled", "required", "readonly", "value"}
	for _, key := range orderedKeys {
		if val, ok := props[key]; ok {
			parts = append(parts, fmt.Sprintf("%s=%s", key, val))
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func axValueString(v *proto.AccessibilityAXValue) string {
	if v == nil {
		return ""
	}
	return v.Value.Str()
}

func accessibilityGetPageFullAXTree(page *rod.Page) (*proto.AccessibilityGetFullAXTreeResult, error) {
	err := proto.AccessibilityEnable{}.Call(page)
	if err != nil {
		return nil, fmt.Errorf("enable accessibility domain: %w", err)
	}
	result, err := proto.AccessibilityGetFullAXTree{}.Call(page)
	if err != nil {
		return nil, fmt.Errorf("get full AX tree: %w", err)
	}
	return result, nil
}
