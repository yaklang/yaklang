package sfvm

import (
	"github.com/gobwas/glob"
	"regexp"
)

type ValueOperator interface {
	GetName() string
	IsMap() bool
	IsList() bool

	// ExactMatch return ops, for OpPushSearchExact
	ExactMatch(string) (bool, ValueOperator, error)
	// GlobMatch return opts, for OpPushSearchGlob
	GlobMatch(glob.Glob) (bool, ValueOperator, error)
	// RegexpMatch for OpPushSearchRegexp
	RegexpMatch(*regexp.Regexp) (bool, ValueOperator, error)

	// GetCallActualParams for OpGetCallArgs
	GetCallActualParams() (ValueOperator, error)

	// GetMembers for list or objct
	GetMembers() (ValueOperator, error)

	// GetTopDef and GetBottomUse is for OpBottomUse
	GetSyntaxFlowTopDef() (ValueOperator, error)
	GetSyntaxFlowBottomUse() (ValueOperator, error)

	// ListIndex for OpListIndex, like a[1] a must be list...
	ListIndex(i int) (ValueOperator, error)
}
