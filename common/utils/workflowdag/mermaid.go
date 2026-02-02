package workflowdag

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// MermaidFlowChartDirection represents the direction of the flowchart
type MermaidFlowChartDirection string

const (
	// MermaidDirectionTB is top to bottom direction
	MermaidDirectionTB MermaidFlowChartDirection = "TB"
	// MermaidDirectionTD is top down (same as TB)
	MermaidDirectionTD MermaidFlowChartDirection = "TD"
	// MermaidDirectionBT is bottom to top direction
	MermaidDirectionBT MermaidFlowChartDirection = "BT"
	// MermaidDirectionLR is left to right direction
	MermaidDirectionLR MermaidFlowChartDirection = "LR"
	// MermaidDirectionRL is right to left direction
	MermaidDirectionRL MermaidFlowChartDirection = "RL"
)

// MermaidFlowChartOptions contains options for generating the flowchart
type MermaidFlowChartOptions struct {
	// Direction of the flowchart
	Direction MermaidFlowChartDirection
	// NodeLabelFunc is a custom function to generate node labels
	// If nil, GetID() is used as the label
	NodeLabelFunc func(node DAGNode) string
	// Title is an optional title for the flowchart (rendered as a comment)
	Title string
	// ShowEdgeLabels controls whether to show "DependsOn" labels on edges
	// Default is true
	ShowEdgeLabels bool
	// EdgeLabelFunc is a custom function to generate edge labels
	// If nil and ShowEdgeLabels is true, "DependsOn" is used
	// The function receives (fromNodeID, toNodeID) and returns the label
	EdgeLabelFunc func(fromNodeID, toNodeID string) string
}

// DefaultMermaidOptions returns the default options for generating mermaid flowcharts
func DefaultMermaidOptions() *MermaidFlowChartOptions {
	return &MermaidFlowChartOptions{
		Direction:      MermaidDirectionTB,
		NodeLabelFunc:  nil,
		Title:          "",
		ShowEdgeLabels: true,
		EdgeLabelFunc:  nil,
	}
}

// sanitizeMermaidID converts a string to a safe Mermaid node ID
// Mermaid IDs should only contain alphanumeric characters and underscores
// We use a deterministic encoding to ensure uniqueness
func sanitizeMermaidID(id string) string {
	if id == "" {
		return "_empty_"
	}

	var sb strings.Builder
	for i, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			// First character cannot be a digit in some contexts, but Mermaid allows it
			sb.WriteRune(r)
		} else if r == '_' {
			sb.WriteString("__") // Escape underscore by doubling
		} else {
			// Encode other characters as _XXXX_ where XXXX is the Unicode code point in hex
			sb.WriteString(fmt.Sprintf("_%04X_", r))
		}

		// Limit ID length to prevent excessively long IDs
		if i > 100 {
			sb.WriteString("_etc")
			break
		}
	}

	result := sb.String()
	// Ensure the ID starts with a letter if it starts with a digit
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = "n" + result
	}

	return result
}

// escapeMermaidLabel escapes a string for use as a Mermaid node label
// The label will be wrapped in double quotes, so we need to escape special characters
func escapeMermaidLabel(label string) string {
	if label == "" {
		return `""`
	}

	var sb strings.Builder
	sb.WriteRune('"')

	for _, r := range label {
		switch r {
		case '"':
			// Escape double quotes using HTML entity
			sb.WriteString("#quot;")
		case '<':
			sb.WriteString("#lt;")
		case '>':
			sb.WriteString("#gt;")
		case '&':
			sb.WriteString("#amp;")
		case '\n':
			// Mermaid uses <br> for line breaks in labels
			sb.WriteString("<br/>")
		case '\r':
			// Skip carriage returns
		case '\t':
			sb.WriteString("    ") // Replace tab with spaces
		case '\\':
			sb.WriteString("#92;") // Backslash as decimal entity
		case '|':
			sb.WriteString("#124;") // Pipe character
		case '[':
			sb.WriteString("#91;")
		case ']':
			sb.WriteString("#93;")
		case '(':
			sb.WriteString("#40;")
		case ')':
			sb.WriteString("#41;")
		case '{':
			sb.WriteString("#123;")
		case '}':
			sb.WriteString("#125;")
		default:
			sb.WriteRune(r)
		}
	}

	sb.WriteRune('"')
	return sb.String()
}

