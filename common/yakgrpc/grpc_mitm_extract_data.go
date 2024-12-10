package yakgrpc

import (
	"bufio"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"path"
	"path/filepath"
	"time"
)

func (s *Server) QueryMITMRuleExtractedData(ctx context.Context, req *ypb.QueryMITMRuleExtractedDataRequest) (*ypb.QueryMITMRuleExtractedDataResponse, error) {
	db := s.GetProjectDatabase()
	if len(req.GetFilter().GetTraceID()) == 0 && req.GetHTTPFlowHiddenIndex() == "" && req.GetHTTPFlowHash() == "" {
		return nil, utils.Error("query mitm rule extracted data need hiddenIndex at last")
	}
	p, data, err := yakit.QueryExtractedDataPagination(db, req)
	if err != nil {
		return nil, err
	}
	return &ypb.QueryMITMRuleExtractedDataResponse{
		Data: funk.Map(data, func(i *schema.ExtractedData) *ypb.MITMRuleExtractedData {
			return &ypb.MITMRuleExtractedData{
				Id:             int64(i.ID),
				CreatedAt:      i.CreatedAt.Unix(),
				SourceType:     i.SourceType,
				TraceId:        i.TraceId,
				Regexp:         utils.EscapeInvalidUTF8Byte([]byte(i.Regexp)),
				RuleName:       utils.EscapeInvalidUTF8Byte([]byte(i.RuleVerbose)),
				Data:           utils.EscapeInvalidUTF8Byte([]byte(i.Data)),
				Index:          int64(i.DataIndex),
				Length:         int64(i.Length),
				IsMatchRequest: i.IsMatchRequest,
			}
		}).([]*ypb.MITMRuleExtractedData),
		Total:      int64(p.TotalRecord),
		Pagination: req.GetPagination(),
	}, nil
}

func (s *Server) ExportMITMRuleExtractedData(req *ypb.ExportMITMRuleExtractedDataRequest, stream ypb.Yak_ExportMITMRuleExtractedDataServer) error {
	db := s.GetProjectDatabase()
	allCount, err := yakit.CountExtractedData(db, req.GetFilter())
	if err != nil {
		return err
	}
	db = yakit.FilterExtractedData(db, req.GetFilter())
	exportPath := req.GetExportFilePath()
	if exportPath == "" {
		exportPath = filepath.Join(consts.GetDefaultYakitBaseTempDir(), fmt.Sprintf("mitm_rule_extracted_data_%s.csv", time.Now().Format("20060102150405")))
	} else if !path.IsAbs(exportPath) {
		exportPath = filepath.Join(consts.GetDefaultYakitBaseTempDir(), exportPath)
	}
	if filepath.Ext(exportPath) != "csv" {
		exportPath = exportPath + ".csv"
	}

	exportFp, err := os.OpenFile(exportPath, os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return err
	}
	defer exportFp.Close()
	exportWriter := bufio.NewWriter(exportFp)
	var currentCount float64
	sendFeedBack := func(verbose string, increase float64) {
		currentCount += increase
		_ = stream.Send(&ypb.ExportMITMRuleExtractedDataResponse{
			Verbose:        verbose,
			ExportFilePath: exportPath,
			Percent:        currentCount / allCount,
		})
	}

	_, err = exportWriter.Write([]byte("提取规则名,数据内容\n"))
	if err != nil {
		return err
	}

	duplicateFilter := filter.NewFilter()

	for data := range bizhelper.YieldModel[*schema.ExtractedData](stream.Context(), db) {
		sendFeedBack(fmt.Sprintf("Exported records"), 1)
		line := fmt.Sprintf("%s,%s\n", utils.QuoteCSV(data.RuleVerbose), utils.QuoteCSV(data.Data))
		if duplicateFilter.Exist(line) {
			continue
		}
		duplicateFilter.Insert(line)
		_, err = exportWriter.Write([]byte(line))
		if err != nil {
			return err
		}
	}
	exportWriter.Flush()

	_ = stream.Send(&ypb.ExportMITMRuleExtractedDataResponse{
		Verbose:        "Exported all records successfully",
		ExportFilePath: exportPath,
		Percent:        1,
	})
	return nil
}
