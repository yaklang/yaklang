package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type PluginGroup struct {
	gorm.Model

	YakScriptName string `json:"yak_script_name" gorm:"index"`
	Group         string `json:"group"`
	Hash          string `json:"hash" gorm:"unique_index"`
	TemporaryId   string `json:"temporary_id"`
	IsPocBuiltIn  bool   `json:"is_poc_built_in"`
}

func (p *PluginGroup) CalcHash() string {
	return utils.CalcSha1(p.YakScriptName, p.Group, p.TemporaryId)
}
