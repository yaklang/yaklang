package cvequeryops

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cveresources"
)

// 回归测试：batchMergeCVEV5Fields 的 SQLite IN (?) 语法修复与批量容错

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(consts.SQLite, ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&cveresources.CVE{}).Error)
	return db
}

func TestBatchMergeCVEV5Fields_QuerySucceeds(t *testing.T) {
	// 验证 IN (?) 语法在 GORM+SQLite 下不再报 near "?": syntax error
	db := setupTestDB(t)

	// 预置两条已有记录（模拟 NVD 2.0 已入库）
	db.Create(&cveresources.CVE{CVE: "CVE-2024-0001", DescriptionMain: "existing desc"})
	db.Create(&cveresources.CVE{CVE: "CVE-2024-0002", DescriptionMain: "existing desc"})

	// V5 补充字段
	v5s := []*cveresources.CVE{
		{CVE: "CVE-2024-0001", Title: "V5 Title for 0001", Vendor: "vendor-a"},
		{CVE: "CVE-2024-0002", Title: "V5 Title for 0002", Vendor: "vendor-b"},
		{CVE: "CVE-2024-0003", Title: "New record from V5"},  // 不存在 → 创建
	}

	err := batchMergeCVEV5Fields(db, v5s)
	require.NoError(t, err, "IN (?) 语法修复后批量查询和合并不应报错")

	// 验证 V5 字段确实落库（不只是总数）
	var rec1, rec2, rec3 cveresources.CVE
	require.NoError(t, db.Where("cve = ?", "CVE-2024-0001").First(&rec1).Error)
	assert.Equal(t, "V5 Title for 0001", rec1.Title, "V5 title 应合并到已有记录")
	assert.Equal(t, "vendor-a", rec1.Vendor)

	require.NoError(t, db.Where("cve = ?", "CVE-2024-0002").First(&rec2).Error)
	assert.Equal(t, "V5 Title for 0002", rec2.Title)

	require.NoError(t, db.Where("cve = ?", "CVE-2024-0003").First(&rec3).Error)
	assert.Equal(t, "New record from V5", rec3.Title, "不存在的记录应被创建")
}

func TestBatchMergeCVEV5Fields_SingleFailureSkipped(t *testing.T) {
	// 验证单条失败不回滚整个事务（其他记录正常处理）
	db := setupTestDB(t)

	// 预置记录
	db.Create(&cveresources.CVE{CVE: "CVE-2024-0100", DescriptionMain: "desc"})
	db.Create(&cveresources.CVE{CVE: "CVE-2024-0101", DescriptionMain: "desc"})

	// 构造一条会导致失败的记录（CVE 字段超长触发 SQLite 约束）
	longCVE := "CVE-" + string(make([]byte, 5000))
	v5s := []*cveresources.CVE{
		{CVE: "CVE-2024-0100", Title: "good title"},   // 应成功
		{CVE: longCVE, Title: "will fail"},             // 应失败但被跳过
		{CVE: "CVE-2024-0101", Title: "also good"},     // 应成功
	}

	err := batchMergeCVEV5Fields(db, v5s)
	require.NoError(t, err, "单条失败不应导致整体报错")

	// 验证好的记录确实被处理了
	var rec1, rec2 cveresources.CVE
	require.NoError(t, db.Where("cve = ?", "CVE-2024-0100").First(&rec1).Error)
	assert.Equal(t, "good title", rec1.Title, "失败前的好记录应已合并")
	require.NoError(t, db.Where("cve = ?", "CVE-2024-0101").First(&rec2).Error)
	assert.Equal(t, "also good", rec2.Title, "失败后的好记录也应被处理")
}

func TestBatchMergeCVEV5Fields_EmptyBatch(t *testing.T) {
	db := setupTestDB(t)
	err := batchMergeCVEV5Fields(db, nil)
	assert.NoError(t, err)
	err = batchMergeCVEV5Fields(db, []*cveresources.CVE{})
	assert.NoError(t, err)
}

func TestBatchMergeCVEV5Fields_LargeBatch(t *testing.T) {
	// 验证大批量下 IN (?) 语法正确展开
	db := setupTestDB(t)
	var v5s []*cveresources.CVE
	for i := 0; i < 100; i++ {
		cveID := fmt.Sprintf("CVE-2024-%04d", i)
		db.Create(&cveresources.CVE{CVE: cveID, DescriptionMain: "seed"})
		v5s = append(v5s, &cveresources.CVE{CVE: cveID, Title: "batch title"})
	}
	err := batchMergeCVEV5Fields(db, v5s)
	require.NoError(t, err)

	var count int64
	db.Model(&cveresources.CVE{}).Where("title = ?", "batch title").Count(&count)
	assert.Equal(t, int64(100), count, "全部 100 条的 title 都应被合并")
}
