package ssadb

import (
	"fmt"
	"testing"

	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

func setupBenchDB(b *testing.B) *gorm.DB {
	db, err := gorm.Open("sqlite3", ":memory:")
	if err != nil {
		b.Fatal(err)
	}
	if err := db.AutoMigrate(&IrCode{}, &IrType{}).Error; err != nil {
		b.Fatal(err)
	}
	for i := int64(1); i <= 500; i++ {
		code := &IrCode{CodeID: i, ProgramName: "bench", OpcodeName: "Call", String: fmt.Sprintf("f%d()", i), Users: Int64Slice{i, i + 1}}
		if err := db.Create(code).Error; err != nil {
			b.Fatal(err)
		}
		typ := &IrType{TypeId: uint64(i), ProgramName: "bench", Kind: 1, String: "T"}
		if err := db.Create(typ).Error; err != nil {
			b.Fatal(err)
		}
	}
	return db
}

func BenchmarkGetIrCodeItemById_Native(b *testing.B) {
	db := setupBenchDB(b)
	b.ResetTimer()
	var sink *IrCode
	for i := 0; i < b.N; i++ {
		sink = GetIrCodeItemById(db, "bench", int64(i%500)+1)
	}
	_ = sink
}

func BenchmarkGetIrCodeItemById_GORM(b *testing.B) {
	db := setupBenchDB(b)
	b.ResetTimer()
	var sink *IrCode
	for i := 0; i < b.N; i++ {
		id := int64(i%500) + 1
		ir := &IrCode{}
		db.Model(&IrCode{}).Where("code_id = ?", id).Where("program_name = ?", "bench").First(ir)
		sink = ir
	}
	_ = sink
}
