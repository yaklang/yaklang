package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"yaklang/common/consts"
	"yaklang/common/go-funk"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/bizhelper"
	"yaklang/common/utils/webforest"
	"yaklang/common/yak/yaklib"
	"yaklang/common/yakgrpc/yakit"
	"yaklang/common/yakgrpc/ypb"
)

var saveYakitLogLock = new(sync.Mutex)

func SaveFromYakitLog(l *yaklib.YakitLog, db *gorm.DB) {
	saveYakitLogLock.Lock()
	defer saveYakitLogLock.Unlock()

	switch l.Level {
	case "asset-port":
		var port yakit.Port
		err := json.Unmarshal([]byte(l.Data), &port)
		if err != nil {
			log.Errorf("unmarshal yakit.Port failed: %s", err)
			return
		}

		err = yakit.CreateOrUpdatePort(db, port.CalcHash(), &port)
		if err != nil {
			log.Errorf("save yakit.Port failed: %s", err)
			return
		}
		return
	}
}

func (s *Server) GenerateWebsiteTree(ctx context.Context, req *ypb.GenerateWebsiteTreeRequest) (
	*ypb.GenerateWebsiteTreeResponse, error,
) {
	targets := utils.PrettifyListFromStringSplited(req.Targets, ",")

	db := s.GetProjectDatabase() // .Debug()
	db = db.Table("http_flows").Select("url")
	db = bizhelper.FuzzQueryStringArrayOrLike(db, "url", targets)
	forest := webforest.NewWebsiteForest(50)

	if targets == nil {
		ctx = utils.TimeoutContextSeconds(3)
	}
	res := yakit.YieldHTTPUrl(db, ctx)
	for r := range res {
		forest.AddNode(r.Url)
	}

	raw, err := json.Marshal(forest.ToBasicOutput())
	if err != nil {
		return nil, err
	}

	return &ypb.GenerateWebsiteTreeResponse{TreeDataJson: raw}, nil
}

func fixUTF8(i string) string {
	return utils.EscapeInvalidUTF8Byte([]byte(i))
}

func (s *Server) QueryPorts(ctx context.Context, req *ypb.QueryPortsRequest) (*ypb.QueryPortsResponse, error) {
	p, res, err := yakit.QueryPorts(s.GetProjectDatabase(), req)
	if err != nil {
		return nil, err
	}
	var results []*ypb.Port
	if !req.GetAll() { // 分页
		for _, r := range res {
			results = append(results, ToGrpcPort(r))
		}
	} else { // 全部
		db := yakit.FilterPort(s.GetProjectDatabase(), req)
		data := yakit.YieldPorts(db, context.Background())
		for r := range data {
			results = append(results, ToGrpcPort(r))
		}
	}

	return &ypb.QueryPortsResponse{
		Pagination: req.Pagination,
		Total:      int64(p.TotalRecord),
		Data:       results,
	}, nil
}

func ToGrpcPort(r *yakit.Port) *ypb.Port {
	return &ypb.Port{
		Host:        utils.EscapeInvalidUTF8Byte([]byte(r.Host)),
		IPInteger:   int64(r.IPInteger),
		Port:        int64(r.Port),
		Proto:       fixUTF8(r.Proto),
		ServiceType: fixUTF8(r.ServiceType),
		State:       fixUTF8(r.State),
		Reason:      fixUTF8(r.Reason),
		Fingerprint: fixUTF8(r.Fingerprint),
		CPE: funk.Map(strings.Split(r.CPE, "|"), func(i string) string {
			return fixUTF8(i)
		}).([]string),
		HtmlTitle: fixUTF8(r.HtmlTitle),
		Id:        int64(r.ID),
		CreatedAt: r.CreatedAt.Unix(),
		UpdatedAt: r.UpdatedAt.Unix(),
		TaskName:  fixUTF8(r.TaskName),
	}
}

