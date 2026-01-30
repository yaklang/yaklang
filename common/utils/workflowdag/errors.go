package workflowdag

import "errors"

var (
	// ErrNodeNotFound is returned when a node with the specified ID is not found
	ErrNodeNotFound = errors.New("node not found")

	// ErrDuplicateNode is returned when adding a node with an ID that already exists
	ErrDuplicateNode = errors.New("duplicate node ID")

	// ErrDependencyNotFound is returned when a node depends on a non-existent node
	ErrDependencyNotFound = errors.New("dependency node not found")

	// ErrDAGNotBuilt is returned when trying to access DAG features before Build() is called
	ErrDAGNotBuilt = errors.New("DAG not built yet, call Build() first")

	// ErrEmptyDAG is returned when the DAG has no nodes
	ErrEmptyDAG = errors.New("DAG has no nodes")
)
