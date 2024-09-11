package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

func main() {
	typ := os.Args[1]
	// get post-scan scripts
	db := consts.GetGormProfileDatabase()
	db = db.Where("type = ?", typ)
	db = db.Order("created_at desc")

	fp, err := os.OpenFile("result.csv", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Errorf("open file failed: %s", err)
		return
	}

	handlerOutput := func(plugin *schema.YakScript, res *ypb.SmokingEvaluatePluginResponse) {
		// write to cvs file, plugin-name | score | result
		line := fmt.Sprintf("%s,%d,", plugin.ScriptName, res.Score)
		for _, r := range res.Results {
			line += fmt.Sprintf("%s,", strconv.Quote(r.Severity+"--"+r.Item))
		}
		line += "\n"
		fp.WriteString(line)
	}
	errorOutput := func(plugin *schema.YakScript, err error) {
		line := fmt.Sprintf("%s,0,%s\n", plugin.ScriptName, err.Error())
		fp.WriteString(line)
	}

	// project and profile
	consts.InitializeYakitDatabase("", "")

	// cve
	_, err = consts.InitializeCVEDatabase()
	if err != nil {
		log.Warnf("initialized cve database failed: %v", err)
	}

	// 调用一些数据库初始化的操作
	err = yakit.CallPostInitDatabase()
	if err != nil {
		return
	}

	// client
	// client, err := NewLocalClient()
	// require.NoError(t, err)
	s, err := yakgrpc.NewServer(yakgrpc.WithInitFacadeServer(true))
	if err != nil {
		log.Errorf("build yakit server failed: %s", err)
		// finalErr = err
		return
	}
	grpcTrans := grpc.NewServer(
		grpc.MaxRecvMsgSize(100*1024*1024),
		grpc.MaxSendMsgSize(100*1024*1024),
	)
	ypb.RegisterYakServer(grpcTrans, s)

	pluginTestingServer := yakgrpc.NewPluginTestingEchoServer(context.Background())

	// smoking
	for script := range yakit.YieldYakScripts(db, context.Background()) {
		// script.ScriptName == "Weblogic CVE-2014-4210 SSRF漏洞检测"
		res, err := s.EvaluatePlugin(context.Background(), script.Content, script.Type, pluginTestingServer)
		if err != nil {
			log.Errorf("error plugin:%s err: %v", script.ScriptName, err)
			errorOutput(script, err)
		} else {
			handlerOutput(script, res)
		}
	}
}
