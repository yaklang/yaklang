package schema

import (
	"time"

	"gorm.io/gorm"
)

type ProjectGeneralStorage struct {
	gorm.Model

	Key string `json:"key" gorm:"uniqueIndex"`

	// 经过 JSON + Strconv
	Value string `json:"value"`

	// 过期时间
	ExpiredAt time.Time

	// YAKIT SUBPROC_ENV
	ProcessEnv bool

	// 帮助信息，描述这个变量是干嘛的
	Verbose string

	// 描述变量所在的组是啥
	Group string
}
