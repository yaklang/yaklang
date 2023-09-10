package yakurl

import (
	"github.com/yaklang/yaklang/common/wsm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type Action interface {
	Get(*ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error)
	Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error)
	Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error)
	Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error)
	Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error)
	Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error)
}

type DefaultActionFactory struct{}

func (f DefaultActionFactory) CreateAction(schema string) Action {
	switch schema {
	case "file":
		return &fileSystemAction{}
	case "behinder":
		return &wsm.BehidnerFileSystemAction{}
	default:
		return nil
	}
}
