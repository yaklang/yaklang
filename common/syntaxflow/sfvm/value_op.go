package sfvm

import (
	"github.com/yaklang/yaklang/common/log"
	"regexp"
	"strings"
)

type RecursiveConfigKey string

const (
	RecursiveConfig_Depth    RecursiveConfigKey = "depth"
	RecursiveConfig_DepthMin RecursiveConfigKey = "depth_min"
	RecursiveConfig_DepthMax RecursiveConfigKey = "depth_max"
	RecursiveConfig_Exclude  RecursiveConfigKey = "exclude"
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
	default:
		log.Warnf("unknown recursive config key: %s", i)
	}
	return ""
}

type RecursiveConfigItem struct {
	Key            string
	Value          string
	SyntaxFlowRule bool
}

type ValueOperator interface {
	String() string
	IsMap() bool
	IsList() bool

	// Recursive will execute with handler for every list or map
	Recursive(func(ValueOperator) error) error

	// ExactMatch return ops, for OpPushSearchExact
	ExactMatch(bool, string) (bool, ValueOperator, error)
	// GlobMatch return opts, for OpPushSearchGlob
	GlobMatch(bool, Glob) (bool, ValueOperator, error)
	// RegexpMatch for OpPushSearchRegexp
	RegexpMatch(bool, *regexp.Regexp) (bool, ValueOperator, error)

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
