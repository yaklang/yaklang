package yakit

import (
	"context"
	"strconv"
	"strings"

	"github.com/jinzhu/gorm"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type Risk struct {
	gorm.Model

	Hash string `json:"hash"`

	// essential
	IP        string `json:"ip"`
	IPInteger int64  `json:"ip_integer"`

	// extraTargets
	Url  string `json:"url"`
	Port int    `json:"port"`
	Host string `json:"host"`

	//
	Title           string `json:"title"`
	TitleVerbose    string `json:"title_verbose"`
	Description     string `json:"description"`
	Solution        string `json:"solution"`
	RiskType        string `json:"risk_type"`
	RiskTypeVerbose string `json:"risk_verbose"`
	Parameter       string `json:"parameter"`
	Payload         string `json:"payload"`
	Details         string `json:"details"`
	Severity        string `json:"severity"`

	// 来源于哪个插件？
	FromYakScript string `json:"from_yak_script"`

	// 等待验证中？
	WaitingVerified bool `json:"waiting_verified"`
	// 用于验证的 ReverseToken
	ReverseToken string `json:"reverse_token"`

	// 设置运行时 ID 为了关联具体漏洞
	RuntimeId      string `json:"runtime_id"`
	QuotedRequest  string `json:"quoted_request"`
	QuotedResponse string `json:"quoted_response"`

	// 潜在威胁：用于输出合规性质的漏洞内容
	IsPotential bool `json:"is_potential"`

	CVE                 string `json:"cve"`
	IsRead              bool   `json:"is_read"`
	Ignore              bool   `json:"ignore"`
	UploadOnline        bool   `json:"upload_online"`
	TaskName            string `json:"task_name"`
	CveAccessVector     string `json:"cve_access_vector"`
	CveAccessComplexity string `json:"cve_access_complexity"`
}

func (p *Risk) ToGRPCModel() *ypb.Risk {
	details, _ := strconv.Unquote(p.Details)
	if details == "" {
		details = p.Details
	}

	var request []byte
	var response []byte

	reqRaw, _ := strconv.Unquote(p.QuotedRequest)
	if reqRaw != "" {
		request = []byte(reqRaw)
	} else {
		request = []byte(p.QuotedRequest)
	}

	rspRaw, _ := strconv.Unquote(p.QuotedResponse)
	if rspRaw != "" {
		response = []byte(rspRaw)
	} else {
		response = []byte(p.QuotedResponse)
	}

	return &ypb.Risk{
		Hash:            utils.EscapeInvalidUTF8Byte([]byte(p.Hash)),
		IP:              utils.EscapeInvalidUTF8Byte([]byte(p.IP)),
		Url:             utils.EscapeInvalidUTF8Byte([]byte(p.Url)),
		Port:            int32(p.Port),
		Host:            utils.EscapeInvalidUTF8Byte([]byte(p.Host)),
		Title:           utils.EscapeInvalidUTF8Byte([]byte(p.Title)),
		TitleVerbose:    utils.EscapeInvalidUTF8Byte([]byte(p.TitleVerbose)),
		Description:     utils.EscapeInvalidUTF8Byte([]byte(p.Description)),
		Solution:        utils.EscapeInvalidUTF8Byte([]byte(p.Solution)),
		RiskType:        utils.EscapeInvalidUTF8Byte([]byte(p.RiskType)),
		RiskTypeVerbose: utils.EscapeInvalidUTF8Byte([]byte(p.RiskTypeVerbose)),
		Parameter:       utils.EscapeInvalidUTF8Byte([]byte(p.Parameter)),
		Payload:         utils.EscapeInvalidUTF8Byte([]byte(p.Payload)),
		Details:         details,
		FromYakScript:   utils.EscapeInvalidUTF8Byte([]byte(p.FromYakScript)),
		WaitingVerified: p.WaitingVerified,
		ReverseToken:    utils.EscapeInvalidUTF8Byte([]byte(p.ReverseToken)),
		Id:              int64(p.ID),
		CreatedAt:       p.CreatedAt.Unix(),
		UpdatedAt:       p.UpdatedAt.Unix(),
		Severity:        utils.EscapeInvalidUTF8Byte([]byte(p.Severity)),

		Request:  request,
		Response: response,

		RuntimeId: utils.EscapeInvalidUTF8Byte([]byte(p.RuntimeId)),
		CVE:       utils.EscapeInvalidUTF8Byte([]byte(p.CVE)),
		TaskName:  utils.EscapeInvalidUTF8Byte([]byte(p.TaskName)),
	}
}

func (p *Risk) BeforeSave() error {
	if p.Hash == "" {
		p.Hash = uuid.NewV4().String()
	}

	p.RiskType = strings.ReplaceAll(p.RiskType, "|", "_")
	p.Severity = strings.ReplaceAll(p.Severity, "|", "_")

	if p.IPInteger <= 0 && p.IP != "" {
		p.IPInteger, _ = utils.IPv4ToUint64(p.IP)
	}

	if p.Severity == "" {
		p.Severity = "info"
	}

	if p.RiskType == "" {
		p.RiskType = "info"
	}

	if p.RiskTypeVerbose == "" {
		p.RiskTypeVerbose = "信息"
	}

	return nil
}

func CreateOrUpdateRisk(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&Risk{})

	var token string
	switch ret := i.(type) {
	case *Risk:
		token = ret.ReverseToken
		if ret.FromYakScript == "" {
			ret.FromYakScript = consts.GetCurrentYakitPluginID()
		}
	case Risk:
		token = ret.ReverseToken
		if ret.FromYakScript == "" {
			ret.FromYakScript = consts.GetCurrentYakitPluginID()
		}
	case map[string]interface{}:
		_, ok := ret["from_yak_script"]
		if !ok {
			ret["from_yak_script"] = consts.GetCurrentYakitPluginID()
		}
		token = utils.MapGetString(ret, "reverse_token")
		if token == "" {
			token = utils.MapGetString(ret, "ReverseToken")
		}
	}

	if token != "" {
		if db := db.Model(&Risk{}).Where(
			"reverse_token LIKE ?", "%"+token+"%",
		).Update(map[string]interface{}{
			"waiting_verified": false,
		}); db.Error != nil {
			log.Errorf("reverse_token[%v] found cannot trigger unfinished risk.", token)
		}
	}

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&Risk{}); db.Error != nil {
		return utils.Errorf("create/update Risk failed: %s", db.Error)
	}

	return nil
}

