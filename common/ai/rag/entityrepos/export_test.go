package entityrepos

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestExportImportEntityRepository(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		t.Fatal("database is nil")
	}

	ctx := context.Background()

	// 创建测试实体仓库
	reposName := "test_export_import_" + utils.RandStringBytes(8)
	repos, err := GetOrCreateEntityRepository(db, reposName, "测试导出导入功能", WithDisableBulkProcess())
	if err != nil {
		t.Fatalf("create entity repository failed: %v", err)
	}
	defer DeleteEntityRepository(db, reposName)

	// 添加测试实体
	entity1 := &schema.ERModelEntity{
		EntityName:        "测试实体1",
		Description:       "这是第一个测试实体",
		EntityType:        "Person",
		EntityTypeVerbose: "人物",
		Attributes: map[string]any{
			"age":  30,
			"city": "北京",
		},
	}
	if err := repos.CreateEntity(entity1); err != nil {
		t.Fatalf("create entity1 failed: %v", err)
	}

	entity2 := &schema.ERModelEntity{
		EntityName:        "测试实体2",
		Description:       "这是第二个测试实体",
		EntityType:        "Company",
		EntityTypeVerbose: "公司",
		Attributes: map[string]any{
			"industry": "科技",
			"founded":  2020,
		},
	}
	if err := repos.CreateEntity(entity2); err != nil {
		t.Fatalf("create entity2 failed: %v", err)
	}

	// 添加测试关系
	if err := repos.AddRelationship(entity1.Uuid, entity2.Uuid, "WORKS_AT", "在...工作", map[string]any{
		"since": "2021",
	}); err != nil {
		t.Fatalf("add relationship failed: %v", err)
	}

	// 导出实体仓库
	t.Log("开始导出实体仓库...")
	exportReader, err := repos.Export(ctx, &ExportEntityRepositoryOptions{
		OnProgressHandler: func(percent float64, message string, messageType string) {
			t.Logf("[%.0f%%] %s", percent, message)
		},
	})
	if err != nil {
		t.Fatalf("export entity repository failed: %v", err)
	}

	// 读取导出数据
	var exportBuf bytes.Buffer
	if _, err := exportBuf.ReadFrom(exportReader); err != nil {
		t.Fatalf("read export data failed: %v", err)
	}
	exportData := exportBuf.Bytes()
	t.Logf("导出数据大小: %d bytes", len(exportData))

	// 删除原实体仓库
	if err := DeleteEntityRepository(db, reposName); err != nil {
		t.Fatalf("delete entity repository failed: %v", err)
	}

	// 导入实体仓库
	t.Log("开始导入实体仓库...")
	importReader := bytes.NewReader(exportData)
	if err := ImportEntityRepository(ctx, db, importReader, &ImportEntityRepositoryOptions{
		NewRepositoryName: reposName,
		OverwriteExisting: true,
		OnProgressHandler: func(percent float64, message string, messageType string) {
			t.Logf("[%.0f%%] %s", percent, message)
		},
	}); err != nil {
		t.Fatalf("import entity repository failed: %v", err)
	}

	// 验证导入结果
	t.Log("验证导入结果...")
	importedRepos, err := GetEntityRepositoryByName(db, reposName, WithDisableBulkProcess())
	if err != nil {
		t.Fatalf("get imported entity repository failed: %v", err)
	}

	// 验证实体数量
	var entityCount int64
	if err := db.Model(&schema.ERModelEntity{}).Where("repository_uuid = ?", importedRepos.info.Uuid).Count(&entityCount).Error; err != nil {
		t.Fatalf("count entities failed: %v", err)
	}
	if entityCount != 2 {
		t.Errorf("expected 2 entities, got %d", entityCount)
	}

	// 验证关系数量
	var relationshipCount int64
	if err := db.Model(&schema.ERModelRelationship{}).Where("repository_uuid = ?", importedRepos.info.Uuid).Count(&relationshipCount).Error; err != nil {
		t.Fatalf("count relationships failed: %v", err)
	}
	if relationshipCount != 1 {
		t.Errorf("expected 1 relationship, got %d", relationshipCount)
	}

	// 验证实体内容
	entities, err := yakit.QueryEntities(db, schema.SimpleBuildEntityFilter(reposName, nil, nil, nil, nil))
	if err != nil {
		t.Fatalf("query entities failed: %v", err)
	}
	if len(entities) != 2 {
		t.Errorf("expected 2 entities, got %d", len(entities))
	}

	// 验证实体属性
	for _, entity := range entities {
		t.Logf("实体: %s (%s) - %v", entity.EntityName, entity.EntityType, entity.Attributes)
		if entity.EntityName == "测试实体1" {
			if entity.Attributes["age"] != float64(30) { // JSON unmarshal 会将数字转为 float64
				t.Errorf("entity1 age mismatch, got %v", entity.Attributes["age"])
			}
		}
	}

	t.Log("导出导入测试完成!")
}
