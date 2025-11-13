package yakgrpc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// GenerateSSAReport 生成SSA扫描报告
// 支持两种生成方式：
// 1. 基于TaskID：生成整个扫描任务的完整报告
// 2. 基于Filter：使用SSARisksFilter过滤器生成报告
// 支持多种报告格式：
// - yakit (默认): Yak原生报告格式，保存到数据库
// - sarif: SARIF格式，保存到文件
// - irify: IRify格式，保存到文件（简化版）
// - irify-full: IRify完整格式，保存到文件（包含完整信息）
func (s *Server) GenerateSSAReport(ctx context.Context, req *ypb.GenerateSSAReportRequest) (*ypb.GenerateSSAReportResponse, error) {
	// 参数验证：TaskID和Filter至少需要一个
	if req.GetTaskID() == "" && req.GetFilter() == nil {
		return nil, utils.Errorf("taskID or filter is required")
	}

	// 获取报告类型，默认为 yakit
	reportKind := req.GetKind()
	if reportKind == "" {
		reportKind = "yakit"
	}

	// 设置报告名称
	reportName := req.GetReportName()
	if reportName == "" {
		reportName = fmt.Sprintf("%s_%s", "SSA项目扫描报告", time.Now().Format("20060102150405"))
	}

	// 根据报告类型选择不同的处理逻辑
	switch reportKind {
	case "yakit":
		return s.generateYakitReport(ctx, req, reportName)
	case "irify":
		return s.generateFileReport(ctx, req, reportName, sfreport.IRifyReportType)
	case "irify-full":
		return s.generateFileReport(ctx, req, reportName, sfreport.IRifyFullReportType)
	case "sarif":
		return s.generateSarifReport(ctx, req, reportName)
	default:
		return nil, utils.Errorf("unsupported report kind: %s, supported kinds: yakit, irify, irify-full, sarif", reportKind)
	}
}

// generateYakitReport 生成Yak原生报告格式（保存到数据库）
func (s *Server) generateYakitReport(ctx context.Context, req *ypb.GenerateSSAReportRequest, reportName string) (*ypb.GenerateSSAReportResponse, error) {
	var ssaReport *sfreport.SSAProjectReport
	var err error

	// 两种独立的报告生成路径
	if req.GetFilter() != nil {
		// 路径1：基于Filter生成报告（支持灵活的风险筛选）
		log.Infof("generating yakit report from filter")
		ssaReport, err = sfreport.GenerateSSAProjectReportFromFilter(ctx, req.GetFilter())
		if err != nil {
			log.Errorf("generate ssa project report from filter failed: %v", err)
			return nil, utils.Wrapf(err, "generate ssa project report from filter failed")
		}
	} else {
		// 路径2：基于TaskID生成完整扫描任务报告（原有功能）
		log.Infof("generating yakit report from task ID: %s", req.GetTaskID())
		db := s.GetSSADatabase()
		task, err := schema.GetSyntaxFlowScanTaskById(db, req.GetTaskID())
		if err != nil {
			log.Errorf("get syntax flow scan task failed: %v", err)
			return nil, utils.Wrapf(err, "get syntax flow scan task failed")
		}

		ssaReport, err = sfreport.GenerateSSAProjectReportFromTask(ctx, task)
		if err != nil {
			log.Errorf("generate ssa project report failed: %v", err)
			return nil, utils.Wrapf(err, "generate ssa project report failed")
		}
	}

	// 创建IRify报告实例
	reportInstance := yakit.NewReport()
	reportInstance.From("ssa-scan")
	reportInstance.Title(reportName)

	// 生成报告内容
	err = sfreport.GenerateYakitReportContent(reportInstance, ssaReport)
	if err != nil {
		log.Errorf("generate yakit report content failed: %v", err)
		return nil, utils.Wrapf(err, "generate yakit report content failed")
	}

	// 保存报告
	reportID := reportInstance.SaveForIRify()
	if reportID == 0 {
		return nil, utils.Errorf("save report failed")
	}

	return &ypb.GenerateSSAReportResponse{
		ReportData: strconv.Itoa(reportID),
		Success:    true,
		Message:    "SSA扫描报告生成成功",
	}, nil
}