func GetRisk(db *gorm.DB, id int64) (*Risk, error) {
	var r Risk
	if db := db.Model(&Risk{}).Where("id = ?", id).First(&r); db.Error != nil {
		return nil, utils.Errorf("get Risk failed: %s", db.Error)
	}
	return &r, nil
}

func GetRiskByHash(db *gorm.DB, hash string) (*Risk, error) {
	var r Risk
	if db := db.Model(&Risk{}).Where("hash = ?", hash).First(&r); db.Error != nil {
		return nil, utils.Errorf("get Risk failed: %s", db.Error)
	}
	return &r, nil
}

func DeleteRiskByID(db *gorm.DB, ids ...int64) error {
	if len(ids) == 1 {
		id := ids[0]
		if db := db.Model(&Risk{}).Where(
			"id = ?", id,
		).Unscoped().Delete(&Risk{}); db.Error != nil {
			return db.Error
		}
		return nil
	}

	if db = bizhelper.ExactQueryInt64ArrayOr(db, "id", ids).Unscoped().Delete(&Risk{}); db.Error != nil {
		return utils.Errorf("delete id(s) failed: %v", db.Error)
	}

	return nil
}

func FixRiskType(db *gorm.DB) {
	db.Model(&Risk{}).Where("(severity = ?) OR (severity is null)", "").Updates(map[string]interface{}{
		"severity": "default",
	})
	db.Model(&Risk{}).Where("(risk_type = ?) OR (risk_type is null)", "").Updates(map[string]interface{}{
		"risk_type": "default",
	})

	// 修复 nuclei 漏洞保存格式
}

func FilterByQueryRisks(db *gorm.DB, params *ypb.QueryRisksRequest) (_ *gorm.DB, _ error) {
	db = db.Model(&Risk{})
	db = db.Where("waiting_verified = ?", params.GetWaitingVerified())
	db = bizhelper.QueryBySpecificPorts(db, "port", params.GetPorts())
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", params.GetNetwork())
	db = bizhelper.FuzzSearchEx(db, []string{
		"ip", "url",
		"title", "title_verbose", "risk_type", "risk_type_verbose",
		"parameter", "payload", "details",
	}, params.GetSearch(), false)
	// 搜索风险类型
	db = bizhelper.FuzzQueryStringArrayOrPrefixLike(
		db, "risk_type",
		utils.PrettifyListFromStringSplitEx(params.GetRiskType()),
	)
	db = bizhelper.FuzzQueryStringArrayOrPrefixLike(
		db, "severity",
		utils.PrettifyListFromStringSplitEx(params.GetSeverity()),
	)
	db = bizhelper.ExactQueryString(db, "token", params.GetToken())
	return db, nil
}

