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

	// Language: yak, java, php, js, etc
	// if the program contains many language,
	// use comma to separate them.
	// e.g. "yak,java,php"
	Language string `json:"language" gorm:"index"`

	// application / library
	ProgramKind          string      `json:"program_kind" gorm:"index"`
	ChildApplicationName StringSlice `json:"child_application_name" gorm:"type:text"`
	// up-stream program is the program that this program depends on
	UpStream StringSlice `json:"up_stream_programs" gorm:"type:text"`
	// down-stream program is the program that depends on this program
	DownStream StringSlice `json:"down_stream_programs" gorm:"type:text"`

	// this  program  contain this file
	FileList StringMap `json:"file_list" gorm:"type:text"`

	// program extra file: *.properties, *.xml, *.json, etc
	ExtraFile StringMap `json:"extra_file" gorm:"type:text"`
}

func CreateProgram(name, kind, version string, childName []string) *IrProgram {
	db := GetDB().Model(&IrProgram{})
	out := &IrProgram{
		ProgramName:          name,
		Version:              version,
		ProgramKind:          kind,
		ChildApplicationName: childName,
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
func GetPrograms(name string) ([]*IrProgram, error) {
	var p []*IrProgram
	db := GetDB().Model(&IrProgram{})
	if name == "" {
		return nil, utils.Errorf("program name is empty")
	}
	if find := db.Where("program_name in (?)", name).Find(p); find.Error != nil {
		return nil, find.Error
	}
	return p, nil
}
func UpdateProgram(prog *IrProgram) {
	GetDB().Model(&IrProgram{}).
		Where("id = ?", prog.ID).
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
	res = append(res, this.ChildApplicationName...)
	return bizhelper.ExactOrQueryStringArrayOr(GetDB(), "program_name", res)
}

func DeleteProgram(db *gorm.DB, program string) {
	// reuse the program object, avoid multiple db operation in short time
	progs := make(map[string]*IrProgram)
	getProgram := func(name string) (*IrProgram, error) {
		if p, ok := progs[name]; ok {
			return p, nil
		}
		p, err := GetProgram(name, "")
		if err != nil {
			return nil, err
		}
		progs[name] = p
		return p, nil
	}
	updateProgram := func(prog *IrProgram) {
		progs[prog.ProgramName] = prog
	}
	defer func() {
		for _, p := range progs {
			UpdateProgram(p)
		}
	}()

	var handlerUpstream func(this *IrProgram)
	handlerUpstream = func(this *IrProgram) {
		// update the down-stream programs
		for _, upStream := range this.UpStream {
			up, err := getProgram(upStream)
			if err != nil {
				log.Infof("get program %s error : %v", upStream, err)
				continue
			}
			up.DownStream = utils.RemoveSliceItem(up.DownStream, this.ProgramName)
			// if the up-stream program is not used by other programs, delete it
			if len(up.DownStream) == 0 {
				handlerUpstream(up)
			} else {
				updateProgram(up)
			}
		}
		// handler down-stream programs
		for _, downStream := range this.DownStream {
			down, err := getProgram(downStream)
			if err != nil {
				log.Infof("get program %s error : %v", downStream, err)
				continue
			}
			down.UpStream = utils.RemoveSliceItem(down.UpStream, this.ProgramName)
			updateProgram(down)
		}
		deleteProgramDBOnly(db, this.ProgramName)
	}
	this, err := getProgram(program)
	if err != nil {
		log.Errorf("get program %s error : %v", program, err)
		return
	}
	handlerUpstream(this)
}

func AllPrograms(db *gorm.DB) []string {
	var programs []string
	db.Model(&IrProgram{}).Select("DISTINCT(program_name)").Pluck("program_name", &programs)
	return programs
}
