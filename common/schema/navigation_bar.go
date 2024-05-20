package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type NavigationBar struct {
	gorm.Model
	Group         string `json:"group" `
	YakScriptName string `json:"yak_script_name"`
	Hash          string `json:"-" gorm:"unique_index"`
	Mode          string `json:"mode"`
	VerboseSort   int64  `json:"verbose_sort"`
	GroupSort     int64  `json:"group_sort"`
	Route         string `json:"route"`
	Verbose       string `json:"verbose"`
	GroupLabel    string `json:"group_label"`
	VerboseLabel  string `json:"verbose_label"`
}

func (m *NavigationBar) CalcHash() string {
	key := m.VerboseLabel
	if key == "" {
		key = m.YakScriptName
	}

	return utils.CalcSha1(m.Group, m.Mode, key)
}
