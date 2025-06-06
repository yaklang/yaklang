package ssaapi

import "context"

type OperationConfig struct {
	// 限制递归深度，每一次递归核心函数，计数器都会加一
	// 上下文计数器受到这个限制
	MaxDepth int
	MinDepth int

	// Hook
	HookEveryNode        []func(*Value) error
	AllowIgnoreCallStack bool
	ctx                  context.Context

	//用来记录上一次的值
	lastValue *Value
}

type OperationOption func(*OperationConfig)

func WithMaxDepth(maxDepth int) OperationOption {
	return func(operationConfig *OperationConfig) {
		operationConfig.MaxDepth = maxDepth
	}
}

func WithLastValue(value *Value) OperationOption {
	return func(operationConfig *OperationConfig) {
		operationConfig.lastValue = value
	}
}

func WithMinDepth(minDepth int) OperationOption {
	return func(operationConfig *OperationConfig) {
		operationConfig.MinDepth = minDepth
	}
}

func WithAllowCallStack(allowCallStack bool) OperationOption {
	return func(operationConfig *OperationConfig) {
		operationConfig.AllowIgnoreCallStack = allowCallStack
	}
}

func WithDepthLimit(depth int) OperationOption {
	return func(operationConfig *OperationConfig) {
		if depth > 0 {
			operationConfig.MaxDepth = depth
			operationConfig.MinDepth = -depth
			return
		}
		operationConfig.MaxDepth = -depth
		operationConfig.MinDepth = depth
	}
}

func WithHookEveryNode(hookNode func(*Value) error) OperationOption {
	return func(operationConfig *OperationConfig) {
		operationConfig.HookEveryNode = append(operationConfig.HookEveryNode, hookNode)
	}
}

func WithExclusiveContext(ctx context.Context) OperationOption {
	return func(operationConfig *OperationConfig) {
		if operationConfig.ctx != nil {
			operationConfig.ctx = ctx
		}
	}
}

func NewOperations(opt ...OperationOption) *OperationConfig {
	config := &OperationConfig{
		MaxDepth:             500,
		MinDepth:             -500,
		AllowIgnoreCallStack: true,
		ctx:                  context.Background(),
	}

	for _, o := range opt {
		o(config)
	}
	return config
}

func FullUseDefChain(value *Value, opts ...OperationOption) *Value {
	value.GetTopDefs(opts...)
	value.GetBottomUses()
	return value
}
