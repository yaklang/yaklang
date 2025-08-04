package ssadb

import (
	"strconv"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/utils"
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
	return irSource2Editor(source), nil
}

func irSource2Editor(source *IrSource) *memedit.MemEditor {
	code := source.QuotedCode
	if s, err := strconv.Unquote(code); err == nil {
		code = s
	}
	_, folder := splitProjectPath(source.FolderPath)
	ret := memedit.NewMemEditor(code)
	ret.SetFolderPath(folder)
	ret.SetFileName(source.FileName)
	ret.SetProgramName(source.ProgramName)
	return ret
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

	return irSource2Editor(&source), nil
}

func MarshalFile(editor *memedit.MemEditor) *IrSource {
	folderPath := irSourceJoin(editor.GetProgramName(), editor.GetFolderPath())
	irSource := &IrSource{
		ProgramName:    editor.GetProgramName(),
		SourceCodeHash: editor.GetIrSourceHash(),
		QuotedCode:     strconv.Quote(editor.GetSourceCode()),
		FileName:       editor.GetFilename(),
		FolderPath:     folderPath,
		IsBigFile:      false,
	}
	return irSource
}

func MarshalFolder(folderPaths []string) *IrSource {
	if len(folderPaths) < 1 {
		return nil
	}
	folderName := folderPaths[len(folderPaths)-1]
	folderPath := irSourceJoin(folderPaths[:len(folderPaths)-1]...)
	programName := folderPaths[0]
	irSource := &IrSource{
		ProgramName:    programName,
		SourceCodeHash: codec.Md5(folderPaths),
		QuotedCode:     "",
		FileName:       folderName,
		FolderPath:     folderPath,
		IsBigFile:      false,
	}
	return irSource
}

func (irSource *IrSource) Save(db *gorm.DB) error {
	if len(irSource.FolderPath) > 0 && irSource.FolderPath[0] != '/' {
		irSource.FolderPath = "/" + irSource.FolderPath
	}

	// log.Infof("save source: %v", irSource.SourceCodeHash)
	// check existed
	if err := db.Save(irSource).Error; err != nil {
		log.Errorf("save ir source failed: %v", err)
		return utils.Wrapf(err, "save ir source failed")
	}
	return nil
}
