package ssadb

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/omap"
)

var Programs = omap.NewEmptyOrderedMap[string, *schema.SSAProgram]()

func CheckAndSwitchDB(name string) *schema.SSAProgram {
	// switch to database
	prog := GetSSAProgram(name)
	if prog == nil {
		return nil
	}
	if prog.DBPath != consts.GetSSADataBasePath() {
		consts.SetSSADataBasePath(prog.DBPath)
	}
	return prog
}

func GetSSAProgram(name string) *schema.SSAProgram {
	if prog, ok := Programs.Get(name); ok {
		return prog
	}

	db := consts.GetGormProfileDatabase()
	var programs []*schema.SSAProgram
	if err := db.Model(&schema.SSAProgram{}).Where("name = ?", name).First(&programs).Error; err != nil {
		log.Errorf("get ssa program [%v] error: %s", name, err)
		return nil
	}
	if len(programs) == 0 {
		return nil
	}
	Programs.Set(name, programs[0])
	return programs[0]
}

func SaveSSAProgram(name, desc, language, info string) error {
	db := consts.GetGormProfileDatabase()

	prog := &schema.SSAProgram{
		Name:          name,
		Description:   desc,
		DBPath:        consts.GetSSADataBasePath(),
		Language:      language,
		EngineVersion: consts.GetYakVersion(),
		ConfigInput:   info,
	}

	Programs.Set(name, prog)

	return db.Model(&schema.SSAProgram{}).Save(prog).Error
}

func DeleteSSAProgram(name string) error {
	db := consts.GetGormProfileDatabase()
	if err := db.Model(&schema.SSAProgram{}).Where("name = ?", name).Unscoped().Delete(&schema.SSAProgram{}).Error; err != nil {
		log.Errorf("delete ssa program [%v] error: %s", name, err)
		return err
	}
	Programs.Delete(name)
	return nil
}

func AllSSAPrograms() []*schema.SSAProgram {
	db := consts.GetGormProfileDatabase()
	var programs []*schema.SSAProgram
	db = db.Model(&schema.SSAProgram{}).Order("created_at DESC").Find(&programs)
	if err := db.Error; err != nil {
		log.Errorf("get all ssa programs error: %s", err)
	}
	for _, p := range programs {
		if p == nil {
			continue
		}
		Programs.Set(p.Name, p)
	}

	return programs
}

func GetProfileSSAProgram() []string {
	db := consts.GetGormProfileDatabase()
	var programs []string
	db.Model(&schema.SSAProgram{}).Select("DISTINCT(name)").Pluck("name", &programs)
	return programs
}
