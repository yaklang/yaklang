package sfvm

import (
	"regexp"
)

type ConfigItem struct {
	Key            string
	Value          string
	SyntaxFlowRule bool
}

type ValueOperator interface {
	GetName() string
	GetNames() []string
	String() string
	IsMap() bool
	IsList() bool

	// ExactMatch return ops, for OpPushSearchExact
	ExactMatch(string) (bool, ValueOperator, error)
	// GlobMatch return opts, for OpPushSearchGlob
	GlobMatch(Glob) (bool, ValueOperator, error)
	// RegexpMatch for OpPushSearchRegexp
	RegexpMatch(*regexp.Regexp) (bool, ValueOperator, error)

	// GetCallActualParams for OpGetCallArgs
	GetCalled() (ValueOperator, error)
	GetCallActualParams(int) (ValueOperator, error)
	GetAllCallActualParams() (ValueOperator, error)

	// GetMembers for list or objct
	GetMembersByString(string) (ValueOperator, error)

	// GetTopDef and GetBottomUse is for OpBottomUse
	// use and def
	GetSyntaxFlowUse() (ValueOperator, error)
	GetSyntaxFlowDef() (ValueOperator, error)
	// top and bottom
	GetSyntaxFlowTopDef(...*ConfigItem) (ValueOperator, error)
	GetSyntaxFlowBottomUse(...*ConfigItem) (ValueOperator, error)

	// ListIndex for OpListIndex, like a[1] a must be list...
	ListIndex(i int) (ValueOperator, error)
}