func (s *Server) DeletePorts(ctx context.Context, req *ypb.DeletePortsRequest) (*ypb.Empty, error) {
	db := s.GetProjectDatabase().Model(&ypb.Port{})

	if req.GetAll() {
		db.Unscoped().Where("true").Delete(&yakit.Port{})
		return &ypb.Empty{}, nil
	}
	if req.GetDeleteAll() {
		db.Unscoped().Where("true").Delete(&yakit.Port{})
		return &ypb.Empty{}, nil
	}
	if req.GetFilter() != nil {
		rdb, _ := yakit.FilterByQueryPorts(s.GetProjectDatabase(), req.GetFilter())
		if rdb != nil {
			if db := rdb.Unscoped().Delete(&yakit.Port{}); db.Error != nil {
				return nil, utils.Errorf("delete error: %s", db.Error)
			}
		}
		return &ypb.Empty{}, nil
	}
	if len(req.GetIds()) > 0 {
		for _, i := range req.GetIds() {
			yakit.DeletePortsByID(s.GetProjectDatabase(), i)
		}
		return &ypb.Empty{}, nil
	}

	if req.GetHosts() == "" && req.GetPorts() == "" && req.GetId() == nil {
		return &ypb.Empty{}, nil
	}

	if req.GetHosts() != "" {
		db = bizhelper.QueryBySpecificAddress(db, "ip_integer", req.GetHosts())
	}

	if req.GetPorts() != "" {
		db = bizhelper.QueryBySpecificPorts(db, "port", req.GetPorts())
	}

	if req.GetId() != nil {
		db = bizhelper.ExactQueryInt64ArrayOr(db, "id", req.GetId())
	}

	db.Unscoped().Delete(&yakit.Port{})

	return &ypb.Empty{}, nil
}

func (s *Server) DeleteHosts(ctx context.Context, req *ypb.DeleteHostsRequest) (*ypb.Empty, error) {
	if req.DeleteAll {
		s.GetProjectDatabase().Model(&yakit.Host{}).Unscoped().Delete(&yakit.Host{})
		return &ypb.Empty{}, nil
	}

	var db = s.GetProjectDatabase().Model(&yakit.Host{}).Unscoped()
	if req.DomainKeyword != "" {
		bizhelper.FuzzQueryLike(db, "domain", req.DomainKeyword).Delete(&yakit.Host{})
		return &ypb.Empty{}, nil
	}

	if req.Network != "" {
		bizhelper.QueryBySpecificAddress(db, "ip_integer", req.Network).Delete(&yakit.Host{})
		return &ypb.Empty{}, nil
	}

	if req.ID > 0 {
		_ = yakit.DeleteHostByID(db, req.ID)
		return &ypb.Empty{}, nil
	}

	return &ypb.Empty{}, nil
}

func (s *Server) QueryHosts(ctx context.Context, req *ypb.QueryHostsRequest) (*ypb.QueryHostsResponse, error) {
	p, data, err := yakit.QueryHost(s.GetProjectDatabase(), req)
	if err != nil {
		return nil, err
	}

	var hosts []*ypb.Host
	for _, i := range data {
		hosts = append(hosts, &ypb.Host{
			Id:            int64(i.ID),
			IP:            i.IP,
			IPInteger:     i.IPInteger,
			IsInPublicNet: i.IsInPublicNet,
			Domains:       utils.PrettifyListFromStringSplited(i.Domains, ","),
		})
	}

	return &ypb.QueryHostsResponse{
		Pagination: req.Pagination,
		Total:      int64(p.TotalPage),
		Data:       hosts,
	}, nil
}

func (s *Server) DeleteDomains(ctx context.Context, req *ypb.DeleteDomainsRequest) (*ypb.Empty, error) {
	if req.DeleteAll {
		s.GetProjectDatabase().Model(&yakit.Domain{}).Unscoped().Delete(&yakit.Domain{})
		return &ypb.Empty{}, nil
	}
	if req.GetFilter() != nil {
		db := yakit.FilterDomain(s.GetProjectDatabase(), req.GetFilter())
		if db := db.Unscoped().Delete(&yakit.Domain{}); db.Error != nil {
			return &ypb.Empty{}, nil
		}
	}

	if len(req.GetIDs()) > 0 {
		for _, i := range req.GetIDs() {
			yakit.DeleteDomainByID(s.GetProjectDatabase(), i)
		}
		return &ypb.Empty{}, nil
	}

	var db = s.GetProjectDatabase().Model(&yakit.Domain{}).Unscoped()
	if req.DomainKeyword != "" {
		bizhelper.FuzzQueryLike(db, "domain", req.DomainKeyword).Delete(&yakit.Domain{})
		return &ypb.Empty{}, nil
	}

	if req.Network != "" {
		bizhelper.QueryBySpecificAddress(db, "ip_integer", req.Network).Delete(&yakit.Domain{})
		return &ypb.Empty{}, nil
	}

	if req.ID > 0 {
		_ = yakit.DeleteDomainByID(db, req.ID)
		return &ypb.Empty{}, nil
	}

	return &ypb.Empty{}, nil
}

