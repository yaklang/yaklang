package yakit

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils"
)

func TestQueryRAGCollectionByName(t *testing.T) {
	db := utils.CreateTempTestDatabaseInMemory()
	collection, err := QueryRAGCollectionByName(db, "test")
	if err != nil {
		t.Fatalf("查询集合失败: %v", err)
	}
	t.Logf("集合: %v", collection)
}
