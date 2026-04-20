//go:build !yakit_exclude

package yakgrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryMITMExtractedAggregate(ctx context.Context, req *ypb.QueryMITMExtractedAggregateRequest) (*ypb.QueryMITMExtractedAggregateResponse, error) {
	db := s.GetProjectDatabase()
	p, rows, distinctGroups, err := yakit.QueryMITMExtractedAggregate(db, req)
	if err != nil {
		return nil, err
	}
	pg := req.GetPagination()
	if pg == nil {
		pg = &ypb.Paging{Page: 1, Limit: 30, OrderBy: "hit_count", Order: "desc"}
	}
	resp := &ypb.QueryMITMExtractedAggregateResponse{
		Data:       rows,
		Total:      int64(p.TotalRecord),
		Pagination: pg,
	}
	if req != nil && req.GetIncludeDistinctRuleGroups() {
		resp.DistinctRuleGroups = distinctGroups
	}
	return resp, nil
}

func (s *Server) QueryMITMRuleExtractedData(ctx context.Context, req *ypb.QueryMITMRuleExtractedDataRequest) (*ypb.QueryMITMRuleExtractedDataResponse, error) {
	db := s.GetProjectDatabase()
	f := req.GetFilter()
	if f == nil || (len(f.GetTraceID()) == 0 && len(f.GetRuleVerbose()) == 0 && len(f.GetAnalyzedIds()) == 0) {
		return nil, utils.Error("query mitm rule extracted data need filter (TraceID, RuleVerbose or AnalyzedIds)")
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

func (s *Server) DeleteMITMRuleExtractedData(ctx context.Context, req *ypb.DeleteMITMRuleExtractedDataRequest) (*ypb.Empty, error) {
	if req == nil || req.GetFilter() == nil {
		return &ypb.Empty{}, nil
	}
	filter := req.GetFilter()
	// 至少需要一种过滤条件，避免误删全表
	if len(filter.GetIds()) == 0 && len(filter.GetTraceID()) == 0 && len(filter.GetRuleVerbose()) == 0 &&
		len(filter.GetAnalyzedIds()) == 0 && filter.GetKeyword() == "" {
		return &ypb.Empty{}, nil
	}
	db := s.GetProjectDatabase().Model(&schema.ExtractedData{})
	db = yakit.FilterExtractedData(db, filter)
	if res := db.Unscoped().Delete(&schema.ExtractedData{}); res.Error != nil {
		return nil, res.Error
	}
	return &ypb.Empty{}, nil
}

// DeduplicateMITMRuleExtractedData 按 trace_id+规则名+规则数据去重，即对指定包内的提取数据去重，
// 删除重复项（保留 id 最小的一条）。Filter 为空或 Filter.TraceID 为空时对全表去重。
func (s *Server) DeduplicateMITMRuleExtractedData(ctx context.Context, req *ypb.DeduplicateMITMRuleExtractedDataRequest) (*ypb.DeduplicateMITMRuleExtractedDataResponse, error) {
	db := s.GetProjectDatabase()
	filter := req.GetFilter()
	var traceIds []string
	if filter != nil {
		traceIds = filter.GetTraceID()
	}
	deleted, err := yakit.DeduplicateExtractedData(db, traceIds...)
	if err != nil {
		return nil, err
	}
	return &ypb.DeduplicateMITMRuleExtractedDataResponse{DeletedCount: deleted}, nil
}

func (s *Server) ExportMITMRuleExtractedData(req *ypb.ExportMITMRuleExtractedDataRequest, stream ypb.Yak_ExportMITMRuleExtractedDataServer) error {
	db := s.GetProjectDatabase()
	allCount, err := yakit.CountExtractedData(db, req.GetFilter())
	if err != nil {
		return err
	}
	db = yakit.FilterExtractedData(db, req.GetFilter())
	exportPath := req.GetExportFilePath()
	exportTyp := req.GetType()
	if exportTyp == "" {
		exportTyp = "csv"
	}
	if exportTyp != "csv" && exportTyp != "json" {
		return utils.Error("export type must be csv or json")
	}
	if exportPath == "" {
		exportPath = filepath.Join(consts.GetDefaultYakitBaseTempDir(), fmt.Sprintf("mitm_rule_extracted_data_%s.%s", time.Now().Format("20060102150405"), exportTyp))
	} else if !path.IsAbs(exportPath) {
		exportPath = filepath.Join(consts.GetDefaultYakitBaseTempDir(), exportPath)
	}
	if !strings.HasSuffix(filepath.Ext(exportPath), exportTyp) {
		exportPath = exportPath + "." + exportTyp
	}

	exportFp, err := os.OpenFile(exportPath, os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return err
	}

	var currentCount float64
	sendFeedBack := func(verbose string, increase float64) {
		currentCount += increase
		_ = stream.Send(&ypb.ExportMITMRuleExtractedDataResponse{
			Verbose:        verbose,
			ExportFilePath: exportPath,
			Percent:        currentCount / allCount,
		})
	}

	duplicateFilter := filter.NewFilter()
	defer exportFp.Close()
	switch exportTyp {
	case "csv":
		exportWriter := bufio.NewWriter(exportFp)
		_, err = exportWriter.Write([]byte("提取规则名,数据内容\n"))
		if err != nil {
			return err
		}
		for data := range bizhelper.YieldModel[*schema.ExtractedData](stream.Context(), db) {
			sendFeedBack("Exported records", 1)
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
	case "json":
		encoder := json.NewEncoder(exportFp)
		exportFp.WriteString("[")
		for data := range bizhelper.YieldModel[*schema.ExtractedData](stream.Context(), db) {
			sendFeedBack("Exported records", 1)
			hash := utils.CalcSha256(data.RuleVerbose, data.Data)
			if duplicateFilter.Exist(hash) {
				continue
			}
			duplicateFilter.Insert(hash)
			if currentCount > 1 {
				exportFp.WriteString(",")
			}
			var result = make(map[string]interface{})
			result["提取规则名"] = data.RuleVerbose
			result["数据内容"] = data.Data
			err = encoder.Encode(result)
			if err != nil {
				return err
			}
		}
		exportFp.WriteString("]")
	}
	_ = stream.Send(&ypb.ExportMITMRuleExtractedDataResponse{
		Verbose:        "Exported all records successfully",
		ExportFilePath: exportPath,
		Percent:        1,
	})
	return nil
}