func QueryRisks(db *gorm.DB, params *ypb.QueryRisksRequest) (*bizhelper.Paginator, []*Risk, error) {
	if params == nil {
		return nil, nil, utils.Errorf("empty params")
	}
	db = db.Model(&Risk{}) // .Debug()
	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}

	p := params.Pagination
	db = bizhelper.QueryOrder(db, p.OrderBy, p.Order)

	var err error
	db, err = FilterByQueryRisks(db, params)
	if err != nil {
		return nil, nil, err
	}
	//db = bizhelper.FuzzQueryLike(db, "service_type", params.GetService())
	//db = bizhelper.FuzzQueryLike(db, "html_title", params.GetTitle())
	//
	//if params.GetState() == "" {
	//	db = bizhelper.ExactQueryString(db, "state", "open")
	//} else {
	//	db = bizhelper.ExactQueryString(db, "state", params.GetState())
	//}

	var ret []*Risk
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func DeleteRiskByTarget(db *gorm.DB, target string) {
	db = db.Model(&Risk{})
	var host, port, _ = utils.ParseStringToHostPort(target)
	if port > 0 {
		db = db.Where("port = ?", port)
		if host != "" {
			db = db.Where("(host = ?) OR (ip = ?)", host, host)
		}
	} else {
		db = db.Where("(ip = ?) OR (url LIKE ?) OR (host LIKE ?) OR (host = ?)", target, target, target, target)
	}

	if db := db.Unscoped().Delete(&Risk{}); db.Error != nil {
		log.Errorf("delete risks failed: %s", db.Error)
	}
	log.Infof("delete risk by targets: %s finished", target)
}

func YieldRisksByTarget(db *gorm.DB, ctx context.Context, target string) chan *Risk {
	outC := make(chan *Risk)
	db = db.Model(&Risk{})
	var host, port, _ = utils.ParseStringToHostPort(target)
	if port > 0 {
		db = db.Where("port = ?", port)
		if host != "" {
			db = db.Where("(host = ?) OR (ip = ?)", host, host)
		}
	} else {
		db = db.Where("(ip = ?) OR (url LIKE ?) OR (host LIKE ?) OR (host = ?)", target, target, target, target)
	}

	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*Risk
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}

func YieldRisksByRuntimeId(db *gorm.DB, ctx context.Context, runtimeId string) chan *Risk {
	outC := make(chan *Risk)
	db = db.Model(&Risk{})
	db = db.Where("runtime_id = ?", runtimeId)

	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*Risk
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}

func YieldRisksByCreateAt(db *gorm.DB, ctx context.Context, timestamp int64) chan *Risk {
	outC := make(chan *Risk)
	db = db.Model(&Risk{})
	db = bizhelper.QueryDateTimeAfterTimestampOr(db, "created_at", timestamp)

	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*Risk
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}

func QueryNewRisk(db *gorm.DB, req *ypb.QueryNewRiskRequest, newRisk bool, isRead bool) (*bizhelper.Paginator, []*Risk, error) {
	if req == nil {
		return nil, nil, utils.Errorf("empty params")
	}
	db = db.Model(&Risk{})
	if newRisk {
		db = db.Where("id > ?", req.AfterId)
	}
	// 未读
	if !isRead {
		db = db.Where("is_read = false")
	}
	db = db.Where("risk_type NOT IN (?) OR ip <> ?", []string{"reverse-http", "reverse-tcp", "reverse-https"}, "127.0.0.1")
	db = db.Order("id desc")
	var ret []*Risk
	paging, db := bizhelper.Paging(db, 1, 5, &ret)

	if db.Error != nil {
		return nil, nil, utils.Errorf("QueryNewRisk failed: %s", db.Error)
	}

	return paging, ret, nil
}

func NewRiskReadRequest(db *gorm.DB, req *ypb.NewRiskReadRequest, Ids []int64) error {
	db = db.Model(&Risk{})
	if len(Ids) > 0 {
		db = db.Where("id in (?)", Ids)
	} else {
		db = db.Where("id > ?", req.AfterId)
	}
	db = db.Update(map[string]interface{}{"is_read": true})
	if db.Error != nil {
		return utils.Errorf("NewRiskReadRequest failed %s", db.Error)
	}
	return nil
}

func YieldRisks(db *gorm.DB, ctx context.Context) chan *Risk {
	outC := make(chan *Risk)
	db = db.Model(&Risk{})
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*Risk
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 15,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 15 {
				return
			}
		}
	}()
	return outC
}

func UploadRiskToOnline(db *gorm.DB, hash []string) error {
	db = db.Model(&Risk{})
	db = db.Where("hash in (?)", hash)
	db = db.Update(map[string]interface{}{"upload_online": true})
	if db.Error != nil {
		return utils.Errorf("UploadRiskToOnline failed %s", db.Error)
	}
	return nil
}
