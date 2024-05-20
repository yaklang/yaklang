package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"strconv"
	"time"
)

type GeneralStorage struct {
	gorm.Model

	Key string `json:"key" gorm:"unique_index"`

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

func (s *GeneralStorage) ToGRPCModel() *ypb.GeneralStorage {
	keyStr, _ := strconv.Unquote(s.Key)
	if keyStr == "" {
		keyStr = s.Key
	}
	valueStr, _ := strconv.Unquote(s.Value)
	if valueStr == "" {
		valueStr = s.Value
	}
	var expiredAt int64 = 0
	if !s.ExpiredAt.IsZero() {
		expiredAt = s.ExpiredAt.Unix()
	}

	if valueStr == `""` {
		valueStr = ""
	}
	return &ypb.GeneralStorage{
		Key:        utils.EscapeInvalidUTF8Byte([]byte(keyStr)),
		Value:      utils.EscapeInvalidUTF8Byte([]byte(valueStr)),
		ExpiredAt:  expiredAt,
		ProcessEnv: s.ProcessEnv,
		Verbose:    s.Verbose,
		Group:      "",
	}
}

func (s *GeneralStorage) EnableProcessEnv() {
	if s == nil {
		return
	}
	if !s.ProcessEnv {
		return
	}
	key := s
	keyStr, _ := strconv.Unquote(key.Key)
	if keyStr == "" {
		keyStr = key.Key
	}

	valueStr, _ := strconv.Unquote(key.Value)
	if valueStr == "" {
		valueStr = key.Value
	}
	err := os.Setenv(keyStr, valueStr)
	if err != nil {
		log.Errorf("set env[%s] failed: %s", keyStr, err)
	}
}
