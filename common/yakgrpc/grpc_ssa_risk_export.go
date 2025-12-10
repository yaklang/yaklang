package yakgrpc

import (
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) ExportSSARisk(req *ypb.ExportSSARiskRequest, stream ypb.Yak_ExportSSARiskServer) error {
	if req == nil {
		return utils.Error("ExportSSARisk Failed:ExportSSARiskRequest is nil")
	}
	filePath := req.GetTargetPath()
	if filePath == "" {
		return utils.Error("ExportSSARisk Failed:TargetPath is empty")
	}

	// 创建文件所在的目录
	targetDir := filepath.Dir(filePath)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return utils.Wrapf(err, "ExportSSARisk Failed:MkdirAll error")
	}

	fp, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
	if err != nil {
		return utils.Wrapf(err, "ExportSSARisk Failed:OpenFile error")
	}
	defer fp.Close()

	reporter := sfreport.NewReport(
		sfreport.IRifyFullReportType,
		sfreport.WithDataflowPath(req.GetWithDataFlowPath()),
		sfreport.WithFileContent(req.GetWithFileContent()),
	)
	db := ssadb.GetDB()
	filter := req.GetFilter()
	allCount, err := yakit.QuerySSARiskCount(db, filter)
	if err != nil {
		return utils.Wrapf(err, "ExportSSARisk Failed:QuerySSARiskCount error")
	}
	allCount += 1 // add 1 for writing reporter to file

	db = yakit.FilterSSARisk(db, filter)
	ch := yakit.YieldSSARisk(db, stream.Context())

	process := 0.0
	handled := 0
	sendFeedBack := func(verbose string, increase int) {
		handled += increase
		process = float64(handled) / float64(allCount)
		stream.Send(&ypb.ExportSSARiskResponse{
			Verbose: verbose,
			Process: process,
		})
	}
	// Add risks to reporter
	for risk := range ch {
		reporter.AddSyntaxFlowRisks(risk)
		sendFeedBack("Exported records", 1)
	}
	// write reporter to file
	err = reporter.PrettyWrite(fp)
	if err != nil {
		return utils.Wrapf(err, "ExportSSARisk Failed:PrettyWrite error")
	}
	sendFeedBack("Exported all records successfully", 1)
	return nil
}

func (s *Server) ImportSSARisk(req *ypb.ImportSSARiskRequest, stream ypb.Yak_ImportSSARiskServer) error {
	if req == nil {
		return utils.Error("ImportSSARisk Failed:ImportSSARiskRequest is nil")
	}
	path := req.GetInputPath()
	if path == "" {
		return utils.Error("ImportSSARisk Failed:InputPath is empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return utils.Wrapf(err, "ImportSSARisk Failed:ReadFile error")
	}
	db := ssadb.GetDB()
	ctx := stream.Context()

	err = sfreport.ImportSSARiskFromJSON(ctx, db, raw, func(msg string, progress float64) {
		stream.Send(&ypb.ImportSSARiskResponse{
			Verbose: msg,
			Process: progress,
		})
	})
	if err != nil {
		return utils.Wrapf(err, "ImportSSARisk Failed:ImportSSARiskFromJSON error")
	}
	return nil
}