// GenerateMermaidFlowChart generates a Mermaid flowchart representation of the DAG
// Returns the Mermaid code as a string
func (dag *WorkflowDAG[T]) GenerateMermaidFlowChart() (string, error) {
	return dag.GenerateMermaidFlowChartWithOptions(DefaultMermaidOptions())
}

// GenerateMermaidFlowChartWithOptions generates a Mermaid flowchart with custom options
func (dag *WorkflowDAG[T]) GenerateMermaidFlowChartWithOptions(opts *MermaidFlowChartOptions) (string, error) {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	if opts == nil {
		opts = DefaultMermaidOptions()
	}

	if len(dag.nodes) == 0 {
		return "", ErrEmptyDAG
	}

	var sb strings.Builder

	// Add title as comment if provided
	if opts.Title != "" {
		sb.WriteString("%% ")
		// Escape newlines in title
		escapedTitle := strings.ReplaceAll(opts.Title, "\n", " ")
		sb.WriteString(escapedTitle)
		sb.WriteString("\n")
	}

	// Write flowchart header
	direction := opts.Direction
	if direction == "" {
		direction = MermaidDirectionTB
	}
	sb.WriteString(fmt.Sprintf("flowchart %s\n", direction))

	// Get sorted node IDs for deterministic output
	nodeIDs := make([]string, 0, len(dag.nodes))
	for id := range dag.nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)

	// Track which edges we've already added to avoid duplicates
	addedEdges := make(map[string]bool)

	// Generate node definitions and edges
	for _, nodeID := range nodeIDs {
		node := dag.nodes[nodeID]

		// Generate safe ID
		safeID := sanitizeMermaidID(nodeID)

		// Get label
		var label string
		if opts.NodeLabelFunc != nil {
			label = opts.NodeLabelFunc(node)
		} else {
			label = nodeID
		}

		// Write node definition with label
		escapedLabel := escapeMermaidLabel(label)
		sb.WriteString(fmt.Sprintf("    %s[%s]\n", safeID, escapedLabel))

		// Write edges (dependencies)
		// In DAG, if A depends on B, we draw B --> A (B flows to A)
		deps := dag.deps[nodeID]
		for _, depID := range deps {
			// Only add edges for nodes that exist in the DAG
			if _, exists := dag.nodes[depID]; !exists {
				continue
			}

			safeDepID := sanitizeMermaidID(depID)
			edgeKey := safeDepID + "-->" + safeID
			if !addedEdges[edgeKey] {
				if opts.ShowEdgeLabels {
					// Get edge label
					var edgeLabel string
					if opts.EdgeLabelFunc != nil {
						edgeLabel = opts.EdgeLabelFunc(depID, nodeID)
					} else {
						edgeLabel = "DependsOn"
					}
					// Escape the edge label for Mermaid
					escapedEdgeLabel := escapeMermaidEdgeLabel(edgeLabel)
					sb.WriteString(fmt.Sprintf("    %s -->|%s| %s\n", safeDepID, escapedEdgeLabel, safeID))
				} else {
					sb.WriteString(fmt.Sprintf("    %s --> %s\n", safeDepID, safeID))
				}
				addedEdges[edgeKey] = true
			}
		}
	}

	return sb.String(), nil
}