func (s *Server) QueryDomains(ctx context.Context, req *ypb.QueryDomainsRequest) (*ypb.QueryDomainsResponse, error) {
	p, data, err := yakit.QueryDomain(s.GetProjectDatabase(), req)
	if err != nil {
		return nil, err
	}

	var results []*ypb.Domain
	if !req.GetAll() {
		for _, i := range data {
			results = append(results, &ypb.Domain{
				ID:         int64(i.ID),
				DomainName: i.Domain,
				IPAddr:     i.IPAddr,
				HTTPTitle:  i.HTTPTitle,
			})
		}
	} else {
		db := yakit.FilterDomain(s.GetProjectDatabase(), req)
		yieldDomains := yakit.YieldDomains(db, context.Background())
		for i := range yieldDomains {
			results = append(results, &ypb.Domain{
				ID:         int64(i.ID),
				DomainName: i.Domain,
				IPAddr:     i.IPAddr,
				HTTPTitle:  i.HTTPTitle,
			})
		}
	}

	return &ypb.QueryDomainsResponse{
		Pagination: req.Pagination,
		Total:      int64(p.TotalRecord),
		Data:       results,
	}, nil
}

func (s *Server) QueryRisks(ctx context.Context, req *ypb.QueryRisksRequest) (*ypb.QueryRisksResponse, error) {
	p, data, err := yakit.QueryRisks(s.GetProjectDatabase(), req)
	if err != nil {
		return nil, err
	}

	rsp := &ypb.QueryRisksResponse{
		Pagination: req.Pagination,
		Total:      int64(p.TotalRecord),
	}
	for _, r := range data {
		rsp.Data = append(rsp.Data, r.ToGRPCModel())
	}
	return rsp, nil
}

func (s *Server) QueryRisk(ctx context.Context, req *ypb.QueryRiskRequest) (*ypb.Risk, error) {
	if req.GetId() > 0 {
		r, err := yakit.GetRisk(s.GetProjectDatabase(), req.GetId())
		if err != nil {
			return nil, err
		}
		return r.ToGRPCModel(), nil
	}

	if req.GetHash() != "" {
		r, err := yakit.GetRiskByHash(s.GetProjectDatabase(), req.GetHash())
		if err != nil {
			return nil, err
		}
		return r.ToGRPCModel(), nil
	}

	return nil, utils.Errorf("empty search")
}

func (s *Server) DeleteRisk(ctx context.Context, req *ypb.DeleteRiskRequest) (*ypb.Empty, error) {
	if req.GetDeleteRepetition() {
		if req.GetFilter() != nil && req.GetId() > 0 {
			rdb, _ := yakit.FilterByQueryRisks(s.GetProjectDatabase(), req.GetFilter())
			if rdb != nil {
				if db := rdb.Unscoped().Where("id <> ?", req.GetId()).Delete(&yakit.Risk{}); db.Error != nil {
					return nil, utils.Errorf("delete error: %s", db.Error)
				}
			}
		}
		return &ypb.Empty{}, nil

	}

	if req.GetId() > 0 {
		_ = yakit.DeleteRiskByID(s.GetProjectDatabase(), req.GetId())
	}

	if req.GetHash() != "" {
		r, err := yakit.GetRiskByHash(s.GetProjectDatabase(), req.GetHash())
		if err != nil {
			return nil, err
		}
		_ = yakit.DeleteRiskByID(s.GetProjectDatabase(), int64(r.ID))
	}

	if req.GetFilter() != nil {
		rdb, _ := yakit.FilterByQueryRisks(s.GetProjectDatabase(), req.GetFilter())
		if rdb != nil {
			if db := rdb.Unscoped().Delete(&yakit.Risk{}); db.Error != nil {
				return nil, utils.Errorf("delete error: %s", db.Error)
			}
		}
	}

	if len(req.GetIds()) > 0 {
		_ = yakit.DeleteRiskByID(s.GetProjectDatabase(), req.GetIds()...)
		return &ypb.Empty{}, nil
	}

	if req.GetDeleteAll() {
		if db := s.GetProjectDatabase().Model(&yakit.Risk{}).Where("true").Unscoped().Delete(&yakit.Risk{}); db.Error != nil {
			return nil, db.Error
		}
	}

	return &ypb.Empty{}, nil
}

