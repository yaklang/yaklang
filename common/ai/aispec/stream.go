package aispec

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

func ChatWithStream(url string, model string, msg string, opt func() ([]poc.PocConfigOption, error)) error {
	opts, err := opt()
	if err != nil {
		return utils.Wrap(err, "failed to get options")
	}

	msgIns := NewChatMessage(model, []ChatDetail{NewUserChatDetail(msg)})
	msgIns.Stream = true

	raw, err := json.Marshal(msgIns)
	if err != nil {
		return utils.Wrap(err, "json.Marshal failed")
	}

	opts = append(opts, poc.WithReplaceHttpPacketBody(raw, false))
	rsp, _, err := poc.DoPOST(url, opts...)
	if err != nil {
		return utils.Wrap(err, "failed to post request")
	}
	_ = rsp
	return nil
}
