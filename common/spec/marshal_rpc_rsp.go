package spec

import (
	"encoding/json"
	"github.com/pkg/errors"
	"yaklang.io/yaklang/common/spec/auditlog"
	"yaklang.io/yaklang/common/spec/hidsevent"
	"yaklang.io/yaklang/common/utils"
)

func UnmarshalRPC_APIResponse(apiName string, response []byte) (interface{}, error) {
	switch apiName {
	case HIDS_API_Sleep:
		var rsp hidsevent.RpcSleepResponse
		err := json.Unmarshal(response, &rsp)
		if err != nil {
			return nil, errors.Errorf("unmarshal %v's response failed: %v", apiName, err)
		}
		return &rsp, nil
	case auditlog.LogAgentAPI_QueryAuditLog:
		var rsp auditlog.QueryAuditLogResponse
		err := json.Unmarshal(response, &rsp)
		if err != nil {
			return nil, utils.Errorf("unmarshal %v failed: %s", auditlog.LogAgentAPI_QueryAuditLog, err)
		}
		return &rsp, nil
	default:
		var rsp interface{}
		err := json.Unmarshal(response, &rsp)
		if err != nil {
			return nil, utils.Errorf("unmarshal response failed: %s", err)
		}
		return &rsp, nil
	}
}
