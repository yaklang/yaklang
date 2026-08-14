package ssadb

import (
	"fmt"
	"testing"

	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
)

func setupNativeBatchBenchDB(b *testing.B) *gorm.DB {
	db, err := gorm.Open("sqlite3", ":memory:")
	if err != nil {
		b.Fatal(err)
	}
	if err := db.AutoMigrate(&IrCode{}).Error; err != nil {
		b.Fatal(err)
	}
	for i := int64(1); i <= 500; i++ {
		code := &IrCode{CodeID: i, ProgramName: "bench", OpcodeName: "Call", String: fmt.Sprintf("f%d()", i), Users: Int64Slice{i, i + 1}}
		if err := db.Create(code).Error; err != nil {
			b.Fatal(err)
		}
	}
	return db
}

// BenchmarkNativeGetIrCodesByIds measures the O2 native batch read path.
func BenchmarkNativeGetIrCodesByIds(b *testing.B) {
	db := setupNativeBatchBenchDB(b)
	ids := make([]int64, 0, 200)
	for i := int64(1); i <= 200; i++ {
		ids = append(ids, i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	var sink []*IrCode
	for i := 0; i < b.N; i++ {
		r, err := nativeGetIrCodesByIds(db, "bench", ids)
		if err != nil {
			b.Fatal(err)
		}
		sink = r
	}
	_ = sink
}

// BenchmarkGormGetIrCodesByIds measures the GORM Find(&irs) batch path that
// PreloadIrCodesByIdsFast currently uses (the O2 baseline).
func BenchmarkGormGetIrCodesByIds(b *testing.B) {
	db := setupNativeBatchBenchDB(b)
	ids := make([]int64, 0, 200)
	for i := int64(1); i <= 200; i++ {
		ids = append(ids, i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	var sink []*IrCode
	for i := 0; i < b.N; i++ {
		var irs []*IrCode
		if err := db.Model(&IrCode{}).Where("program_name = ?", "bench").Where("code_id in (?)", ids).Find(&irs).Error; err != nil {
			b.Fatal(err)
		}
		sink = irs
	}
	_ = sink
}
