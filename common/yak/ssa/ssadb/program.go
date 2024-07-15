package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

type IrProgram struct {
	gorm.Model

	ProgramName string `json:"program_name" gorm:"index"`
	Version     string `json:"package_version" gorm:"index"`
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
		Where("program_name = ?", prog.ProgramName).
		Where("program_kind = ?", prog.ProgramKind).
		Update(prog)
}

func GetDBInProgram(program string) *gorm.DB {
	this, err := GetProgram(program, "")
	if err != nil {
		log.Errorf("get program %s error : %v", program, err)
		return GetDB().Where("program_name = ?", program)
	}
	res := append(this.UpStream, program)
	return bizhelper.ExactOrQueryStringArrayOr(GetDB(), "program_name", res)
}

func DeleteProgram(db *gorm.DB, program string) {
	this, err := GetProgram(program, "")
	if err != nil {
		log.Errorf("get program %s error : %v", program, err)
		return
	}
	// update the down-stream programs
	for _, upStream := range this.UpStream {
		up, err := GetProgram(upStream, "")
		if err != nil {
			log.Infof("get program %s error : %v", upStream, err)
			continue
		}
		up.DownStream = utils.RemoveSliceItem(up.DownStream, program)
		// if the up-stream program is not used by other programs, delete it
		if len(up.DownStream) == 0 {
			DeleteProgram(db, upStream)
		} else {
			UpdateProgram(up)
		}
	}
	// handler down-stream programs
	for _, downStream := range this.DownStream {
		down, err := GetProgram(downStream, "")
		if err != nil {
			log.Infof("get program %s error : %v", downStream, err)
			continue
		}
		down.UpStream = utils.RemoveSliceItem(down.UpStream, program)
		UpdateProgram(down)
	}
	// delete the program
	db.Model(&IrCode{}).Where("program_name = ?", program).Unscoped().Delete(&IrCode{})
	db.Model(&IrVariable{}).Where("program_name = ?", program).Unscoped().Delete(&IrVariable{})
	db.Model(&IrScopeNode{}).Where("program_name = ?", program).Unscoped().Delete(&IrScopeNode{})
	db.Model(&IrProgram{}).Where("program_name = ?", program).Unscoped().Delete(&IrProgram{})
}

func AllPrograms(db *gorm.DB) []string {
	var programs []string
	db.Model(&IrCode{}).Select("DISTINCT(program_name)").Pluck("program_name", &programs)
	return programs
}
