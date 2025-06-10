package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/webforest"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var saveYakitLogLock = new(sync.Mutex)

func SaveFromYakitLog(l *yaklib.YakitLog, db *gorm.DB) {
	saveYakitLogLock.Lock()
	defer saveYakitLogLock.Unlock()

	switch l.Level {
	case "asset-port":
		var port schema.Port
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
	var results []*ypb.Port
	if req.GetAll() {
		db := yakit.FilterPort(s.GetProjectDatabase(), req)
		count := bizhelper.QueryCount(db, &schema.Port{}, nil)
		db = bizhelper.QueryOrder(db, req.GetOrderBy(), req.GetOrder())
		data := yakit.YieldPorts(db, context.Background())
		for r := range data {
			results = append(results, ToGrpcPort(r))
		}
		return &ypb.QueryPortsResponse{
			Pagination: req.Pagination,
			Total:      int64(count),
			Data:       results,
		}, nil
	} else { // 全部
		p, res, err := yakit.QueryPorts(s.GetProjectDatabase(), req) // Query ports by pagination and query total count
		if err != nil {
			return nil, err
		}
		for _, r := range res {
			results = append(results, ToGrpcPort(r))
		}
		return &ypb.QueryPortsResponse{
			Pagination: req.Pagination,
			Total:      int64(p.TotalRecord),
			Data:       results,
		}, nil
	}
}

func ToGrpcPort(r *schema.Port) *ypb.Port {
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
		db.Unscoped().Where("true").Delete(&schema.Port{})
		return &ypb.Empty{}, nil
	}
	if req.GetDeleteAll() {
		db.Unscoped().Where("true").Delete(&schema.Port{})
		return &ypb.Empty{}, nil
	}
	if req.GetFilter() != nil {
		if db := yakit.FilterPort(s.GetProjectDatabase(), req.GetFilter()).Unscoped().Delete(&schema.Port{}); db.Error != nil {
			return nil, utils.Errorf("delete error: %s", db.Error)
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

	db.Unscoped().Delete(&schema.Port{})

	return &ypb.Empty{}, nil
}

func (s *Server) DeleteHosts(ctx context.Context, req *ypb.DeleteHostsRequest) (*ypb.Empty, error) {
	if req.DeleteAll {
		s.GetProjectDatabase().Model(&schema.Host{}).Unscoped().Delete(&schema.Host{})
		return &ypb.Empty{}, nil
	}

	var db = s.GetProjectDatabase().Model(&schema.Host{}).Unscoped()
	if req.DomainKeyword != "" {
		bizhelper.FuzzQueryLike(db, "domain", req.DomainKeyword).Delete(&schema.Host{})
		return &ypb.Empty{}, nil
	}

	if req.Network != "" {
		bizhelper.QueryBySpecificAddress(db, "ip_integer", req.Network).Delete(&schema.Host{})
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
		s.GetProjectDatabase().Model(&schema.Domain{}).Unscoped().Delete(&schema.Domain{})
		return &ypb.Empty{}, nil
	}
	if req.GetFilter() != nil {
		db := yakit.FilterDomain(s.GetProjectDatabase(), req.GetFilter())
		if db := db.Unscoped().Delete(&schema.Domain{}); db.Error != nil {
			return &ypb.Empty{}, nil
		}
	}

	if len(req.GetIDs()) > 0 {
		for _, i := range req.GetIDs() {
			yakit.DeleteDomainByID(s.GetProjectDatabase(), i)
		}
		return &ypb.Empty{}, nil
	}

	var db = s.GetProjectDatabase().Model(&schema.Domain{}).Unscoped()
	if req.DomainKeyword != "" {
		bizhelper.FuzzQueryLike(db, "domain", req.DomainKeyword).Delete(&schema.Domain{})
		return &ypb.Empty{}, nil
	}

	if req.Network != "" {
		bizhelper.QueryBySpecificAddress(db, "ip_integer", req.Network).Delete(&schema.Domain{})
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
			rdb := yakit.FilterByQueryRisks(s.GetProjectDatabase(), req.GetFilter())
			if rdb != nil {
				if db := rdb.Unscoped().Where("id <> ?", req.GetId()).Delete(&schema.Risk{}); db.Error != nil {
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
		rdb := yakit.FilterByQueryRisks(s.GetProjectDatabase(), req.GetFilter())
		if rdb != nil {
			if db := rdb.Unscoped().Delete(&schema.Risk{}); db.Error != nil {
				return nil, utils.Errorf("delete error: %s", db.Error)
			}
		}
	}

	if len(req.GetIds()) > 0 {
		_ = yakit.DeleteRiskByID(s.GetProjectDatabase(), req.GetIds()...)
		return &ypb.Empty{}, nil
	}

	if req.GetDeleteAll() {
		if db := s.GetProjectDatabase().Model(&schema.Risk{}).Where("true").Unscoped().Delete(&schema.Risk{}); db.Error != nil {
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
	riskTypes, err := AvailableRiskType(s.GetProjectDatabase())
	if err != nil {
		return nil, utils.Errorf("query risk types failed: %s", err)
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
		return "信息"
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
	severities, err := AvailableRiskLevel(s.GetProjectDatabase())
	if err != nil {
		return nil, utils.Errorf("fetch risk type error: %s", err)
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

	var latestVul schema.Risk
	if db := s.GetProjectDatabase().Model(&schema.Risk{}).Where("").Order("updated_at desc").First(&latestVul); db.Error != nil {
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
	// 优先 id -> webfuzzerIndex -> 全部

	if d.GetId() > 0 {
		err := yakit.DeleteWebFuzzerTask(s.GetProjectDatabase(), int64(d.GetId()))
		if err != nil {
			return nil, err
		}
		return &ypb.Empty{}, nil
	}
	if d.GetWebFuzzerIndex() != "" {
		err := yakit.DeleteWebFuzzerTaskByWebFuzzerIndex(s.GetProjectDatabase(), d.GetWebFuzzerIndex())
		if err != nil {
			return nil, err
		}
		return &ypb.Empty{}, nil
	}
	err := yakit.DeleteWebFuzzerTaskAll(s.GetProjectDatabase())
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) GetProjectDBWithType(typ string) *gorm.DB {
	if typ == yakit.TypeSSAProject {
		return s.GetSSADatabase()
	}
	return s.GetProjectDatabase()
}

func (s *Server) QueryReports(ctx context.Context, d *ypb.QueryReportsRequest) (*ypb.QueryReportsResponse, error) {
	// projectDB :=
	projectDB := s.GetProjectDBWithType(d.Type)
	p, res, err := yakit.QueryReportRecord(projectDB, d)
	if err != nil {
		return nil, err
	}

	result := funk.Map(res, func(i *schema.ReportRecord) *ypb.Report {
		return i.ToGRPCModel()
	}).([]*ypb.Report)
	return &ypb.QueryReportsResponse{
		Data:       result,
		Total:      int64(p.TotalRecord),
		Pagination: d.Pagination,
	}, nil
}

func (s *Server) QueryReport(ctx context.Context, d *ypb.QueryReportRequest) (*ypb.Report, error) {
	projectDB := s.GetProjectDBWithType(d.Type)
	r, err := yakit.GetReportRecord(projectDB, d.GetId())
	if err != nil {
		f, err := yakit.GetReportRecordByHash(projectDB, d.GetHash())
		if err != nil {
			return nil, err
		}
		return f.ToGRPCModel(), err
	}
	return r.ToGRPCModel(), nil
}

func (s *Server) DeleteReport(ctx context.Context, d *ypb.DeleteReportRequest) (*ypb.Empty, error) {
	projectDB := s.GetProjectDBWithType(d.Type)
	_ = yakit.DeleteReportRecordByID(projectDB, d.GetId())
	_ = yakit.DeleteReportRecordByHash(projectDB, d.GetHash())
	if d.GetDeleteAll() {
		if db := projectDB.Model(&schema.ReportRecord{}).Where("true").Unscoped().Delete(&schema.ReportRecord{}); db.Error != nil {
			return nil, db.Error
		}
	}
	if d.GetFilter() != nil {
		if db := yakit.FilterReportRecord(projectDB.Model(&schema.ReportRecord{}), d.GetFilter()).Unscoped().Delete(&schema.ReportRecord{}); db.Error != nil {
			log.Errorf("error: %s", db.Error)
		}
	}
	if len(d.GetIDs()) > 0 {
		for _, i := range d.GetIDs() {
			yakit.DeleteReportRecordByID(projectDB, i)
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

	count, _ := yakit.QueryRiskCount(s.GetProjectDatabase(), "")
	unreadCount, _ := yakit.QueryRiskCount(s.GetProjectDatabase(), "false")

	rsp := &ypb.QueryNewRiskResponse{
		Total:        count,
		NewRiskTotal: int64(p.TotalRecord),
		Data:         nil,
		Unread:       unreadCount,
	}
	for _, r := range data {
		rsp.Data = append(rsp.Data, NewRiskGRPCModel(r))
	}

	return rsp, nil
}

func NewRiskGRPCModel(p *schema.Risk) *ypb.NewRisk {
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
	var filter *ypb.QueryRisksRequest
	if req.GetFilter() != nil {
		filter = req.GetFilter() // use filter, good
	} else if len(req.GetIds()) > 0 {
		filter = &ypb.QueryRisksRequest{ // just use id, this is older frontend
			Ids: req.GetIds(),
		}
	} else {
		filter = nil // nil is all risk mark as read
	}
	err := yakit.NewRiskReadRequest(s.GetProjectDatabase(), filter)
	if err != nil {
		return nil, err
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
		content, err := json.Marshal(k)
		if err != nil {
			continue
		}
		raw, err := json.Marshal(yaklib.QueryUploadRiskOnlineRequest{
			req.ProjectName,
			content,
			req.ExternalProjectCode,
			req.ExternalModule,
		})
		if err != nil {
			continue
		}
		client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
		err = client.UploadToOnline(ctx, req.Token, raw, "api/risk/upload")
		if err != nil {
			log.Errorf("uploadRiskToOnline failed: %s", err)
			return &ypb.Empty{}, nil
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

func (s *Server) QueryPortsGroup(ctx context.Context, req *ypb.Empty) (*ypb.QueryPortsGroupResponse, error) {
	data, err := yakit.PortsServiceTypeGroup()
	var tagsCode ypb.QueryPortsGroupResponse
	if data == nil {
		return nil, err
	}
	tagsCode = PortsServiceTypeGroup(data)
	return &tagsCode, nil
}

func PortsServiceTypeGroup(data []*yakit.PortsTypeGroup) ypb.QueryPortsGroupResponse {
	var (
		portGroup                       ypb.QueryPortsGroupResponse
		databaseGroupList, webGroupList ypb.PortsGroup
	)
	serviceTypeKey := map[string]string{
		"Nginx":                   "nginx",
		"Apache":                  "apache",
		"IIS":                     "iis",
		"Litespeed":               "litespeed",
		"Tomcat":                  "tomcat",
		"OracleHTTPServer":        "oracle_http_server",
		"Openresty":               "openresty",
		"Jetty":                   "jetty",
		"Caddy":                   "caddy",
		"Gunicorn":                "gunicorn",
		"Cowboy":                  "cowboy",
		"Lighttpd":                "lighttpd",
		"Resin":                   "resin",
		"Zeus":                    "zeus",
		"Cherrypy":                "cherrypy",
		"Tengine":                 "tengine",
		"Glassfish":               "glassfish",
		"PhusionPassenger":        "phusion_passenger",
		"Tornadoserver":           "tornadoserver",
		"Hiawatha":                "hiawatha",
		"OracleApplicationServer": "oracle_application_serve",
		"AbyssWebServer":          "abyss_web_server",
		"Boa":                     "boa",
		"Xitami":                  "xitami",
		"Simplehttp":              "simplehttp",
		"Cherokee":                "cherokee",
		"MonkeyHTTPServer":        "monkey_http_server",
		"NodeJS":                  "node.js",
		"Websphere":               "websphere",
		"Zope":                    "zope",
		"Mongoose":                "mongoose",
		"Macos":                   "macos",
		"Kestrel":                 "kestrel",
		"Aolserver":               "aolserver",
		"Dnsmasq":                 "dnsmasq",
		"Ruby":                    "ruby",
		"Webrick":                 "webrick",
		"WeblogicServer":          "weblogic_server",
		"Jboss":                   "jboss",
		"SqlServer":               "sql_server",
		"Mysql":                   "mysql",
		"Mongodb":                 "mongodb",
		"Redis":                   "redis",
		"Elasticsearch":           "elasticsearch",
		"Postgresql":              "postgresql",
		"DB2":                     "db2",
		"Hbase":                   "hbase",
		"Memcached":               "memcached",
		"Splunkd":                 "splunkd",
	}
	databaseValues := []string{"sql_server", "mysql", "mongodb", "redis", "elasticsearch", "postgresql", "db2", "hbase", "memcached", "splunkd"}
	for k, v := range serviceTypeKey {
		if reflect.ValueOf(data[0]).Elem().FieldByName(k).Interface().(int32) > 0 {
			if IsValueInSortedSlice(v, databaseValues) {
				databaseGroupList.GroupName = "数据库"
				databaseGroupList.GroupLists = append(databaseGroupList.GroupLists, &ypb.GroupList{
					ServiceType:     v,
					ShowServiceType: k,
					Total:           reflect.ValueOf(data[0]).Elem().FieldByName(k).Interface().(int32),
				})
			} else {
				webGroupList.GroupName = "服务器"
				webGroupList.GroupLists = append(webGroupList.GroupLists, &ypb.GroupList{
					ServiceType:     v,
					ShowServiceType: k,
					Total:           reflect.ValueOf(data[0]).Elem().FieldByName(k).Interface().(int32),
				})
			}
		}
	}
	if len(databaseGroupList.GroupLists) > 0 {
		portGroup.PortsGroupList = append(portGroup.PortsGroupList, &databaseGroupList)
	}
	if len(webGroupList.GroupLists) > 0 {
		portGroup.PortsGroupList = append(portGroup.PortsGroupList, &webGroupList)
	}
	return portGroup
}

func IsValueInSortedSlice(value string, slice []string) bool {
	for _, v := range slice {
		if strings.Contains(v, value) {
			return true
		}
	}
	return false
}

func (s *Server) SetTagForRisk(ctx context.Context, req *ypb.SetTagForRiskRequest) (*ypb.Empty, error) {
	if len(req.GetTags()) > 0 {
		risk, err := yakit.GetRiskByIDOrHash(s.GetProjectDatabase(), req.Id, req.Hash)
		if err != nil {
			return nil, err
		}
		extLen := len(req.GetTags())
		tagsData := make([]string, extLen)
		if extLen > 0 {
			for i := 0; i < extLen; i++ {
				tagsData[i] = req.Tags[i]
			}
		}
		risk.Tags = strings.Join(utils.RemoveRepeatStringSlice(tagsData), "|")
		err = yakit.UpdateRiskTags(s.GetProjectDatabase(), risk)
		if err != nil {
			return nil, err
		}
		return &ypb.Empty{}, nil
	}
	return &ypb.Empty{}, nil
}

func (s *Server) QueryRiskTags(ctx context.Context, req *ypb.Empty) (*ypb.QueryRiskTagsResponse, error) {
	var riskTags []*ypb.FieldGroup
	db := s.GetProjectDatabase().Where("tags IS NOT NULL")
	data := yakit.YieldRisks(db, context.Background())
	tagCounts := make(map[string]int)
	for k := range data {
		for _, tag := range strings.Split(k.Tags, "|") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tagCounts[tag]++
			}
		}
	}
	for k, v := range tagCounts {
		riskTags = append(riskTags, &ypb.FieldGroup{
			Name:  k,
			Total: int32(v),
		})

	}
	return &ypb.QueryRiskTagsResponse{RiskTags: riskTags}, nil
}

func (s *Server) RiskFieldGroup(ctx context.Context, req *ypb.Empty) (*ypb.RiskFieldGroupResponse, error) {
	riskLevel, err := AvailableRiskLevel(s.GetProjectDatabase())
	if err != nil {
		log.Errorf("risk level group filed %s", err.Error())
	}

	var riskTypeGroup, riskLevelGroup []*ypb.FieldName
	for _, v := range riskLevel {
		riskLevelGroup = append(riskLevelGroup, &ypb.FieldName{
			Name:    v.Name,
			Verbose: v.Verbose,
			Total:   v.Total,
		})
	}

	riskType, err := AvailableRiskType(s.GetProjectDatabase())
	if err != nil {
		log.Errorf("risk level group filed %s", err.Error())
	}

	typeSlice := make([]*ypb.FieldName, 0, len(riskType))
	for _, value := range riskType {
		typeSlice = append(typeSlice, value)
	}

	sort.Slice(typeSlice, func(i, j int) bool {
		return typeSlice[i].Total > typeSlice[j].Total
	})

	for _, v := range typeSlice {
		if len(riskTypeGroup) < 10 {
			riskTypeGroup = append(riskTypeGroup, &ypb.FieldName{
				Name:    v.Name,
				Verbose: v.Verbose,
				Total:   v.Total,
			})
		}
	}

	riskIP, err := AvailableRiskIP(s.GetProjectDatabase())
	if err != nil {
		log.Errorf("risk ip group filed %s", err.Error())
	}
	return &ypb.RiskFieldGroupResponse{
		RiskIPGroup:    riskIP,
		RiskLevelGroup: riskLevelGroup,
		RiskTypeGroup:  riskTypeGroup,
	}, nil
}

func AvailableRiskLevel(db *gorm.DB) (map[string]*ypb.FieldName, error) {
	fixRiskOnce.Do(func() {
		yakit.FixRiskType(db)
	})
	var severities = make(map[string]*ypb.FieldName)
	if rows, err := db.Raw(
		`select distinct severity, count(*) as total from risks where waiting_verified = false group by severity;`,
	).Rows(); err != nil {
		return nil, utils.Errorf("fetch risk level error: %s", err)
	} else {
		var severityRaw interface{} = ""
		var total int32
		for rows.Next() {
			err = rows.Scan(&severityRaw, &total)
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
	return severities, nil
}

func AvailableRiskType(db *gorm.DB) (map[string]*ypb.FieldName, error) {
	var types []*riskType
	if rows, err := db.Table("risks").Raw(
		`select distinct risk_type_verbose, risk_type, count(*) as total from risks  where waiting_verified = false group by risk_type_verbose;`,
	).Rows(); err != nil {
		return nil, utils.Errorf("query risk types failed: %s", err)
	} else {
		for rows.Next() {
			var verbose string
			var typeStr string
			var total int32
			err = rows.Scan(&verbose, &typeStr, &total)
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

	return riskTypes, nil
}

func AvailableRiskIP(db *gorm.DB) ([]*ypb.FieldGroup, error) {
	var riskIP []*ypb.FieldGroup
	if rows, err := db.Table("risks").Raw(
		`SELECT	((ip_segment >> 24) & 255) || '.' || ((ip_segment >> 16) & 255) || '.' || ((ip_segment >> 8) & 255) || '.1/24' AS ip_segment, total
	FROM (
		SELECT(ip_integer & 0xFFFFFF00) AS ip_segment, COUNT(*) AS total FROM risks	WHERE waiting_verified = false	GROUP BY ip_segment) AS grouped_segments ORDER BY total DESC `).Rows(); err != nil {
		return nil, utils.Errorf("query risk IP failed: %s", err)
	} else {
		for rows.Next() {
			var (
				value string
				total int32
			)
			err = rows.Scan(&value, &total)
			if err != nil {
				log.Errorf("scan risk ip failed: %s", err)
				continue
			}
			riskIP = append(riskIP, &ypb.FieldGroup{
				Name:  value,
				Total: total,
			})
		}
	}

	return riskIP, nil
}

func (s *Server) RiskFeedbackToOnline(ctx context.Context, req *ypb.UploadRiskToOnlineRequest) (*ypb.Empty, error) {
	if req.Token == "" || req.Hash == nil {
		return nil, utils.Errorf("params empty")
	}
	db := s.GetProjectDatabase()
	db = bizhelper.ExactQueryStringArrayOr(db, "hash", req.Hash)
	data := yakit.YieldRisks(db, context.Background())
	for k := range data {
		content, err := json.Marshal(k)
		if err != nil {
			continue
		}
		raw, err := json.Marshal(yaklib.UploadOnlineRequest{
			content,
		})
		if err != nil {
			continue
		}
		client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
		err = client.UploadToOnline(ctx, req.Token, raw, "api/risk/feed/back")
		if err != nil {
			log.Errorf("uploadRiskToOnline failed: %s", err)
			return &ypb.Empty{}, nil
		}
	}

	return &ypb.Empty{}, nil
}
