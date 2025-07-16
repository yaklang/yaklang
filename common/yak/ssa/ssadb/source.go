package ssadb

import (
	"strconv"
	"sync/atomic"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/databasex"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

var irSourceCache = utils.NewTTLCache[*memedit.MemEditor]()

type IrSource struct {
	ProgramName    string `json:"program_name" gorm:"index"`
	SourceCodeHash string `json:"source_code_hash" gorm:"index"` // default md5

	// file path
	FolderPath string `json:"folder_path"`
	FileName   string `json:"file_name"`

	// file content
	QuotedCode string `json:"quoted_code" gorm:"type:longtext"`
	IsBigFile  bool   `json:"is_big_file"` // if set this flag, the source code is too big, QuotedCode contain this file path

}

func GetIrSourceByPath(path string) ([]*IrSource, error) {
	db := GetDB()
	var sources []*IrSource
	if err := db.Where("folder_path = ?", path).Find(&sources).Error; err != nil {
		return nil, utils.Wrapf(err, "query source via path: %v failed", path)
	}
	return sources, nil
}

func GetIrSourceByPathAndName(path, name string) (*IrSource, error) {
	db := GetDB()
	var source IrSource
	if err := db.Where("folder_path = ? and file_name = ?", path, name).First(&source).Error; err != nil {
		return nil, utils.Wrapf(err, "query source via path: %v failed", path)
	}
	return &source, nil
}

func GetEditorByFileName(fileName string) (*memedit.MemEditor, error) {
	dir, name := pathSplit(fileName)
	source, err := GetIrSourceByPathAndName(dir, name)
	if err != nil {
		return nil, err
	}
	code := source.QuotedCode
	if s, err := strconv.Unquote(code); err == nil {
		code = s
	}
	ret := memedit.NewMemEditor(code)
	_, filePath := splitProjectPath(fileName)
	ret.SetUrl(filePath)
	return ret, nil
}

func SaveSource() {
	sourceSave.Close()
}

var sourceSave = databasex.NewSave[*IrSource](func(is []*IrSource) {
	db := GetDB()
	utils.GormTransaction(db, func(tx *gorm.DB) error {
		for _, irSource := range is {
			if len(irSource.FolderPath) > 0 && irSource.FolderPath[0] != '/' {
				irSource.FolderPath = "/" + irSource.FolderPath
			}
			if err := irSource.save(tx); err != nil {
				log.Errorf("save source %v failed: %v", irSource, err)
			}
		}
		return nil
	})
})

func SaveFile(filename, content string, programName string, folderPaths []string) string {
	start := time.Now()
	defer func() {
		atomic.AddUint64(&_SSASourceCodeCost, uint64(time.Now().Sub(start).Nanoseconds()))
	}()
	if programName == "" {
		// only use memory
		return ""
	}
	// append program name with folder path as full path
	fullPathParts := []string{programName}
	fullPathParts = append(fullPathParts, folderPaths...)
	fullPath := irSourceJoin(fullPathParts...)
	// calc file hash
	folderPath := irSourceJoin(folderPaths...)
	fileUrl := irSourceJoin(folderPath, filename)
	editor := memedit.NewMemEditorWithFileUrl(content, fileUrl)
	hash := editor.GetIrSourceHash(programName)

	irSource := &IrSource{
		ProgramName:    programName,
		SourceCodeHash: hash,
		QuotedCode:     strconv.Quote(content),
		FileName:       filename,
		FolderPath:     fullPath,
		IsBigFile:      false,
	}
	// go irSource.save(db)
	sourceSave.Save(irSource)
	return irSource.SourceCodeHash
}

func SaveFolder(folderName string, folderPaths []string) error {
	start := time.Now()
	defer func() {
		atomic.AddUint64(&_SSASourceCodeCost, uint64(time.Now().Sub(start).Nanoseconds()))
	}()

	if len(folderPaths) == 0 || folderPaths[0] == "" {
		return utils.Errorf("folder path is empty")
	}
	programName := folderPaths[0]
	folderPath := irSourceJoin(folderPaths...)

	irSource := &IrSource{
		ProgramName:    programName,
		SourceCodeHash: codec.Md5(programName + folderPath + folderName),
		QuotedCode:     "",
		FileName:       folderName,
		FolderPath:     folderPath,
		IsBigFile:      false,
	}
	// irSource.save(db)
	sourceSave.Save(irSource)
	return nil
}

func (irSource *IrSource) save(db *gorm.DB) error {
	if len(irSource.FolderPath) > 0 && irSource.FolderPath[0] != '/' {
		irSource.FolderPath = "/" + irSource.FolderPath
	}
	// log.Infof("save source: %v", irSource)
	// check existed
	if err := db.Save(irSource).Error; err != nil {
		return utils.Wrapf(err, "save ir source failed")
	}
	// var existed IrSource
	// if db.Where("source_code_hash = ?", irSource.SourceCodeHash).First(&existed).RecordNotFound() {
	// 	if err := db.Create(irSource).Error; err != nil {
	// 		return utils.Wrapf(err, "save ir source failed")
	// 	}
	// }
	return nil
}

// GetIrSourceFromHash fetch editor from cache by hash(md5)
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
	//_, folder, _ := strings.Cut(source.FolderPath, source.ProgramName)
	_, fileUrl := splitProjectPath(irSourceJoin(source.FolderPath, source.FileName))
	editor := memedit.NewMemEditorWithFileUrl(code, fileUrl)
	return editor, nil
}
