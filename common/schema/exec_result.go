package schema

import (
	"encoding/json"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type ExecResult struct {
	gorm.Model

	YakScriptName string `json:"yak_script_name" gorm:"index"`
	Raw           string `json:"raw"`
}

func (e *ExecResult) ToGRPCModel() *ypb.ExecResult {
	var res ypb.ExecResult
	err := json.Unmarshal([]byte(e.Raw), &res)
	if err != nil {
		return nil
	}
	res.Id = int64(e.ID)
	return &res
}