type riskType struct {
	RiskTypeVerbose string
	RiskType        string
	Total           int32
}

var riskTypeGroup = []string{
	"nuclei-",
	"reverse",
	"random-port-trigger",
}

func riskTypeGroupVerbose(i string, defaultStr string) string {
	switch i {
	case "nuclei-":
		return "漏洞(From Nuclei)"
	case "reverse":
		return "反连"
	case "random-port-trigger":
		return "随机端口反连"
	case "default":
		return "指纹/信息"
	default:
		return defaultStr
	}
}

func (s *Server) QueryAvailableRiskType(ctx context.Context, req *ypb.Empty) (*ypb.Fields, error) {
	var types []*riskType
	if rows, err := s.GetProjectDatabase().Table("risks").Raw(
		`select distinct risk_type_verbose, risk_type, count(*) as total from risks  where waiting_verified = false group by risk_type_verbose;`,
	).Rows(); err != nil {
		return nil, utils.Errorf("query risk types failed: %s", err)
	} else {
		for rows.Next() {
			var verbose string
			var typeStr string
			var total int32
			err := rows.Scan(&verbose, &typeStr, &total)
			if err != nil {
				log.Errorf("scan risk_type failed: %s", err)
				continue
			}
			types = append(types, &riskType{
				RiskTypeVerbose: verbose,
				RiskType:        typeStr,
				Total:           total,
			})
		}
	}

	var riskTypes = make(map[string]*ypb.FieldName)
	for _, t := range types {
		var typeStr = t.RiskType
		for _, prefix := range riskTypeGroup {
			if strings.HasPrefix(t.RiskType, prefix) {
				typeStr = prefix
			}
		}
		result, ok := riskTypes[typeStr]
		if !ok {
			result = &ypb.FieldName{
				Name:    typeStr,
				Verbose: riskTypeGroupVerbose(typeStr, t.RiskTypeVerbose),
				Total:   t.Total,
			}
			riskTypes[typeStr] = result
			continue
		}
		result.Total += t.Total
	}

	return &ypb.Fields{
		Values: funk.Values(riskTypes).([]*ypb.FieldName),
	}, nil
}

func severityVerbose(i string) string {
	i = strings.ToLower(i)
	switch i {
	case "trace", "debug", "note":
		return "调试信息"
	case "info", "fingerprint", "infof", "default":
		return "信息/指纹"
	case "low":
		return "低危"
	case "middle", "warn", "warning", "medium":
		return "中危"
	case "high":
		return "高危"
	case "fatal", "critical", "panic":
		return "严重"
	default:
		return fmt.Sprintf(`[%v]`, strings.ToUpper(i))
	}
}

var (
	fixRiskOnce = new(sync.Once)
)

func (s *Server) QueryAvailableRiskLevel(ctx context.Context, _ *ypb.Empty) (*ypb.Fields, error) {
	fixRiskOnce.Do(func() {
		yakit.FixRiskType(s.GetProjectDatabase())
	})
	var severities = make(map[string]*ypb.FieldName)
	if rows, err := s.GetProjectDatabase().Raw(
		`select distinct severity, count(*) as total from risks where waiting_verified = false group by severity;`,
	).Rows(); err != nil {
		return nil, utils.Errorf("fetch risk type error: %s", err)
	} else {
		var severityRaw interface{} = ""
		var total int32
		for rows.Next() {
			err := rows.Scan(&severityRaw, &total)
			if err != nil {
				log.Errorf("scan severity level failed: %s", err)
				continue
			}
			var severityStr string
			switch ret := severityRaw.(type) {
			case string:
				severityStr = ret
			case []byte:
				severityStr = string(ret)
			default:
				severityStr = ""
			}
			r, ok := severities[severityStr]
			if !ok {
				r = &ypb.FieldName{
					Name:    severityStr,
					Verbose: severityVerbose(severityStr),
					Total:   total,
				}
				severities[severityStr] = r
				continue
			}
			r.Total += total
		}
	}
	return &ypb.Fields{Values: funk.Values(severities).([]*ypb.FieldName)}, nil
}

