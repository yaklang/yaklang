package sfdb

import (
	"context"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// OSSRuleLoader OSS规则加载器
// 从OSS对象存储服务读取.sf规则文件
type OSSRuleLoader struct {
	ossClient    OSSClient                         // OSS客户端
	bucket       string                            // OSS bucket名称
	prefix       string                            // 规则文件前缀
	cache        map[string]*schema.SyntaxFlowRule // 单个规则缓存（LoadRuleByName使用）
	allRules     []*schema.SyntaxFlowRule          // 所有规则缓存（LoadRules使用）
	allRulesCond *sync.Cond                        // 用于同步allRules的加载
	allRulesErr  error                             // 加载所有规则时的错误
	enableCache  bool                              // 是否启用缓存
	mu           sync.RWMutex                      // 保护cache和allRules的读写锁
}

// OSSLoaderOption OSS加载器选项
type OSSLoaderOption func(*OSSRuleLoader)

// WithOSSBucket 设置OSS bucket
func WithOSSBucket(bucket string) OSSLoaderOption {
	return func(l *OSSRuleLoader) {
		l.bucket = bucket
	}
}

// WithOSSPrefix 设置规则文件前缀
func WithOSSPrefix(prefix string) OSSLoaderOption {
	return func(l *OSSRuleLoader) {
		l.prefix = prefix
	}
}

// WithOSSCache 设置是否启用缓存
func WithOSSCache(enable bool) OSSLoaderOption {
	return func(l *OSSRuleLoader) {
		l.enableCache = enable
	}
}

// NewOSSRuleLoader 创建OSS规则加载器
func NewOSSRuleLoader(ossClient OSSClient, opts ...OSSLoaderOption) *OSSRuleLoader {
	loader := &OSSRuleLoader{
		ossClient:   ossClient,
		bucket:      "yaklang-rules", // 默认bucket
		prefix:      "syntaxflow/",   // 默认前缀
		cache:       make(map[string]*schema.SyntaxFlowRule),
		enableCache: true,
	}
	loader.allRulesCond = sync.NewCond(&loader.mu)

	for _, opt := range opts {
		opt(loader)
	}

	return loader
}

// LoadRules 根据筛选条件加载规则列表
func (l *OSSRuleLoader) LoadRules(ctx context.Context, filter *ypb.SyntaxFlowRuleFilter) ([]*schema.SyntaxFlowRule, error) {
	// 检查上下文
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 如果启用了缓存，尝试从缓存加载所有规则
	if l.enableCache {
		allRules, err := l.loadAllRulesWithCache(ctx)
		if err != nil {
			return nil, err
		}

		// 应用筛选条件
		filtered := make([]*schema.SyntaxFlowRule, 0)
		for _, rule := range allRules {
			if l.matchFilter(rule, filter) {
				filtered = append(filtered, rule)
			}
		}
		return filtered, nil
	}

	// 未启用缓存时，直接从OSS加载
	return l.loadRulesFromOSS(ctx, filter)
}

// loadAllRulesWithCache 加载所有规则（带缓存）
func (l *OSSRuleLoader) loadAllRulesWithCache(ctx context.Context) ([]*schema.SyntaxFlowRule, error) {
	// 先尝试读锁检查缓存
	l.mu.RLock()
	if l.allRules != nil {
		defer l.mu.RUnlock()
		return l.allRules, l.allRulesErr
	}
	l.mu.RUnlock()

	// 切换到写锁进行加载
	l.mu.Lock()
	defer l.mu.Unlock()

	// 双重检查：可能在等待写锁期间，其他goroutine已经加载了
	if l.allRules != nil {
		return l.allRules, l.allRulesErr
	}

	// 执行实际的加载
	rules, err := l.loadRulesFromOSS(ctx, nil)
	l.allRules = rules
	l.allRulesErr = err

	// 同时填充单个规则缓存
	if err == nil {
		for _, rule := range rules {
			l.cache[rule.RuleName] = rule
		}
	}

	return l.allRules, l.allRulesErr
}

// loadRulesFromOSS 从OSS加载规则（无缓存）
func (l *OSSRuleLoader) loadRulesFromOSS(ctx context.Context, filter *ypb.SyntaxFlowRuleFilter) ([]*schema.SyntaxFlowRule, error) {
	rules := make([]*schema.SyntaxFlowRule, 0)

	// 列出OSS中的规则文件
	objects, err := l.ossClient.ListObjects(l.bucket, l.prefix)
	if err != nil {
		return nil, utils.Wrapf(err, "list oss objects failed")
	}

	for _, obj := range objects {
		// 检查上下文
		select {
		case <-ctx.Done():
			return rules, ctx.Err()
		default:
		}

		// 只处理 .sf 文件
		if !strings.HasSuffix(obj.Key, ".sf") {
			continue
		}

		rule, err := l.loadRuleFromOSS(ctx, obj.Key)
		if err != nil {
			log.Errorf("load rule from oss %s failed: %v", obj.Key, err)
			continue
		}

		// 应用筛选条件
		if filter == nil || l.matchFilter(rule, filter) {
			rules = append(rules, rule)
		}
	}

	return rules, nil
}

// LoadRuleByName 根据规则名称加载单个规则
func (l *OSSRuleLoader) LoadRuleByName(ctx context.Context, ruleName string) (*schema.SyntaxFlowRule, error) {
	// 检查上下文
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 如果启用缓存，先尝试从缓存读取
	if l.enableCache {
		l.mu.RLock()
		// 先检查单个规则缓存
		if rule, ok := l.cache[ruleName]; ok {
			l.mu.RUnlock()
			return rule, nil
		}
		// 如果allRules已经加载，从中查找
		if l.allRules != nil {
			l.mu.RUnlock()
			for _, rule := range l.allRules {
				if rule.RuleName == ruleName {
					return rule, nil
				}
			}
			return nil, utils.Errorf("rule %s not found", ruleName)
		}
		l.mu.RUnlock()
	}

	// 构造OSS key
	ossKey := l.prefix + ruleName + ".sf"
	rule, err := l.loadRuleFromOSS(ctx, ossKey)
	if err != nil {
		return nil, err
	}

	// 更新缓存
	if l.enableCache {
		l.mu.Lock()
		l.cache[ruleName] = rule
		l.mu.Unlock()
	}

	return rule, nil
}

// YieldRules 流式加载规则
func (l *OSSRuleLoader) YieldRules(ctx context.Context, filter *ypb.SyntaxFlowRuleFilter) <-chan *RuleItem {
	ch := make(chan *RuleItem, 10)

	go func() {
		defer close(ch)

		// 如果启用缓存，优先从缓存加载
		if l.enableCache {
			allRules, err := l.loadAllRulesWithCache(ctx)
			if err != nil {
				ch <- &RuleItem{Error: err}
				return
			}

			for _, rule := range allRules {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if l.matchFilter(rule, filter) {
					ch <- &RuleItem{Rule: rule}
				}
			}
			return
		}

		// 未启用缓存时，直接从OSS流式加载
		objects, err := l.ossClient.ListObjects(l.bucket, l.prefix)
		if err != nil {
			ch <- &RuleItem{Error: utils.Wrapf(err, "list objects failed")}
			return
		}

		for _, obj := range objects {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if !strings.HasSuffix(obj.Key, ".sf") {
				continue
			}

			rule, err := l.loadRuleFromOSS(ctx, obj.Key)
			if err != nil {
				ch <- &RuleItem{Error: err}
				continue
			}

			if l.matchFilter(rule, filter) {
				ch <- &RuleItem{Rule: rule}
			}
		}
	}()

	return ch
}

// loadRuleFromOSS 从OSS加载规则
func (l *OSSRuleLoader) loadRuleFromOSS(ctx context.Context, ossKey string) (*schema.SyntaxFlowRule, error) {
	// 检查上下文
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 从OSS获取规则文件内容
	content, err := l.ossClient.GetObject(l.bucket, ossKey)
	if err != nil {
		return nil, utils.Wrapf(err, "get object %s from oss failed", ossKey)
	}

	// 解析规则内容
	rule, err := CheckSyntaxFlowRuleContent(string(content))
	if err != nil {
		return nil, utils.Wrapf(err, "parse rule content failed")
	}

	// 设置规则名称（从OSS key提取）
	parts := strings.Split(ossKey, "/")
	fileName := parts[len(parts)-1]
	ruleName := strings.TrimSuffix(fileName, ".sf")
	rule.RuleName = ruleName

	return rule, nil
}

// matchFilter 检查规则是否匹配筛选条件
func (l *OSSRuleLoader) matchFilter(rule *schema.SyntaxFlowRule, filter *ypb.SyntaxFlowRuleFilter) bool {
	if filter == nil {
		return true
	}

	// 规则名称筛选
	if len(filter.RuleNames) > 0 {
		matched := false
		for _, name := range filter.RuleNames {
			if rule.RuleName == name {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// 语言筛选
	if len(filter.Language) > 0 {
		matched := false
		for _, lang := range filter.Language {
			if rule.Language == lang {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// 用途筛选
	if len(filter.Purpose) > 0 {
		matched := false
		for _, purpose := range filter.Purpose {
			if string(rule.Purpose) == purpose {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// 严重程度筛选
	if len(filter.Severity) > 0 {
		matched := false
		for _, severity := range filter.Severity {
			if string(rule.Severity) == severity {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// 标签筛选
	if len(filter.Tag) > 0 {
		matched := false
		for _, tag := range filter.Tag {
			if rule.Tag == tag {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// 关键词搜索
	if filter.Keyword != "" {
		keyword := strings.ToLower(filter.Keyword)
		searchText := strings.ToLower(rule.RuleName + " " + rule.Title + " " + rule.TitleZh + " " + rule.Description + " " + rule.Content)
		if !strings.Contains(searchText, keyword) {
			return false
		}
	}

	// Lib规则筛选
	if filter.FilterLibRuleKind != "" {
		switch filter.FilterLibRuleKind {
		case "noLib":
			if rule.AllowIncluded {
				return false
			}
		case "onlyLib":
			if !rule.AllowIncluded {
				return false
			}
		}
	}

	return true
}

// GetLoaderType 返回加载器类型
func (l *OSSRuleLoader) GetLoaderType() RuleLoaderType {
	return LoaderTypeOSS
}

// Close 关闭加载器
func (l *OSSRuleLoader) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 清空所有缓存
	l.cache = make(map[string]*schema.SyntaxFlowRule)
	l.allRules = nil
	l.allRulesErr = nil

	// 关闭OSS客户端
	return l.ossClient.Close()
}

// String 返回加载器的字符串表示
func (l *OSSRuleLoader) String() string {
	return "OSSRuleLoader{bucket=" + l.bucket + ", prefix=" + l.prefix + "}"
}
