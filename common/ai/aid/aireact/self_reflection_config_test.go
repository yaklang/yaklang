package aireact

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

// TestSelfReflectionConfig 测试自我反思配置是否正确传递到 reactloops
func TestSelfReflectionConfig(t *testing.T) {
	// 测试启用自我反思
	t.Run("EnableSelfReflection", func(t *testing.T) {
		reactIns, err := NewReAct(
			WithContext(context.Background()),
			WithEnableSelfReflection(true),
		)
		assert.NoError(t, err)
		assert.True(t, reactIns.config.GetEnableSelfReflection())

		// 创建 loop 验证配置传递
		loop, err := reactloops.CreateLoopByName(
			schema.AI_REACT_LOOP_NAME_DEFAULT,
			reactIns,
			reactloops.WithEnableSelfReflection(reactIns.config.GetEnableSelfReflection()),
		)
		assert.NoError(t, err)
		assert.NotNil(t, loop)

		// 验证 loop 中的反思配置
		assert.True(t, loop.GetEnableSelfReflection())
	})

	// 测试禁用自我反思
	t.Run("DisableSelfReflection", func(t *testing.T) {
		reactIns, err := NewReAct(
			WithContext(context.Background()),
			WithEnableSelfReflection(false),
		)
		assert.NoError(t, err)
		assert.False(t, reactIns.config.GetEnableSelfReflection())

		// 创建 loop 验证配置传递
		loop, err := reactloops.CreateLoopByName(
			schema.AI_REACT_LOOP_NAME_DEFAULT,
			reactIns,
			reactloops.WithEnableSelfReflection(reactIns.config.GetEnableSelfReflection()),
		)
		assert.NoError(t, err)
		assert.NotNil(t, loop)

		// 验证 loop 中的反思配置
		assert.False(t, loop.GetEnableSelfReflection())
	})

	// 测试默认配置（应该是禁用的）
	t.Run("DefaultSelfReflection", func(t *testing.T) {
		reactIns, err := NewReAct(WithContext(context.Background()))
		assert.NoError(t, err)
		assert.False(t, reactIns.config.GetEnableSelfReflection()) // 默认应该是 false

		// 创建 loop 验证配置传递
		loop, err := reactloops.CreateLoopByName(
			schema.AI_REACT_LOOP_NAME_DEFAULT,
			reactIns,
			reactloops.WithEnableSelfReflection(reactIns.config.GetEnableSelfReflection()),
		)
		assert.NoError(t, err)
		assert.NotNil(t, loop)

		// 验证 loop 中的反思配置
		assert.False(t, loop.GetEnableSelfReflection())
	})
}
