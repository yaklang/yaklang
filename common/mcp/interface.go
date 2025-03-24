package mcp

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type IGetProfileDatabase interface {
	GetProfileDatabase() *gorm.DB
}

var NewLocalClient func(locals ...bool) (ypb.YakClient, error)

func SetNewLocalClient(f func(locals ...bool) (ypb.YakClient, error)) {
	NewLocalClient = f
}
