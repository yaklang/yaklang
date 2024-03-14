package chatglm

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"time"
)

func post(apiURL, token string, params map[string]interface{}, timeout time.Duration) (map[string]interface{}, error) {

	rsp, req, err := poc.DoPOST(
		apiURL,
		poc.WithReplaceAllHttpPacketHeaders(map[string]string{
			"Accept":        "application/json",
			"Content-Type":  "application/json; charset=UTF-8",
			"Authorization": token,
		}),
		poc.WithReplaceHttpPacketBody(utils.Jsonify(params), false),
	)
	if err != nil {
		return nil, fmt.Errorf("request post to %v：%v", apiURL, err)
	}
	_ = req

	var result = make(map[string]any)
	err = json.Unmarshal(rsp.GetBody(), &result)
	if err != nil {
		return nil, fmt.Errorf("JSON response failed：%w", err)
	}
	if utils.MapGetString(result, "error") != "" {
		return nil, fmt.Errorf("API to %v error: %s, message: %v\n\n\n", apiURL, utils.MapGetString(result, "error"), string(rsp.GetBody()))
	}
	return result, nil
}
