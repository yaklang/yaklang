package schema

import "github.com/jinzhu/gorm"

type AliveHost struct {
	gorm.Model

	Hash string `json:"hash"`

	IP        string `json:"ip"`
	IPInteger int64  `json:"ip_integer"`

	// 设置运行时 ID 为了关联具体漏洞
	RuntimeId string `json:"runtime_id"`
}
