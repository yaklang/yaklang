package ssadb

import (
	"strconv"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
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
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}
	err := db.Where("folder_path = ?", path).Find(&sources).Error
	if err == nil {
		return sources, nil
	}
	return nil, utils.Wrapf(err, "query source via path: %v failed", path)
}

func GetIrSourceByPathAndName(path, name string) (*IrSource, error) {
	db := GetDB()
	var source IrSource
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}
	err := db.Where("folder_path = ? and file_name = ?", path, name).First(&source).Error
	if err == nil {
		return &source, nil
	}
	return nil, utils.Wrapf(err, "query source via path: %v failed", path)

}

func GetEditorByFileName(fileName string) (*memedit.MemEditor, error) {
	dir, name := pathSplit(fileName)
	if !strings.HasSuffix(dir, "/") {
		dir = dir + "/"
	}
	source, err := GetIrSourceByPathAndName(dir, name)
	if err == nil {
		return irSource2Editor(source), nil
	}
	return nil, utils.Wrapf(err, "get editor by file name: %s failed", fileName)
}

func irSource2Editor(source *IrSource) *memedit.MemEditor {
	code := source.QuotedCode
	if s, err := strconv.Unquote(code); err == nil {
		code = s
	}
	folderPath := source.FolderPath
	prefix := "/" + source.ProgramName + "/"
	if strings.HasPrefix(folderPath, prefix) {
		folderPath = folderPath[len(prefix):]
	}
	if !strings.HasSuffix(folderPath, "/") {
		folderPath = folderPath + "/"
	}
	// _, folder := splitProjectPath(source.FolderPath)
	ret := memedit.NewMemEditor(code)
	ret.SetFolderPath(folderPath)
	ret.SetFileName(source.FileName)
	ret.SetProgramName(source.ProgramName)
	if ret.GetIrSourceHash() != source.SourceCodeHash {
		log.Errorf(
			`ir source hash not match: [%s] != [%s]`,
			ret.GetIrSourceHash(),
			source.SourceCodeHash,
		)
	}
	return ret
}

func GetEditorByHash(hash string) (*memedit.MemEditor, error) {
	db := GetDB()
	result, ok := irSourceCache.Get(hash)
	if ok {
		log.Debugf("GetEditorByHash: found in cache, hash=%s", hash)
		return result, nil
	}

	if hash == "" {
		return nil, utils.Error("source code hash is empty, contact developers to fix it")
	}

	var source IrSource
	if err := db.Where("source_code_hash = ?", hash).First(&source).Error; err != nil {
		var count int64
		db.Model(&IrSource{}).Count(&count)
		log.Debugf("GetEditorByHash: source not found for hash=%s, total_ir_source_count=%d, error=%v (this is expected for external/undefined values)", hash, count, err)
		return nil, utils.Wrapf(err, "query source via hash: %v failed", hash)
	}

	log.Debugf("GetEditorByHash: loaded from database, hash=%s, program=%s, path=%s", hash, source.ProgramName, source.FolderPath+source.FileName)
	return irSource2Editor(&source), nil
}

func GetEditorCountByProgramName(programName string) (int, error) {
	db := GetDB()
	var count int
	if err := db.Model(&IrSource{}).Where("program_name = ?", programName).Count(&count).Error; err != nil {
		return 0, utils.Wrapf(err, "query source via program name: %v failed", programName)
	}
	return count, nil
}

func GetEditorByProgramName(programName string) ([]*memedit.MemEditor, error) {
	db := GetDB()
	var sources []*IrSource
	if err := db.Where("program_name = ?", programName).Find(&sources).Error; err != nil {
		return nil, utils.Wrapf(err, "query source via program name: %v failed", programName)
	}
	editors := make([]*memedit.MemEditor, 0, len(sources))
	for _, source := range sources {
		editors = append(editors, irSource2Editor(source))
	}
	return editors, nil
}

func MarshalFile(editor *memedit.MemEditor) *IrSource {
	prefix := "/" + editor.GetProgramName() + "/"
	folderPath := editor.GetFolderPath()
	if !strings.HasPrefix(folderPath, prefix) {
		folderPath = prefix + folderPath
	}
	path := editor.GetFolderPath()
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}
	editor.SetFolderPath(path)

	if !strings.HasSuffix(folderPath, "/") {
		folderPath = folderPath + "/"
	}
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
	if !strings.HasSuffix(folderPath, "/") {
		folderPath = folderPath + "/"
	}
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
	var err error
	ssaprofile.ProfileAdd(true, "Database.Source", func() {
		err = db.Save(irSource).Error
	})
	if err != nil {
		log.Errorf("save ir source failed: %v", err)
		return utils.Wrapf(err, "save ir source failed")
	}
	return nil
}
