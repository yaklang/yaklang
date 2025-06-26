package rag

import (
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

func TestExportVectorData(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	err := ExportVectorData(db, "yaklang_plugins_default", "/tmp/plugins_rag1.zip")
	if err != nil {
		t.Fatal(err)
	}
}

func TestImportVectorData(t *testing.T) {
	// tmp database
	db, err := gorm.Open(consts.SQLite, "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	err = ImportVectorData(db, "/tmp/plugins_rag1.zip")
	if err != nil {
		t.Fatal(err)
	}
}
