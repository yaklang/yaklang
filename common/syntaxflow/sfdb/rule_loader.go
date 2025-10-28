package sfdb

import (
	"context"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// RuleLoader 规则加载器接口
// 用于从不同来源（数据库、RemoteFS、OSS等）加载SyntaxFlow规则
type RuleLoader interface {
	// LoadRules 根据筛选条件加载规则列表
	// ctx: 上下文，用于取消和超时控制
	// filter: 规则筛选条件（语言、用途、严重程度等）
	// 返回: 规则列表和错误
	LoadRules(ctx context.Context, filter *ypb.SyntaxFlowRuleFilter) ([]*schema.SyntaxFlowRule, error)

	// LoadRuleByName 根据规则名称加载单个规则
	// ctx: 上下文
	// ruleName: 规则名称
	// 返回: 规则对象和错误
	LoadRuleByName(ctx context.Context, ruleName string) (*schema.SyntaxFlowRule, error)

	// YieldRules 流式加载规则（通过channel）
	// 适用于大量规则的场景，避免一次性加载到内存
	// ctx: 上下文
	// filter: 规则筛选条件
	// 返回: 规则项的channel
	YieldRules(ctx context.Context, filter *ypb.SyntaxFlowRuleFilter) <-chan *RuleItem

	// GetLoaderType 返回加载器类型
	// 用于标识加载器的来源类型
	GetLoaderType() RuleLoaderType

	// Close 关闭加载器，释放资源
	// 用于清理缓存、关闭连接等
	Close() error
}

// RuleItem 规则项，包含规则和可能的错误
// 用于YieldRules方法的返回值
type RuleItem struct {
	Rule  *schema.SyntaxFlowRule // 规则对象，如果加载成功
	Error error                  // 错误信息，如果加载失败
}

// RuleLoaderType 规则加载器类型
type RuleLoaderType string

const (
	LoaderTypeDatabase RuleLoaderType = "database" // 数据库加载器
	LoaderTypeRemoteFS RuleLoaderType = "remotefs" // 远程文件系统加载器
	LoaderTypeOSS      RuleLoaderType = "oss"      // OSS对象存储加载器
	LoaderTypeHybrid   RuleLoaderType = "hybrid"   // 混合加载器（多来源）
)

// String 返回加载器类型的字符串表示
func (t RuleLoaderType) String() string {
	return string(t)
}

// IsValid 检查加载器类型是否有效
func (t RuleLoaderType) IsValid() bool {
	switch t {
	case LoaderTypeDatabase, LoaderTypeRemoteFS, LoaderTypeOSS, LoaderTypeHybrid:
		return true
	default:
		return false
	}
}