var (
	riskStatsLock   = new(sync.Mutex)
	OriginRiskStats *ypb.RiskTableStats
)

func compareRiskType(stat *ypb.RiskTableStats, wait *ypb.RiskTableStats) *ypb.RiskTableStats {
	if wait == nil {
		return stat
	}
	if stat.RiskTypeStats != nil && wait.RiskTypeStats != nil {
		for _, value := range stat.RiskTypeStats.Values {
			for _, compare := range wait.RiskTypeStats.Values {
				if compare.Name == value.Name {
					value.Delta = value.Total - compare.Total
				}
			}
		}
	}

	if stat.RiskLevelStats != nil && wait.RiskLevelStats != nil {
		for _, value := range stat.RiskLevelStats.Values {
			for _, compare := range wait.RiskLevelStats.Values {
				if compare.Name == value.Name {
					value.Delta = value.Total - compare.Total
				}
			}
		}
	}
	return stat
}

func (s *Server) QueryRiskTableStats(ctx context.Context, e *ypb.Empty) (*ypb.RiskTableStats, error) {
	riskStatsLock.Lock()
	defer riskStatsLock.Unlock()

	var latestVul yakit.Risk
	if db := s.GetProjectDatabase().Model(&yakit.Risk{}).Where("").Order("updated_at desc").First(&latestVul); db.Error != nil {
		return nil, utils.Errorf("fetch newest vul failed: %s", db.Error)
	}

	currentStats := &ypb.RiskTableStats{}
	if latestVul.UpdatedAt.Unix() > 0 {
		currentStats.LatestCreatedAtTimestamp = latestVul.UpdatedAt.Unix()
	}
	currentStats.RiskLevelStats, _ = s.QueryAvailableRiskLevel(ctx, e)
	currentStats.RiskTypeStats, _ = s.QueryAvailableRiskType(ctx, e)
	if OriginRiskStats == nil {
		OriginRiskStats = currentStats
		return currentStats, nil
	}

	d := compareRiskType(currentStats, OriginRiskStats)
	return d, nil
}

func (s *Server) ResetRiskTableStats(ctx context.Context, e *ypb.Empty) (*ypb.Empty, error) {
	riskStatsLock.Lock()
	defer riskStatsLock.Unlock()
	OriginRiskStats = nil
	return e, nil
}

func (s *Server) DeleteHistoryHTTPFuzzerTask(ctx context.Context, d *ypb.DeleteHistoryHTTPFuzzerTaskRequest) (*ypb.Empty, error) {
	if d.GetId() > 0 {
		err := yakit.DeleteWebFuzzerTask(s.GetProjectDatabase(), int64(d.GetId()))
		if err != nil {
			return nil, err
		}
	}
	err := yakit.DeleteWebFuzzerTaskAll(s.GetProjectDatabase())
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) QueryReports(ctx context.Context, d *ypb.QueryReportsRequest) (*ypb.QueryReportsResponse, error) {
	p, res, err := yakit.QueryReportRecord(s.GetProjectDatabase(), d)
	if err != nil {
		return nil, err
	}

	result := funk.Map(res, func(i *yakit.ReportRecord) *ypb.Report {
		return i.ToGRPCModel()
	}).([]*ypb.Report)
	return &ypb.QueryReportsResponse{
		Data:       result,
		Total:      int64(p.TotalRecord),
		Pagination: d.Pagination,
	}, nil
}

func (s *Server) QueryReport(ctx context.Context, d *ypb.QueryReportRequest) (*ypb.Report, error) {
	r, err := yakit.GetReportRecord(s.GetProjectDatabase(), d.GetId())
	if err != nil {
		f, err := yakit.GetReportRecordByHash(s.GetProjectDatabase(), d.GetHash())
		if err != nil {
			return nil, err
		}
		return f.ToGRPCModel(), err
	}
	return r.ToGRPCModel(), nil
}

