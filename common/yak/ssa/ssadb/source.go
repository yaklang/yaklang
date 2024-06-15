package ssadb

import (
	"net/url"
	"path"
	"strconv"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

var irSourceCache = utils.NewTTLCache[*memedit.MemEditor]()
var migrateIrSource = new(sync.Once)

type IrSource struct {
	SourceCodeHash string `json:"source_code_hash" gorm:"unique_index"`
	QuotedCode     string `json:"quoted_code"`
	FileUrl        string `json:"file_url"`
	Filename       string `json:"filename"`
	Filepath       string `json:"filepath"`
}

func SaveIrSource(editor *memedit.MemEditor, hash string) error {
	db := GetDB()

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

func GetIrSourceFromHash(hash string) (*memedit.MemEditor, error) {
	db := GetDB()
	result, ok := irSourceCache.Get(hash)
	if ok {
		return result, nil
	}

	if hash == "" {
		return nil, utils.Error("source code hash is empty, contact developers to fix it")
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