// generateFileReport 生成文件格式报告（写入到文件系统）
func (s *Server) generateFileReport(ctx context.Context, req *ypb.GenerateSSAReportRequest, reportName string, reportType sfreport.ReportType) (*ypb.GenerateSSAReportResponse, error) {
	var risks []*schema.SSARisk
	var err error

	// 获取风险数据
	db := ssadb.GetDB()
	if req.GetFilter() != nil {
		// 使用Filter获取风险
		log.Infof("generating %s report from filter", reportType)
		filteredDB := yakit.FilterSSARisk(db, req.GetFilter())
		ch := yakit.YieldSSARisk(filteredDB, ctx)
		for risk := range ch {
			risks = append(risks, risk)
		}
	} else {
		// 使用TaskID获取风险
		log.Infof("generating %s report from task ID: %s", reportType, req.GetTaskID())
		task, err := schema.GetSyntaxFlowScanTaskById(db, req.GetTaskID())
		if err != nil {
			log.Errorf("get syntax flow scan task failed: %v", err)
			return nil, utils.Wrapf(err, "get syntax flow scan task failed")
		}

		// 通过RuntimeID（TaskID）获取风险
		filter := &ypb.SSARisksFilter{
			RuntimeID: []string{task.TaskId},
		}
		filteredDB := yakit.FilterSSARisk(db, filter)
		ch := yakit.YieldSSARisk(filteredDB, ctx)
		for risk := range ch {
			risks = append(risks, risk)
		}
	}

	if len(risks) == 0 {
		return nil, utils.Errorf("no risks found")
	}

	// 创建报告目录：yakit_home/temp
	reportDir := consts.GetDefaultYakitBaseTempDir()
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		log.Errorf("create report directory failed: %v", err)
		return nil, utils.Wrapf(err, "create report directory failed")
	}

	// 生成文件名
	timestamp := time.Now().Format("20060102_150405")
	var fileExt string
	switch reportType {
	case sfreport.SarifReportType:
		fileExt = "sarif"
	case sfreport.IRifyReportType, sfreport.IRifyFullReportType:
		fileExt = "json"
	default:
		fileExt = "json"
	}

	fileName := fmt.Sprintf("ssa_report_%s_%s.%s", reportName, timestamp, fileExt)
	// 清理文件名中的非法字符
	fileName = sanitizeFileName(fileName)
	filePath := filepath.Join(reportDir, fileName)

	// 创建文件
	fp, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		log.Errorf("create report file failed: %v", err)
		return nil, utils.Wrapf(err, "create report file failed")
	}
	defer fp.Close()

	// 根据报告类型设置选项
	var reportOpts []sfreport.Option
	if reportType == sfreport.IRifyFullReportType {
		reportOpts = []sfreport.Option{
			sfreport.WithDataflowPath(true),
			sfreport.WithFileContent(true),
		}
	} else {
		reportOpts = []sfreport.Option{
			sfreport.WithDataflowPath(false),
			sfreport.WithFileContent(false),
		}
	}

	// 创建报告
	report := sfreport.NewReport(reportType, reportOpts...)

	// 添加风险到报告
	for _, risk := range risks {
		reportRisk, _ := sfreport.NewRisk(risk, report)
		report.AddRisks(reportRisk)
	}

	// 设置报告信息
	if len(risks) > 0 {
		report.SetProgramName(risks[0].ProgramName)
	}

	// 设置Writer并保存
	err = report.SetWriter(fp)
	if err != nil {
		log.Errorf("set report writer failed: %v", err)
		return nil, utils.Wrapf(err, "set report writer failed")
	}

	err = report.Save()
	if err != nil {
		log.Errorf("save report failed: %v", err)
		return nil, utils.Wrapf(err, "save report failed")
	}

	log.Infof("report saved to: %s", filePath)

	return &ypb.GenerateSSAReportResponse{
		ReportData: filePath,
		Success:    true,
		Message:    fmt.Sprintf("%s报告生成成功，包含 %d 个风险，已保存到: %s", reportType, len(risks), filePath),
	}, nil
}

