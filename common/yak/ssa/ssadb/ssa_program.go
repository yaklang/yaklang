package ssadb

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

var Programs = make(map[string]*schema.SSAProgram)

func GetSSAProgram(name string) *schema.SSAProgram {
	if prog, ok := Programs[name]; ok {
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
	Programs[name] = programs[0]
	return programs[0]
}

func SaveSSAProgram(name, desc, language string) error {

	db := consts.GetGormProfileDatabase()

	prog := &schema.SSAProgram{
		Name:        name,
		Description: desc,
		DBPath:      consts.GetSSADataBasePath(),
		Language:    language,
	}

	Programs[name] = prog

	return db.Save(prog).Error
}

func AllSSAPrograms() []*schema.SSAProgram {
	if len(Programs) > 0 {
		return lo.Values(Programs)
	}

	db := consts.GetGormProfileDatabase()
	var programs []*schema.SSAProgram
	db.Model(&schema.SSAProgram{}).Find(&programs)
	for _, p := range programs {
		Programs[p.Name] = p
	}

	return programs
}