func (s *Server) DeleteReport(ctx context.Context, d *ypb.DeleteReportRequest) (*ypb.Empty, error) {
	_ = yakit.DeleteReportRecordByID(s.GetProjectDatabase(), d.GetId())
	_ = yakit.DeleteReportRecordByHash(s.GetProjectDatabase(), d.GetHash())
	if d.GetDeleteAll() {
		if db := s.GetProjectDatabase().Model(&yakit.ReportRecord{}).Where("true").Unscoped().Delete(&yakit.ReportRecord{}); db.Error != nil {
			return nil, db.Error
		}
	}
	if d.GetFilter() != nil {
		if db := yakit.FilterReportRecord(s.GetProjectDatabase().Model(&yakit.ReportRecord{}), d.GetFilter()).Unscoped().Delete(&yakit.ReportRecord{}); db.Error != nil {
			log.Errorf("error: %s", db.Error)
		}
	}
	if len(d.GetIDs()) > 0 {
		for _, i := range d.GetIDs() {
			yakit.DeleteReportRecordByID(s.GetProjectDatabase(), i)
		}
	}
	return &ypb.Empty{}, nil
}

func (s *Server) QueryAvailableReportFrom(ctx context.Context, _ *ypb.Empty) (*ypb.Fields, error) {
	return nil, nil
}

func (s *Server) QueryNewRisk(ctx context.Context, req *ypb.QueryNewRiskRequest) (*ypb.QueryNewRiskResponse, error) {
	_, data, err := yakit.QueryNewRisk(s.GetProjectDatabase(), req, true, true)
	if err != nil {
		return nil, err
	}
	p, _, _ := yakit.QueryNewRisk(s.GetProjectDatabase(), req, true, false)

	paging, _, _ := yakit.QueryNewRisk(s.GetProjectDatabase(), req, false, true)

	rsp := &ypb.QueryNewRiskResponse{
		Total:        int64(paging.TotalRecord),
		NewRiskTotal: int64(p.TotalRecord),
		Data:         nil,
	}
	for _, r := range data {
		rsp.Data = append(rsp.Data, NewRiskGRPCModel(r))
	}

	return rsp, nil
}

func NewRiskGRPCModel(p *yakit.Risk) *ypb.NewRisk {
	return &ypb.NewRisk{
		Title:        p.Title,
		TitleVerbose: p.TitleVerbose,
		Id:           int64(p.ID),
		CreatedAt:    p.CreatedAt.Unix(),
		UpdatedAt:    p.UpdatedAt.Unix(),
		Verbose:      severityVerbose(p.Severity),
		IsRead:       p.IsRead,
	}
}

func (s *Server) NewRiskRead(ctx context.Context, req *ypb.NewRiskReadRequest) (*ypb.Empty, error) {
	if len(req.GetIds()) > 0 {
		for _, v := range funk.ChunkInt64s(req.Ids, 100) {
			err := yakit.NewRiskReadRequest(s.GetProjectDatabase(), req, v)
			log.Error(err)
		}
	} else {
		err := yakit.NewRiskReadRequest(s.GetProjectDatabase(), req, req.Ids)
		if err != nil {
			return nil, utils.Errorf("NewRiskRead error: %v", err)
		}
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DownloadReport(ctx context.Context, req *ypb.DownloadReportRequest) (*ypb.Empty, error) {
	if req.FileData == "" || req.FileDir == "" || req.FileName == "" {
		return nil, utils.Errorf("params empty")
	}
	dataPath := filepath.Join(req.FileDir, req.FileName)
	os.RemoveAll(dataPath)
	err := ioutil.WriteFile(dataPath, []byte(req.FileData), 0666)
	if err != nil {
		return nil, utils.Errorf("write script failed: %s", err)
	}
	return &ypb.Empty{}, nil
}

func (s *Server) UploadRiskToOnline(ctx context.Context, req *ypb.UploadRiskToOnlineRequest) (*ypb.Empty, error) {
	if req.Token == "" {
		return nil, utils.Errorf("params empty")
	}
	var hash []string
	db := s.GetProjectDatabase()
	db = db.Where("upload_online <> '1' or upload_online IS NULL")
	data := yakit.YieldRisks(db, context.Background())
	for k := range data {
		client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
		err := client.UploadRiskToOnlineWithToken(ctx, req.Token, k)
		if err != nil {
			log.Errorf("uploadRiskToOnline failed: %s", err)
		} else {
			hash = append(hash, k.Hash)
		}
	}
	for _, v := range funk.ChunkStrings(hash, 100) {
		err := yakit.UploadRiskToOnline(s.GetProjectDatabase(), v)
		if err != nil {
			log.Errorf("uploadRiskToOnline failed: %s", err)
		}
	}

	return &ypb.Empty{}, nil
}
