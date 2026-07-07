package schema

import "gorm.io/gorm"

type Host struct {
	gorm.Model

	IP        string `json:"ip" gorm:"uniqueIndex"`
	IPInteger int64  `json:"ip_integer"`

	IsInPublicNet bool

	// splite by comma
	Domains string
}
