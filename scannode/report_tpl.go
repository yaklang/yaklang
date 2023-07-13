package scannode

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mq"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/scannode/scanrpc"
)

//go:embed yak_scripts/gen_report.yak
var embedGenReport []byte

const GENREPORT_KEY = "JznQXuFDSepeNWHbiLGEwONiaBxhvj_SERVER_SCAN_MANAGER"

func genReportFromKey(ctx context.Context, node string, helper *scanrpc.SCANServerHelper, broker *mq.Broker, req *scanrpc.SCAN_InvokeScriptRequest) error {
	if value := yakit.GetKey(consts.GetGormProjectDatabase(), GENREPORT_KEY); value != "" {
		yakit.DelKey(consts.GetGormProjectDatabase(), GENREPORT_KEY)
		genGeport := &scanrpc.SCAN_InvokeScriptRequest{
			TaskId:          req.TaskId,
			RuntimeId:       req.RuntimeId,
			SubTaskId:       req.SubTaskId,
			ScriptContent:   string(embedGenReport),
			ScriptJsonParam: value,
		}
		_, err := helper.DoSCAN_InvokeScript(
			ctx,
			node, genGeport,
			broker,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
