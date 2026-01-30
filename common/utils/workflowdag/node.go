package workflowdag

import (
	"context"
	"fmt"
)

// NodeStatus represents the execution status of a DAG node
type NodeStatus int

const (
	NodeStatusPending NodeStatus = iota
	NodeStatusProcessing
	NodeStatusCompleted
	NodeStatusFailed
	NodeStatusSkipped
)

func (s NodeStatus) String() string {
	switch s {
	case NodeStatusPending:
		return "pending"
	case NodeStatusProcessing:
		return "processing"
	case NodeStatusCompleted:
		return "completed"
	case NodeStatusFailed:
		return "failed"
	case NodeStatusSkipped:
		return "skipped"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// IsDone returns true if the status represents a terminal state
func (s NodeStatus) IsDone() bool {
	return s == NodeStatusCompleted || s == NodeStatusFailed || s == NodeStatusSkipped
}

// DAGNode defines the basic interface for a workflow DAG node
type DAGNode interface {
	// GetID returns the unique identifier of the node
	GetID() string

	// DependsOn returns the IDs of nodes this node depends on
	// If A.DependsOn() returns ["B", "C"], it means B and C must execute before A
	DependsOn() []string

	// AllowFailed returns whether this node can continue execution
	// even if its dependencies failed
	AllowFailed() bool

	// IsDone returns true if the node has finished execution
	IsDone() bool

	// IsProcessing returns true if the node is currently executing
	IsProcessing() bool

	// IsFailed returns true if the node execution failed
	IsFailed() bool

	// IsSkipped returns true if the node was skipped
	IsSkipped() bool
}

// ExecutableNode extends DAGNode with execution capabilities
type ExecutableNode interface {
	DAGNode

	// Execute runs the node's task
	Execute(ctx context.Context) error

	// SetStatus sets the node's execution status
	SetStatus(status NodeStatus)

	// GetStatus returns the node's current status
	GetStatus() NodeStatus
}

// BaseNode provides a basic implementation of DAGNode
type BaseNode struct {
	ID          string
	Dependencies []string
	Status      NodeStatus
	AllowFail   bool
}

func NewBaseNode(id string, deps ...string) *BaseNode {
	return &BaseNode{
		ID:          id,
		Dependencies: deps,
		Status:      NodeStatusPending,
		AllowFail:   false,
	}
}

func (n *BaseNode) GetID() string {
	return n.ID
}

func (n *BaseNode) DependsOn() []string {
	return n.Dependencies
}

func (n *BaseNode) AllowFailed() bool {
	return n.AllowFail
}

func (n *BaseNode) IsDone() bool {
	return n.Status == NodeStatusCompleted
}

func (n *BaseNode) IsProcessing() bool {
	return n.Status == NodeStatusProcessing
}

func (n *BaseNode) IsFailed() bool {
	return n.Status == NodeStatusFailed
}

func (n *BaseNode) IsSkipped() bool {
	return n.Status == NodeStatusSkipped
}

func (n *BaseNode) SetStatus(status NodeStatus) {
	n.Status = status
}

func (n *BaseNode) GetStatus() NodeStatus {
	return n.Status
}
