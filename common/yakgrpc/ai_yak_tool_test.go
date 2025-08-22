package yakgrpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestSaveAITool(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	err = db.AutoMigrate(&schema.AIYakTool{}).Error
	assert.NoError(t, err)

	// 1. 创建一个 AIYakTool 记录
	originalTool := &schema.AIYakTool{
		Name:        "test_tool",
		Description: "这是一个测试工具",
		Keywords:    "test,tool,example",
		Content:     "print('hello world')",
		Params:      "--param1 value1",
		Path:        "/test/path",
		IsFavorite:  false,
	}

	// 保存到数据库，BeforeSave hook 会自动计算 Hash
	yakit.SaveAIYakTool(db, originalTool)
	assert.NoError(t, err)
	assert.NotZero(t, originalTool.ID)    // 确保ID已生成
	assert.NotEmpty(t, originalTool.Hash) // 确保Hash已计算

	// 验证数据已保存到数据库
	var savedTool schema.AIYakTool
	err = db.Where("name = ?", "test_tool").First(&savedTool).Error
	assert.NoError(t, err)
	assert.Equal(t, "test_tool", savedTool.Name)
	assert.Equal(t, "这是一个测试工具", savedTool.Description)
	assert.Equal(t, "test,tool,example", savedTool.Keywords)
	assert.Equal(t, "print('hello world')", savedTool.Content)
	assert.Equal(t, "--param1 value1", savedTool.Params)
	assert.Equal(t, "/test/path", savedTool.Path)
	assert.False(t, savedTool.IsFavorite)

	// 保存原始 Hash 值用于后续比较
	originalHash := savedTool.Hash

	// 2. 更新描述信息
	savedTool.Description = "这是一个更新后的测试工具描述"
	savedTool.Keywords = "test,tool,example,updated"
	savedTool.IsFavorite = true

	// 执行更新操作，BeforeSave hook 会重新计算 Hash
	affected, err := yakit.UpdateAIYakToolByID(db, &savedTool)
	_ = affected
	assert.NoError(t, err)

	// 3. 验证更新结果
	var updatedTool schema.AIYakTool
	err = db.Where("name = ?", "test_tool").First(&updatedTool).Error
	assert.NoError(t, err)

	// 验证基本字段没有变化
	assert.Equal(t, "test_tool", updatedTool.Name)
	assert.Equal(t, "print('hello world')", updatedTool.Content)
	assert.Equal(t, "--param1 value1", updatedTool.Params)
	assert.Equal(t, "/test/path", updatedTool.Path)

	// 验证更新的字段
	assert.Equal(t, "这是一个更新后的测试工具描述", updatedTool.Description)
	assert.Equal(t, "test,tool,example,updated", updatedTool.Keywords)
	assert.True(t, updatedTool.IsFavorite)

	// 验证 Hash 值已更新（因为 Description 和 Keywords 发生了变化）
	assert.NotEmpty(t, updatedTool.Hash)
	assert.NotEqual(t, originalHash, updatedTool.Hash, "Hash值应该在更新后发生变化")

	// 4. 验证 Hash 计算方法
	expectedHash := utils.CalcSha1(updatedTool.Name, updatedTool.Content, updatedTool.Params, updatedTool.Path, updatedTool.Description, updatedTool.Keywords)
	assert.Equal(t, expectedHash, updatedTool.Hash, "Hash值应该与计算的值一致")

	// 5. 测试更新不改变内容的情况（Hash不应该变化）
	currentHash := updatedTool.Hash
	updatedTool.UpdatedAt = updatedTool.UpdatedAt.Add(1) // 只改变 UpdatedAt，不影响 Hash 计算
	err = db.Save(&updatedTool).Error
	assert.NoError(t, err)

	var finalTool schema.AIYakTool
	err = db.Where("name = ?", "test_tool").First(&finalTool).Error
	assert.NoError(t, err)
	assert.Equal(t, currentHash, finalTool.Hash, "当内容未变化时，Hash值应该保持不变")
}
