package yaklib

import (
	"bytes"
	"os"
	"regexp"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// MaliciousSignature 恶意文件特征
type MaliciousSignature struct {
	Name        string `json:"name"`         // 特征名称
	Category    string `json:"category"`     // 分类（如：php_webshell, privilege_escalation, backdoor等）
	Pattern     string `json:"pattern"`      // 正则表达式模式
	BytePattern []byte `json:"byte_pattern"` // 字节特征
	Description string `json:"description"`  // 描述信息
	Severity    string `json:"severity"`     // 严重程度（low, medium, high, critical）
}

// MaliciousFileMatcher 恶意文件特征匹配器
type MaliciousFileMatcher struct {
	mu         sync.RWMutex
	signatures map[string]*MaliciousSignature // 按名称索引
	patterns   map[string]*regexp.Regexp      // 编译后的正则表达式
	byCategory map[string][]string            // 按分类索引特征名称
}

// NewMaliciousFileMatcher 创建一个恶意文件特征匹配器，内置常见 webshell/恶意代码特征
// 返回值:
//   - 恶意文件特征匹配器对象
//
// Example:
// ```
// matcher = file.NewMaliciousFileMatcher()
// matches, err = matcher.MatchContent([]byte("<?php eval($_POST[1]);?>"))
// ```
func NewMaliciousFileMatcher() *MaliciousFileMatcher {
	matcher := &MaliciousFileMatcher{
		signatures: make(map[string]*MaliciousSignature),
		patterns:   make(map[string]*regexp.Regexp),
		byCategory: make(map[string][]string),
	}
	// 加载默认特征库
	matcher.loadDefaultSignatures()
	return matcher
}

// AddSignature 添加恶意文件特征
// 支持两种输入类型：
//   - *MaliciousSignature: 结构体指针
//   - map[string]interface{}: yaklang 字典（会自动转换）
func (m *MaliciousFileMatcher) AddSignature(sig interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var signature *MaliciousSignature

	// 处理不同类型的输入
	switch v := sig.(type) {
	case *MaliciousSignature:
		signature = v
	case map[string]string:
		// 从 map 创建 MaliciousSignature
		signature = &MaliciousSignature{}
		if name, ok := v["name"]; ok {
			signature.Name = utils.InterfaceToString(name)
		}
		if category, ok := v["category"]; ok {
			signature.Category = utils.InterfaceToString(category)
		}
		if pattern, ok := v["pattern"]; ok {
			signature.Pattern = utils.InterfaceToString(pattern)
		}
		if desc, ok := v["description"]; ok {
			signature.Description = utils.InterfaceToString(desc)
		}
		if severity, ok := v["severity"]; ok {
			signature.Severity = utils.InterfaceToString(severity)
		}
		if bytePattern, ok := v["byte_pattern"]; ok {
			// 使用 InterfaceToBytesSlice 处理字节数组
			content := utils.InterfaceToBytesSlice(bytePattern)
			if len(content) > 0 {
				signature.BytePattern = content[0]
			}
		}
	default:
		return utils.Errorf("unsupported signature type: %T, expected *MaliciousSignature or map[string]interface{}", sig)
	}

	if signature == nil {
		return utils.Errorf("signature cannot be nil")
	}

	if signature.Name == "" {
		return utils.Errorf("signature name cannot be empty")
	}

	// 编译正则表达式
	if signature.Pattern != "" {
		re, err := regexp.Compile(signature.Pattern)
		if err != nil {
			return utils.Errorf("invalid regex pattern for %s: %v", signature.Name, err)
		}
		m.patterns[signature.Name] = re
	}

	m.signatures[signature.Name] = signature

	// 按分类索引
	if signature.Category != "" {
		m.byCategory[signature.Category] = append(m.byCategory[signature.Category], signature.Name)
	}

	return nil
}

// RemoveSignature 移除特征
func (m *MaliciousFileMatcher) RemoveSignature(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sig, ok := m.signatures[name]
	if !ok {
		return
	}

	delete(m.signatures, name)
	delete(m.patterns, name)

	// 从分类索引中移除
	if sig.Category != "" {
		names := m.byCategory[sig.Category]
		for i, n := range names {
			if n == name {
				m.byCategory[sig.Category] = append(names[:i], names[i+1:]...)
				break
			}
		}
	}
}

// MatchFile 匹配文件内容，返回匹配到的特征名称列表
func (m *MaliciousFileMatcher) MatchFile(filePath string) ([]string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return m.MatchContent(content), nil
}

// MatchContent 匹配内容，返回匹配到的特征名称列表
func (m *MaliciousFileMatcher) MatchContent(content []byte) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	matches := make([]string, 0)
	matched := make(map[string]bool) // 去重

	// 匹配正则表达式
	for name, re := range m.patterns {
		if re.Match(content) {
			if !matched[name] {
				matches = append(matches, name)
				matched[name] = true
			}
		}
	}

	// 匹配字节特征
	for name, sig := range m.signatures {
		if len(sig.BytePattern) > 0 && bytes.Contains(content, sig.BytePattern) {
			if !matched[name] {
				matches = append(matches, name)
				matched[name] = true
			}
		}
	}

	return matches
}

// MatchFileWithDetails 匹配文件并返回详细信息
func (m *MaliciousFileMatcher) MatchFileWithDetails(filePath string) ([]*MaliciousSignature, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return m.MatchContentWithDetails(content), nil
}

// MatchContentWithDetails 匹配内容并返回详细信息
func (m *MaliciousFileMatcher) MatchContentWithDetails(content []byte) []*MaliciousSignature {
	m.mu.RLock()
	defer m.mu.RUnlock()

	matches := make([]*MaliciousSignature, 0)
	matched := make(map[string]bool) // 去重

	// 匹配正则表达式
	for name, re := range m.patterns {
		if re.Match(content) {
			if !matched[name] {
				if sig, ok := m.signatures[name]; ok {
					matches = append(matches, sig)
					matched[name] = true
				}
			}
		}
	}

	// 匹配字节特征
	for name, sig := range m.signatures {
		if len(sig.BytePattern) > 0 && bytes.Contains(content, sig.BytePattern) {
			if !matched[name] {
				matches = append(matches, sig)
				matched[name] = true
			}
		}
	}

	return matches
}

// GetSignaturesByCategory 按分类获取特征
func (m *MaliciousFileMatcher) GetSignaturesByCategory(category string) []*MaliciousSignature {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sigs := make([]*MaliciousSignature, 0)
	if names, ok := m.byCategory[category]; ok {
		for _, name := range names {
			if sig, ok := m.signatures[name]; ok {
				sigs = append(sigs, sig)
			}
		}
	}
	return sigs
}

// GetAllCategories 获取所有分类
func (m *MaliciousFileMatcher) GetAllCategories() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	categories := make([]string, 0, len(m.byCategory))
	for cat := range m.byCategory {
		categories = append(categories, cat)
	}
	return categories
}

// GetSignature 获取指定特征
func (m *MaliciousFileMatcher) GetSignature(name string) *MaliciousSignature {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.signatures[name]
}

// loadDefaultSignatures 加载默认恶意文件特征库
func (m *MaliciousFileMatcher) loadDefaultSignatures() {
	sigs, err := loadSignaturesFromEmbed()
	if err != nil {
		panic(err)
	}
	for _, sig := range sigs {
		_ = m.AddSignature(sig)
	}
}
