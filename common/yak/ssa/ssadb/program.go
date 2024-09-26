package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type IrProgram struct {
	gorm.Model

	ProgramName string `json:"program_name" gorm:"index"`
	Version     string `json:"package_version" gorm:"index"`

	// Language: yak, java, php, js, etc
	// if the program contains many language,
	// use comma to separate them.
	// e.g. "yak,java,php"
	Language string `json:"language" gorm:"index"`

	// application / library
	ProgramKind string `json:"program_kind" gorm:"index"`

	// up-stream program is the program that this program depends on
	UpStream StringSlice `json:"up_stream_programs" gorm:"type:text"`
	// down-stream program is the program that depends on this program
	DownStream StringSlice `json:"down_stream_programs" gorm:"type:text"`

	// this  program  contain this file
	FileList StringMap `json:"file_list" gorm:"type:text"`

	// program extra file: *.properties, *.xml, *.json, etc
	ExtraFile StringMap `json:"extra_file" gorm:"type:text"`
}

func CreateProgram(name, kind, version string) *IrProgram {
	db := GetDB().Model(&IrProgram{})
	out := &IrProgram{
		ProgramName: name,
		Version:     version,
		ProgramKind: kind,
	}
	db.Save(out)
	return out
}

func GetLibrary(name, version string) (*IrProgram, error) {
	var p IrProgram
	db := GetDB().Model(&IrProgram{})
	if name == "" {
		return nil, utils.Errorf("program name is empty")
	}
	db = db.Where("program_name = ?", name)
	db = db.Where("program_kind = ?", "library")
	db = db.Where("version = ?", version)
	if err := db.First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func GetProgram(name, kind string) (*IrProgram, error) {

	var p IrProgram
	db := GetDB().Model(&IrProgram{})
	if name == "" {
		return nil, utils.Errorf("program name is empty")
	}
	db = db.Where("program_name = ?", name)
	if kind != "" {
		db = db.Where("program_kind = ?", kind)
	}
	if err := db.First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func UpdateProgram(prog *IrProgram) {
	GetDB().Model(&IrProgram{}).
		Where("id = ?", prog.ID).
		Where("program_name = ?", prog.ProgramName).
		Where("program_kind = ?", prog.ProgramKind).
		Update(prog)
}

func GetDBInProgram(program string) *gorm.DB {
	return GetDB().Where("program_name = ?", program)
}

func DeleteProgram(db *gorm.DB, program string) {
	deleteProgramDBOnly(db, program)
}

func AllPrograms(db *gorm.DB) []string {
	var programs []string
	db.Model(&IrProgram{}).Select("DISTINCT(program_name)").Pluck("program_name", &programs)
	return programs
}
