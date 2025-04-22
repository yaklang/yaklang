package yakurl

import (
	"time"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type extra struct {
	name  string
	value any
}

func createNewRes(originParam *ypb.YakURL, size int, extra []extra) *ypb.YakURLResource {
	yakURL := &ypb.YakURL{
		Schema:   originParam.Schema,
		User:     originParam.GetUser(),
		Pass:     originParam.GetPass(),
		Location: originParam.GetLocation(),
		Path:     originParam.GetPath(),
		Query:    originParam.GetQuery(),
	}

	res := &ypb.YakURLResource{
		Size:              int64(size),
		ModifiedTimestamp: time.Now().Unix(),
		Path:              originParam.Path,
		YakURLVerbose:     "",
		Url:               yakURL,
	}
	if len(extra) > 0 {
		for _, v := range extra {
			res.Extra = append(res.Extra, &ypb.KVPair{
				Key:   v.name,
				Value: codec.AnyToString(v.value),
			})
		}
	}
	return res
}
