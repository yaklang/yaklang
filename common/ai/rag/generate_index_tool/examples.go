package generate_index_tool

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
)

// ScriptIndexableItem 将 YakScript 适配为 IndexableItem
type ScriptIndexableItem struct {
	script *schema.YakScript
}

// NewScriptIndexableItem 创建脚本索引项
func NewScriptIndexableItem(script *schema.YakScript) *ScriptIndexableItem {
	return &ScriptIndexableItem{
		script: script,
	}
}

// GetKey 获取脚本的唯一标识符
func (s *ScriptIndexableItem) GetKey() string {
	return s.script.ScriptName
}

// GetContent 获取用于生成向量的内容
func (s *ScriptIndexableItem) GetContent() (string, error) {
	var contentParts []string

	if s.script.ScriptName != "" {
		contentParts = append(contentParts, fmt.Sprintf("名称: %s", s.script.ScriptName))
	}
	if s.script.Help != "" {
		contentParts = append(contentParts, fmt.Sprintf("描述: %s", s.script.Help))
	}
	if s.script.Author != "" {
		contentParts = append(contentParts, fmt.Sprintf("作者: %s", s.script.Author))
	}
	if s.script.Tags != "" {
		contentParts = append(contentParts, fmt.Sprintf("标签: %s", s.script.Tags))
	}
	if s.script.Type != "" {
		contentParts = append(contentParts, fmt.Sprintf("类型: %s", s.script.Type))
	}

	if len(contentParts) == 0 {
		return fmt.Sprintf("脚本名称: %s", s.script.ScriptName), nil
	}

	return strings.Join(contentParts, "\n"), nil
}

// GetMetadata 获取元数据信息
func (s *ScriptIndexableItem) GetMetadata() map[string]interface{} {
	return map[string]interface{}{
		"id":          s.script.ID,
		"script_name": s.script.ScriptName,
		"type":        s.script.Type,
		"author":      s.script.Author,
		"tags":        s.script.Tags,
		"level":       s.script.Level,
		"ignored":     s.script.Ignored,
		"created_at":  s.script.CreatedAt,
		"updated_at":  s.script.UpdatedAt,
	}
}

// GetDisplayName 获取显示名称
func (s *ScriptIndexableItem) GetDisplayName() string {
	return s.script.ScriptName
}

// ConvertScriptsToIndexableItems 将脚本列表转换为可索引项列表
func ConvertScriptsToIndexableItems(scripts []*schema.YakScript) []IndexableItem {
	items := make([]IndexableItem, len(scripts))
	for i, script := range scripts {
		items[i] = NewScriptIndexableItem(script)
	}
	return items
}
