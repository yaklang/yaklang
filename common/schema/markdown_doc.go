package schema

import "gorm.io/gorm"

type MarkdownDoc struct {
	gorm.Model

	YakScriptId   int64  `json:"yak_script_id" gorm:"index"`
	YakScriptName string `json:"yak_script_name" gorm:"index"`
	Markdown      string `json:"markdown"`
}
