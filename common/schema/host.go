package schema

import "github.com/yaklang/gorm"

type Host struct {
	gorm.Model

	IP        string `json:"ip" gorm:"unique_index"`
	IPInteger int64  `json:"ip_integer"`

	IsInPublicNet bool

	// splite by comma
	Domains string
}
