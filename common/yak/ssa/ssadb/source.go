package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"sync"
)

var irSourceCache = utils.NewTTLCache[*memedit.MemEditor]()
var migrateIrSource = new(sync.Once)

type IrSource struct {
	gorm.Model

	SourceCodeHash string `json:"source_code_hash" gorm:"unique_index"`
	Code           string `json:"code"`
}

func SaveIrSource(db *gorm.DB, editor *memedit.MemEditor, hash string) error {
	migrateIrSource.Do(func() {
		db.AutoMigrate(&IrSource{})
	})

	_, ok := irSourceCache.Get(hash)
	if ok {
		return nil
	}

	irSource := &IrSource{
		SourceCodeHash: hash,
		Code:           editor.GetSourceCode(),
	}
	// check existed
	var existed IrSource
	if db.Where("source_code_hash = ?", hash).First(&existed).RecordNotFound() {
		if err := db.Create(irSource).Error; err != nil {
			return utils.Wrapf(err, "save ir source failed")
		}
		irSourceCache.Set(hash, editor)
		return nil
	}
	return nil
}

func GetIrSourceFromHash(db *gorm.DB, hash string) (*memedit.MemEditor, error) {
	result, ok := irSourceCache.Get(hash)
	if ok {
		return result, nil
	}

	var source IrSource
	if err := db.Where("source_code_hash = ?", hash).First(&source).Error; err != nil {
		return nil, utils.Wrapf(err, "query source via hash: %v failed", hash)
	}
	return memedit.NewMemEditor(source.Code), nil
}
