package sfvm

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type RecursiveConfigKey string

const (
	RecursiveConfig_NULL     RecursiveConfigKey = ""
	RecursiveConfig_Depth                       = "depth"
	RecursiveConfig_DepthMin                    = "depth_min"
	RecursiveConfig_DepthMax                    = "depth_max"
	RecursiveConfig_Exclude                     = "exclude"
	RecursiveConfig_Until                       = "until"
)

func FormatRecursiveConfigKey(i string) RecursiveConfigKey {
	switch strings.TrimSpace(strings.ToLower(i)) {
	case "depth":
		return RecursiveConfig_Depth
	case "depth_min", "depth-min", "min_depth", "min-depth", "mindepth", "depthmin":
		return RecursiveConfig_DepthMin
	case "depth_max", "depth-max", "max_depth", "max-depth", "maxdepth", "depthmax":
		return RecursiveConfig_DepthMax
	case "exclude":
		return RecursiveConfig_Exclude
	case "until":
		return RecursiveConfig_Until
	default:
		log.Warnf("unknown recursive config key: %s", i)
	}
	return RecursiveConfig_NULL
}

type RecursiveConfigItem struct {
	Key            RecursiveConfigKey
	Value          string
	SyntaxFlowRule bool
}

// type MatchMode int
const (
	NameMatch int = 1
	KeyMatch      = 1 << 1
	BothMatch     = NameMatch | KeyMatch
)

func MatchModeString(mode int) string {
	switch mode {
	case NameMatch:
		return "name"
	case KeyMatch:
		return "key"
	case BothMatch:
		return "name+key"
	}
	return "Unknown"
}

type ValueOperator interface {
	String() string
	IsMap() bool
	IsList() bool
	GetOpcode() string
	// Len() int

	// Recursive will execute with handler for every list or map
	Recursive(func(ValueOperator) error) error

	// ExactMatch return ops, for OpPushSearchExact
	ExactMatch(int, string) (bool, ValueOperator, error)
	// GlobMatch return opts, for OpPushSearchGlob
	GlobMatch(int, ssa.Glob) (bool, ValueOperator, error)
	// RegexpMatch for OpPushSearchRegexp
	RegexpMatch(int, *regexp.Regexp) (bool, ValueOperator, error)

	// GetCallActualParams for OpGetCallArgs
	GetCalled() (ValueOperator, error)
	GetCallActualParams(int) (ValueOperator, error)
	GetAllCallActualParams() (ValueOperator, error)

	// GetTopDef and GetBottomUse is for OpBottomUse
	// use and def
	GetSyntaxFlowUse() (ValueOperator, error)
	GetSyntaxFlowDef() (ValueOperator, error)
	// top and bottom
	GetSyntaxFlowTopDef(...*RecursiveConfigItem) (ValueOperator, error)
	GetSyntaxFlowBottomUse(...*RecursiveConfigItem) (ValueOperator, error)

	// ListIndex for OpListIndex, like a[1] a must be list...
	ListIndex(i int) (ValueOperator, error)
}
