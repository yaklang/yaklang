package interaction

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const ComponentContentType = "application/vnd.memfit.ui+json"

const MaxCatalogComponents = 512

type ComponentDefinition struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Schema      map[string]any            `json:"schema"`
	Children    bool                      `json:"children,omitempty"`
	Bindings    []string                  `json:"bindings,omitempty"`
	Actions     map[string]map[string]any `json:"actions,omitempty"`
}

type Catalog struct {
	Version            string                `json:"version"`
	CompatibleVersions []string              `json:"compatibleVersions,omitempty"`
	Components         []ComponentDefinition `json:"components"`
}

// SupportsVersion permits explicitly declared additive upgrades when replaying
// older documents. Unknown required components still force a text fallback.
func (c Catalog) SupportsVersion(version string) bool {
	if version == "" {
		return false
	}
	if version == c.Version {
		return true
	}
	for _, supported := range c.CompatibleVersions {
		if version == supported {
			return true
		}
	}
	return false
}

type ComponentAction struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type ComponentNode struct {
	ID        string            `json:"id"`
	Component string            `json:"component"`
	Props     map[string]any    `json:"props"`
	Children  []string          `json:"children,omitempty"`
	Bindings  map[string]string `json:"bindings,omitempty"`
	Action    *ComponentAction  `json:"action,omitempty"`
}

type ComponentDocument struct {
	ID                 string          `json:"id"`
	CatalogVersion     string          `json:"catalogVersion"`
	Fallback           string          `json:"fallback"`
	Root               string          `json:"root"`
	Nodes              []ComponentNode `json:"nodes"`
	RequiredComponents []string        `json:"requiredComponents"`
}

