package ssadb

type GraphValue interface {
	GetId() int64
	GetEffectOnGraphValues() GraphValues
	GetDependOnGraphValues() GraphValues
	GetGraphPredecessors() GraphPredecessors
	GetVerboseName() string
}

type GraphValues []GraphValue

type GraphPredecessors []*GraphPredecessor

type GraphPredecessor struct {
	GraphValue GraphValue
	Step       int
	Label      string
}
