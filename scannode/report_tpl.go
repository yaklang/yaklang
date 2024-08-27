package scannode

import (
	"context"
	_ "embed"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mq"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/scannode/scanrpc"
)

//go:embed yak_scripts/gen_report.yak
var EmbedGenReport []byte

const GENREPORT_KEY = "JznQXuFDSepeNWHbiLGEwONiaBxhvj_SERVER_SCAN_MANAGER"

func genReportFromKey(ctx context.Context, node string, helper *scanrpc.SCANServerHelper, broker *mq.Broker, req *scanrpc.SCAN_InvokeScriptRequest) error {
	if value := yakit.GetKey(consts.GetGormProfileDatabase(), GENREPORT_KEY); value != "" {
		yakit.DelKey(consts.GetGormProfileDatabase(), GENREPORT_KEY)
		genGeport := &scanrpc.SCAN_InvokeScriptRequest{
			TaskId:          req.TaskId,
			RuntimeId:       req.RuntimeId,
			SubTaskId:       req.SubTaskId,
			ScriptContent:   string(EmbedGenReport),
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
		log.Info("genReportFromKey success")
	}
	return nil
}
