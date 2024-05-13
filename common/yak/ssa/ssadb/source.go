package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"net/url"
	"path"
	"strconv"
	"sync"
)

var irSourceCache = utils.NewTTLCache[*memedit.MemEditor]()
var migrateIrSource = new(sync.Once)

type IrSource struct {
	gorm.Model

	SourceCodeHash string `json:"source_code_hash" gorm:"unique_index"`
	QuotedCode     string `json:"quoted_code"`
	FileUrl        string `json:"file_url"`
	Filename       string `json:"filename"`
	Filepath       string `json:"filepath"`
}

func SaveIrSource(db *gorm.DB, editor *memedit.MemEditor, hash string) error {
	migrateIrSource.Do(func() {
		db.AutoMigrate(&IrSource{})
	})

	if editor.GetSourceCode() == "" {
		return utils.Errorf("source code is empty")
	}

	_, ok := irSourceCache.Get(hash)
	if ok {
		return nil
	}

	var fileUrl string
	var filename, filepath string
	if editor.GetUrl() != "" {
		fileUrl = editor.GetUrl()
		urlIns, err := url.Parse(fileUrl)
		if err != nil {
			log.Warnf("parse url %s failed: %v", fileUrl, err)
		}
		if urlIns != nil {
			filename, filepath = path.Split(urlIns.Path)
		}
	}

	irSource := &IrSource{
		SourceCodeHash: hash,
		QuotedCode:     strconv.Quote(editor.GetSourceCode()),
		FileUrl:        fileUrl,
		Filename:       filename,
		Filepath:       filepath,
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
	code, _ := strconv.Unquote(source.QuotedCode)
	if code == "" {
		code = source.QuotedCode
	}
	editor := memedit.NewMemEditor(code)
	if source.FileUrl != "" {
		editor.SetUrl(source.FileUrl)
	}
	return editor, nil
}
