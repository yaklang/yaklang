package schema

import (
	"encoding/json"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type CodecFlow struct {
	gorm.Model
	FlowName string
	WorkFlow []byte
}

func (cf *CodecFlow) ToGRPC() *ypb.CustomizeCodecFlow {
	var workFlow []*ypb.CodecWork
	err := json.Unmarshal(cf.WorkFlow, &workFlow)
	if err != nil {
		log.Errorf("unmarshal codec flow failed: %s", err)
	}
	return &ypb.CustomizeCodecFlow{
		FlowName: cf.FlowName,
		WorkFlow: workFlow,
	}
}