// escapeMermaidEdgeLabel escapes a string for use as a Mermaid edge label
// Edge labels use |text| syntax, so we need to escape pipe characters and other special chars
func escapeMermaidEdgeLabel(label string) string {
	if label == "" {
		return ""
	}

	var sb strings.Builder
	for _, r := range label {
		switch r {
		case '|':
			sb.WriteString("#124;")
		case '"':
			sb.WriteString("#quot;")
		case '<':
			sb.WriteString("#lt;")
		case '>':
			sb.WriteString("#gt;")
		case '&':
			sb.WriteString("#amp;")
		case '\n':
			sb.WriteString(" ")
		case '\r':
			// Skip
		case '\t':
			sb.WriteString(" ")
		case '[':
			sb.WriteString("#91;")
		case ']':
			sb.WriteString("#93;")
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// GenerateMermaidFlowChartWithStyles generates a Mermaid flowchart with node styles
// based on node status (for ExecutableNode types)
func (dag *WorkflowDAG[T]) GenerateMermaidFlowChartWithStyles() (string, error) {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	if len(dag.nodes) == 0 {
		return "", ErrEmptyDAG
	}

	var sb strings.Builder

	// Write flowchart header
	sb.WriteString("flowchart TB\n")

	// Get sorted node IDs for deterministic output
	nodeIDs := make([]string, 0, len(dag.nodes))
	for id := range dag.nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)

	// Track which edges we've already added to avoid duplicates
	addedEdges := make(map[string]bool)

	// Group nodes by status for styling
	pendingNodes := []string{}
	processingNodes := []string{}
	completedNodes := []string{}
	failedNodes := []string{}
	skippedNodes := []string{}

	// Generate node definitions and edges
	for _, nodeID := range nodeIDs {
		node := dag.nodes[nodeID]
		safeID := sanitizeMermaidID(nodeID)

		// Write node definition
		escapedLabel := escapeMermaidLabel(nodeID)
		sb.WriteString(fmt.Sprintf("    %s[%s]\n", safeID, escapedLabel))

		// Categorize node by status for styling
		if node.IsDone() {
			completedNodes = append(completedNodes, safeID)
		} else if node.IsFailed() {
			failedNodes = append(failedNodes, safeID)
		} else if node.IsSkipped() {
			skippedNodes = append(skippedNodes, safeID)
		} else if node.IsProcessing() {
			processingNodes = append(processingNodes, safeID)
		} else {
			pendingNodes = append(pendingNodes, safeID)
		}

		// Write edges (dependencies) with "DependsOn" label
		deps := dag.deps[nodeID]
		for _, depID := range deps {
			if _, exists := dag.nodes[depID]; !exists {
				continue
			}

			safeDepID := sanitizeMermaidID(depID)
			edgeKey := safeDepID + "-->" + safeID
			if !addedEdges[edgeKey] {
				sb.WriteString(fmt.Sprintf("    %s -->|DependsOn| %s\n", safeDepID, safeID))
				addedEdges[edgeKey] = true
			}
		}
	}

	// Add style definitions
	sb.WriteString("\n")
	sb.WriteString("    %% Style definitions\n")
	sb.WriteString("    classDef pending fill:#f9f9f9,stroke:#333,stroke-width:1px\n")
	sb.WriteString("    classDef processing fill:#fff3cd,stroke:#ffc107,stroke-width:2px\n")
	sb.WriteString("    classDef completed fill:#d4edda,stroke:#28a745,stroke-width:2px\n")
	sb.WriteString("    classDef failed fill:#f8d7da,stroke:#dc3545,stroke-width:2px\n")
	sb.WriteString("    classDef skipped fill:#e2e3e5,stroke:#6c757d,stroke-width:1px\n")

	// Apply styles to nodes
	if len(pendingNodes) > 0 {
		sb.WriteString(fmt.Sprintf("    class %s pending\n", strings.Join(pendingNodes, ",")))
	}
	if len(processingNodes) > 0 {
		sb.WriteString(fmt.Sprintf("    class %s processing\n", strings.Join(processingNodes, ",")))
	}
	if len(completedNodes) > 0 {
		sb.WriteString(fmt.Sprintf("    class %s completed\n", strings.Join(completedNodes, ",")))
	}
	if len(failedNodes) > 0 {
		sb.WriteString(fmt.Sprintf("    class %s failed\n", strings.Join(failedNodes, ",")))
	}
	if len(skippedNodes) > 0 {
		sb.WriteString(fmt.Sprintf("    class %s skipped\n", strings.Join(skippedNodes, ",")))
	}

	return sb.String(), nil
}

// mermaidSyntaxValidator provides basic validation of generated Mermaid syntax
var mermaidIDPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$|^n[0-9][a-zA-Z0-9_]*$`)

// ValidateMermaidID checks if an ID is valid for Mermaid
func ValidateMermaidID(id string) bool {
	return mermaidIDPattern.MatchString(id)
}