type Diagnostic struct {
	NodeID  string `json:"nodeId"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Catalog schemas are self-contained data. Never resolve external references
// supplied by a client/model, or allow executable UI extensions.
func CheckSchema(value any, depth int) error {
	if depth > 24 {
		return fmt.Errorf("schema exceeds nesting limit")
	}
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			switch key {
			case "$ref", "$dynamicRef", "$recursiveRef", "$id", "__proto__", "constructor", "prototype":
				return fmt.Errorf("unsupported schema key %s", key)
			}
			if err := CheckSchema(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := CheckSchema(child, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func ValidateSchema(s map[string]any, value any) error {
	schema, err := compileSchema(s)
	if err != nil {
		return err
	}
	return schema.Validate(value)
}

func compileSchema(s map[string]any) (*jsonschema.Schema, error) {
	// JSON Schema validators expect JSON-shaped arrays/numbers, including when
	// definitions originate in Go rather than a decoded manifest.
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	if err := CheckSchema(s, 0); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", s); err != nil {
		return nil, err
	}
	return compiler.Compile("schema.json")
}

func (c Catalog) Definition(name string) *ComponentDefinition {
	for i := range c.Components {
		if c.Components[i].Name == name {
			return &c.Components[i]
		}
	}
	return nil
}

func (c Catalog) Validate() error {
	if c.Version == "" || len(c.Version) > 64 || len(c.Components) < 1 || len(c.Components) > MaxCatalogComponents {
		return fmt.Errorf("invalid component catalog")
	}
	names := map[string]bool{}
	for _, entry := range c.Components {
		if entry.Name == "" || len(entry.Name) > 64 || names[entry.Name] || entry.Schema["type"] != "object" {
			return fmt.Errorf("invalid component definition")
		}
		names[entry.Name] = true
		if err := CheckSchema(entry.Schema, 0); err != nil {
			return err
		}
		for name, schema := range entry.Actions {
			if name != "submit_to_chat" {
				return fmt.Errorf("unsupported UI action %s", name)
			}
			if err := CheckSchema(schema, 0); err != nil {
				return err
			}
		}
	}
	return nil
}

type missingUIValue struct{}

func normalize(s map[string]any, value any, path string, diagnose func(string, string)) (any, bool) {
	if _, missing := value.(missingUIValue); missing {
		if fallback, exists := s["default"]; exists && ValidateSchema(s, fallback) == nil {
			return fallback, true
		}
		diagnose(path, "required value is missing")
		return nil, false
	}
	if props, ok := value.(map[string]any); ok && s["type"] == "object" {
		owned := make(map[string]any, len(props))
		for key, value := range props {
			owned[key] = value
		}
		props = owned
		definitions, _ := s["properties"].(map[string]any)
		required := map[string]bool{}
		if values, ok := s["required"].([]any); ok {
			for _, key := range values {
				if key, ok := key.(string); ok {
					required[key] = true
				}
			}
		}
		for key := range required {
			if _, exists := props[key]; !exists {
				props[key] = missingUIValue{}
			}
		}
		for key, definition := range definitions {
			if child, ok := definition.(map[string]any); ok {
				if _, hasDefault := child["default"]; hasDefault {
					if _, exists := props[key]; !exists {
						props[key] = missingUIValue{}
					}
				}
			}
		}
		result := map[string]any{}
		for key, input := range props {
			if key == "__proto__" || key == "constructor" || key == "prototype" {
				diagnose(path+"."+key, "unsafe property removed")
				continue
			}
			child, ok := definitions[key].(map[string]any)
			if !ok {
				child, ok = s["additionalProperties"].(map[string]any)
			}
			if !ok {
				if s["additionalProperties"] == false {
					diagnose(path+"."+key, "undeclared property removed")
				} else {
					result[key] = input
				}
				continue
			}
			next, valid := normalize(child, input, path+"."+key, diagnose)
			if valid {
				result[key] = next
			} else if required[key] {
				return nil, false
			}
		}
		value = result
	}
	if values, ok := value.([]any); ok && s["type"] == "array" {
		if itemSchema, ok := s["items"].(map[string]any); ok {
			items := make([]any, len(values))
			for i, item := range values {
				var valid bool
				items[i], valid = normalize(itemSchema, item, fmt.Sprintf("%s[%d]", path, i), diagnose)
				if !valid {
					return nil, false
				}
			}
			value = items
		}
	}
	if err := ValidateSchema(s, value); err != nil {
		diagnose(path, "value does not match schema")
		return nil, false
	}
	return value, true
}

func (c Catalog) Normalize(doc ComponentDocument) (ComponentDocument, []Diagnostic) {
	var diagnostics []Diagnostic
	if doc.ID == "" || len(doc.ID) > 128 || !c.SupportsVersion(doc.CatalogVersion) || strings.TrimSpace(doc.Fallback) == "" || len(doc.Nodes) > 64 {
		doc.Nodes = nil
		return doc, []Diagnostic{{Path: "document", Message: "require stable id, matching catalogVersion, fallback text and at most 64 nodes"}}
	}
	for _, name := range doc.RequiredComponents {
		if c.Definition(name) == nil {
			doc.Nodes = nil
			return doc, []Diagnostic{{Path: "requiredComponents", Message: "required component is unavailable; use fallback"}}
		}
	}
	var nodes []ComponentNode
	ids, components := map[string]bool{}, map[string]bool{}
	for _, node := range doc.Nodes {
		diagnose := func(path, message string) {
			diagnostics = append(diagnostics, Diagnostic{NodeID: node.ID, Path: path, Message: message})
		}
		if node.ID == "" || len(node.ID) > 128 || ids[node.ID] {
			diagnose("id", "missing or duplicate stable node id")
			continue
		}
		ids[node.ID] = true
		entry := c.Definition(node.Component)
		if entry == nil {
			diagnose("component", "unknown component removed")
			continue
		}
		props, ok := normalize(entry.Schema, node.Props, "props", diagnose)
		if !ok {
			continue
		}
		node.Props = props.(map[string]any)
		if len(node.Children) > 0 && !entry.Children {
			node.Children = nil
			diagnose("children", "component does not accept children")
		}
		for key, value := range node.Bindings {
			if key != "value" || len(entry.Bindings) == 0 || len(value) > 64 {
				delete(node.Bindings, key)
				diagnose("bindings", "unsupported binding removed")
			}
		}
		if node.Action != nil {
			actionSchema, exists := entry.Actions[node.Action.Name]
			if node.Action.Name != "submit_to_chat" || !exists || ValidateSchema(actionSchema, node.Action.Args) != nil {
				node.Action = nil
				diagnose("action", "invalid action disabled")
			}
		}
		if !NormalizeVisualization(&node, diagnose) || !NormalizeNavigation(&node, diagnose) || !normalizeForm(node, diagnose) {
			continue
		}
		components[node.Component] = true
		nodes = append(nodes, node)
	}
	doc.Nodes = nodes
	doc.RequiredComponents = nil
	for name := range components {
		doc.RequiredComponents = append(doc.RequiredComponents, name)
	}
	sort.Strings(doc.RequiredComponents)
	byID := map[string]ComponentNode{}
	for _, node := range nodes {
		byID[node.ID] = node
	}
	visited := map[string]bool{}
	var visit func(string, int) bool
	visit = func(id string, depth int) bool {
		node, exists := byID[id]
		if !exists || visited[id] || depth > 24 {
			diagnostics = append(diagnostics, Diagnostic{NodeID: id, Path: "children", Message: "missing, repeated or cyclic reference"})
			return false
		}
		visited[id] = true
		var children []string
		for _, child := range node.Children {
			if visit(child, depth+1) {
				children = append(children, child)
			}
		}
		node.Children = children
		byID[id] = node
		return true
	}
	if !visit(doc.Root, 0) {
		doc.Nodes = nil
	} else {
		doc.Nodes = nil
		for _, node := range nodes {
			if visited[node.ID] {
				doc.Nodes = append(doc.Nodes, byID[node.ID])
			}
		}
	}
	return doc, diagnostics
}

func (doc ComponentDocument) SubmissionText(receipt Submission, catalog Catalog) (string, error) {
	var err error
	receipt, err = doc.NormalizeSubmission(receipt)
	if err != nil {
		return "", err
	}
	for _, node := range doc.Nodes {
		if node.ID != receipt.NodeID {
			continue
		}
		entry := catalog.Definition(node.Component)
		if entry == nil || node.Action == nil || node.Action.Name != "submit_to_chat" {
			return "", fmt.Errorf("node has no authorized submit action")
		}
		actionSchema, exists := entry.Actions[node.Action.Name]
		if !exists || ValidateSchema(actionSchema, node.Action.Args) != nil {
			return "", fmt.Errorf("action is not allowed by the catalog")
		}
		if node.Component == "DataTable" {
			return tableSubmissionText(node, receipt)
		}
		schema, ok := node.Props["schema"].(map[string]any)
		if !ok || ValidateSchema(schema, receipt.Values) != nil {
			return "", fmt.Errorf("form values do not match the declared schema")
		}
		encoded, _ := json.MarshalIndent(receipt.Values, "", "  ")
		title, _ := node.Props["title"].(string)
		return "表单回答：" + title + "\n" + string(encoded), nil
	}
	return "", fmt.Errorf("node does not belong to this component block")
}