// generateSarifReport 生成SARIF格式报告（写入到文件系统）
func (s *Server) generateSarifReport(ctx context.Context, req *ypb.GenerateSSAReportRequest, reportName string) (*ypb.GenerateSSAReportResponse, error) {
	db := ssadb.GetDB()

	// 获取 SyntaxFlowResult 列表
	var resultIDs []uint64
	if req.GetFilter() != nil {
		// 使用Filter获取风险，然后提取ResultID
		log.Infof("generating sarif report from filter")
		filteredDB := yakit.FilterSSARisk(db, req.GetFilter())
		ch := yakit.YieldSSARisk(filteredDB, ctx)

		resultIDSet := make(map[uint64]struct{})
		for risk := range ch {
			if risk.ResultID > 0 {
				resultIDSet[risk.ResultID] = struct{}{}
			}
		}

		for id := range resultIDSet {
			resultIDs = append(resultIDs, id)
		}
	} else {
		// 使用TaskID获取所有SyntaxFlowResult
		log.Infof("generating sarif report from task ID: %s", req.GetTaskID())
		task, err := schema.GetSyntaxFlowScanTaskById(db, req.GetTaskID())
		if err != nil {
			log.Errorf("get syntax flow scan task failed: %v", err)
			return nil, utils.Wrapf(err, "get syntax flow scan task failed")
		}

		// 通过TaskID获取所有结果
		var results []*ssadb.AuditResult
		if err := ssadb.GetDB().Where("task_id = ?", task.TaskId).Find(&results).Error; err != nil {
			log.Errorf("query syntax flow results failed: %v", err)
			return nil, utils.Wrapf(err, "query syntax flow results failed")
		}

		for _, result := range results {
			resultIDs = append(resultIDs, uint64(result.ID))
		}
	}

	if len(resultIDs) == 0 {
		return nil, utils.Errorf("no syntax flow results found")
	}

	// 创建报告目录
	reportDir := consts.GetDefaultYakitBaseTempDir()
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		log.Errorf("create report directory failed: %v", err)
		return nil, utils.Wrapf(err, "create report directory failed")
	}

	// 生成文件名
	timestamp := time.Now().Format("20060102_150405")
	fileName := fmt.Sprintf("ssa_report_%s_%s.sarif", reportName, timestamp)
	fileName = sanitizeFileName(fileName)
	filePath := filepath.Join(reportDir, fileName)

	// 创建文件
	fp, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		log.Errorf("create report file failed: %v", err)
		return nil, utils.Wrapf(err, "create report file failed")
	}
	defer fp.Close()

	// 创建SARIF报告
	sarifReport, err := sfreport.NewSarifReport()
	if err != nil {
		log.Errorf("create sarif report failed: %v", err)
		return nil, utils.Wrapf(err, "create sarif report failed")
	}

	// 设置Writer
	err = sarifReport.SetWriter(fp)
	if err != nil {
		log.Errorf("set report writer failed: %v", err)
		return nil, utils.Wrapf(err, "set report writer failed")
	}

	// 加载并添加所有SyntaxFlowResult
	addedCount := 0
	for _, resultID := range resultIDs {
		result, err := ssaapi.LoadResultByID(uint(resultID))
		if err != nil {
			log.Warnf("load result %d failed: %v, skip", resultID, err)
			continue
		}

		if sarifReport.AddSyntaxFlowResult(result) {
			addedCount++
		}
	}

	if addedCount == 0 {
		return nil, utils.Errorf("no valid syntax flow results added to sarif report")
	}

	// 保存报告
	err = sarifReport.Save()
	if err != nil {
		log.Errorf("save sarif report failed: %v", err)
		return nil, utils.Wrapf(err, "save sarif report failed")
	}

	log.Infof("sarif report saved to: %s", filePath)

	return &ypb.GenerateSSAReportResponse{
		ReportData: filePath,
		Success:    true,
		Message:    fmt.Sprintf("SARIF报告生成成功，包含 %d 个扫描结果，已保存到: %s", addedCount, filePath),
	}, nil
}
