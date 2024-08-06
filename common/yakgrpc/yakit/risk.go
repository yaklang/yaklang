package yakit

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)

func CreateOrUpdateRisk(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&schema.Risk{})

	var token string
	switch ret := i.(type) {
	case *schema.Risk:
		token = ret.ReverseToken
		if ret.FromYakScript == "" {
			ret.FromYakScript = consts.GetCurrentYakitPluginID()
		}
	case schema.Risk:
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
		if db := db.Model(&schema.Risk{}).Where(
			"reverse_token LIKE ?", "%"+token+"%",
		).Update(map[string]interface{}{
			"waiting_verified": false,
		}); db.Error != nil {
			log.Errorf("reverse_token[%v] found cannot trigger unfinished risk.", token)
		}
	}

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&schema.Risk{}); db.Error != nil {
		return utils.Errorf("create/update Risk failed: %s", db.Error)
	}

	return nil
}

func GetRisk(db *gorm.DB, id int64) (*schema.Risk, error) {
	var r schema.Risk
	if db := db.Model(&schema.Risk{}).Where("id = ?", id).First(&r); db.Error != nil {
		return nil, utils.Errorf("get Risk failed: %s", db.Error)
	}
	return &r, nil
}

func GetRisksByRuntimeId(db *gorm.DB, runtimeId string) ([]*schema.Risk, error) {
	var r []*schema.Risk
	if db := db.Model(&schema.Risk{}).Where("runtime_id = ?", runtimeId).Find(&r); db.Error != nil {
		return nil, utils.Errorf("get Risks failed: %s", db.Error)
	}
	return r, nil
}

func CountRiskByRuntimeId(db *gorm.DB, runtimeId string) (int, error) {
	var count int
	if db := db.Model(&schema.Risk{}).Where("runtime_id = ?", runtimeId).Count(&count); db.Error != nil {
		return 0, utils.Errorf("get Risks count failed: %s", db.Error)
	}
	return count, nil
}

func GetRiskByHash(db *gorm.DB, hash string) (*schema.Risk, error) {
	var r schema.Risk
	if db := db.Model(&schema.Risk{}).Where("hash = ?", hash).First(&r); db.Error != nil {
		return nil, utils.Errorf("get Risk failed: %s", db.Error)
	}
	return &r, nil
}

func DeleteRiskByID(db *gorm.DB, ids ...int64) error {
	if len(ids) == 1 {
		id := ids[0]
		if db := db.Model(&schema.Risk{}).Where(
			"id = ?", id,
		).Unscoped().Delete(&schema.Risk{}); db.Error != nil {
			return db.Error
		}
		return nil
	}

	if db = bizhelper.ExactQueryInt64ArrayOr(db, "id", ids).Unscoped().Delete(&schema.Risk{}); db.Error != nil {
		return utils.Errorf("delete id(s) failed: %v", db.Error)
	}

	return nil
}

func DeleteRisk(db *gorm.DB, request *ypb.QueryRisksRequest) error {
	filterDb, err := FilterByQueryRisks(db, request)
	if err != nil {
		return err
	}
	if db := filterDb.Unscoped().Delete(&schema.Risk{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func FixRiskType(db *gorm.DB) {
	db.Model(&schema.Risk{}).Where("(severity = ?) OR (severity is null)", "").Updates(map[string]interface{}{
		"severity": "default",
	})
	db.Model(&schema.Risk{}).Where("(risk_type = ?) OR (risk_type is null)", "").Updates(map[string]interface{}{
		"risk_type": "default",
	})

	// 修复 nuclei 漏洞保存格式
}

func FilterByQueryRisks(db *gorm.DB, params *ypb.QueryRisksRequest) (_ *gorm.DB, _ error) {
	db = db.Model(&schema.Risk{})
	if runtimeId := params.GetRuntimeId(); runtimeId != "" {
		db = db.Where("runtime_id = ?", runtimeId)
	}
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
		db, "risk_type_verbose",
		utils.PrettifyListFromStringSplitEx(params.GetRiskType()),
	)
	db = bizhelper.FuzzQueryStringArrayOrPrefixLike(
		db, "severity",
		utils.PrettifyListFromStringSplitEx(params.GetSeverity()),
	)
	db = bizhelper.FuzzQueryStringArrayOrLike(
		db, "tags",
		utils.PrettifyListFromStringSplitEx(params.GetTags(), "|"),
	)
	if params.IsRead == "false" {
		db = db.Where("is_read = false OR is_read IS NULL")
	}
	db = bizhelper.FuzzSearchEx(db, []string{
		"title", "title_verbose",
	}, params.GetTitle(), false)
	db = bizhelper.ExactQueryInt64ArrayOr(db, "id", params.GetIds())
	//db = bizhelper.ExactQueryString(db, "reverse_token", params.GetToken())
	return db, nil
}

func QueryRisks(db *gorm.DB, params *ypb.QueryRisksRequest) (*bizhelper.Paginator, []*schema.Risk, error) {
	if params == nil {
		return nil, nil, utils.Errorf("empty params")
	}
	db = db.Model(&schema.Risk{}) // .Debug()
	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	db = db.Debug()
	p := params.Pagination
	db = bizhelper.QueryOrder(db, p.OrderBy, p.Order)

	if params.GetFromId() > 0 {
		log.Infof("query offset from id: %v", params.GetFromId())
		db = db.Where("id > ?", params.GetFromId())
	}

	if params.GetUntilId() > 0 {
		db = db.Where("id < ?", params.GetUntilId())
	}

	var err error
	db, err = FilterByQueryRisks(db, params)
	if err != nil {
		return nil, nil, err
	}
	var ret []*schema.Risk
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func DeleteRiskByTarget(db *gorm.DB, target string) {
	db = db.Model(&schema.Risk{})
	host, port, _ := utils.ParseStringToHostPort(target)
	if port > 0 {
		db = db.Where("port = ?", port)
		if host != "" {
			db = db.Where("(host = ?) OR (ip = ?)", host, host)
		}
	} else {
		db = db.Where("(ip = ?) OR (url LIKE ?) OR (host LIKE ?) OR (host = ?)", target, target, target, target)
	}

	if db := db.Unscoped().Delete(&schema.Risk{}); db.Error != nil {
		log.Errorf("delete risks failed: %s", db.Error)
	}
	log.Infof("delete risk by targets: %s finished", target)
}

func YieldRisksByTarget(db *gorm.DB, ctx context.Context, target string) chan *schema.Risk {
	outC := make(chan *schema.Risk)
	db = db.Model(&schema.Risk{})
	host, port, _ := utils.ParseStringToHostPort(target)
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

		page := 1
		for {
			var items []*schema.Risk
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

func YieldRisksByRuntimeId(db *gorm.DB, ctx context.Context, runtimeId string) chan *schema.Risk {
	outC := make(chan *schema.Risk)
	db = db.Model(&schema.Risk{})
	db = db.Where("runtime_id = ?", runtimeId)

	go func() {
		defer close(outC)

		page := 1
		for {
			var items []*schema.Risk
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

func YieldRisksByCreateAt(db *gorm.DB, ctx context.Context, timestamp int64) chan *schema.Risk {
	outC := make(chan *schema.Risk)
	db = db.Model(&schema.Risk{})
	db = bizhelper.QueryDateTimeAfterTimestampOr(db, "created_at", timestamp)

	go func() {
		defer close(outC)

		page := 1
		for {
			var items []*schema.Risk
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

func QueryNewRisk(db *gorm.DB, req *ypb.QueryNewRiskRequest, newRisk bool, isRead bool) (*bizhelper.Paginator, []*schema.Risk, error) {
	if req == nil {
		return nil, nil, utils.Errorf("empty params")
	}
	db = db.Model(&schema.Risk{})
	if newRisk {
		db = db.Where("id > ?", req.AfterId)
	}
	// 未读
	if !isRead {
		db = db.Where("is_read = false OR is_read IS NULL")
	}
	db = db.Where("waiting_verified = false")
	db = db.Where("risk_type NOT IN (?) OR ip <> ?", []string{"reverse-http", "reverse-tcp", "reverse-https"}, "127.0.0.1")
	db = db.Order("id desc")
	var ret []*schema.Risk
	paging, db := bizhelper.Paging(db, 1, 5, &ret)

	if db.Error != nil {
		return nil, nil, utils.Errorf("QueryNewRisk failed: %s", db.Error)
	}

	return paging, ret, nil
}

func NewRiskReadRequest(db *gorm.DB, Ids []int64) error {
	db = db.Model(&schema.Risk{})
	if len(Ids) > 0 {
		db = bizhelper.ExactQueryInt64ArrayOr(db, "id", Ids)
	} else {
		db = db.Where("created_at <= ?", time.Unix(time.Now().Unix(), 0))
	}
	db = db.Update(map[string]interface{}{"is_read": true})
	if db.Error != nil {
		return utils.Errorf("NewRiskReadRequest failed %s", db.Error)
	}
	return nil
}

func YieldRisks(db *gorm.DB, ctx context.Context) chan *schema.Risk {
	outC := make(chan *schema.Risk)
	db = db.Model(&schema.Risk{})
	go func() {
		defer close(outC)

		page := 1
		for {
			var items []*schema.Risk
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
	db = db.Model(&schema.Risk{})
	db = db.Where("hash in (?)", hash)
	db = db.Update(map[string]interface{}{"upload_online": true})
	if db.Error != nil {
		return utils.Errorf("UploadRiskToOnline failed %s", db.Error)
	}
	return nil
}

func GetRiskByIDOrHash(db *gorm.DB, id int64, hash string) (*schema.Risk, error) {
	var req schema.Risk
	if db := db.Model(&schema.Risk{}).Where("id = ? OR hash = ?", id, hash).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Risk failed: %s", db.Error)
	}

	return &req, nil
}

func UpdateRiskTags(db *gorm.DB, i *schema.Risk) error {
	if i == nil {
		return nil
	}
	db = db.Model(&schema.Risk{})

	if i.ID > 0 {
		if db = db.Where("id = ?", i.ID).Update("tags", i.Tags); db.Error != nil {
			log.Errorf("update tags(by id) failed: %s", db.Error)
			return db.Error
		}
	} else if i.Hash != "" {
		if db = db.Where("hash = ?", i.Hash).Update("tags", i.Tags); db.Error != nil {
			log.Errorf("update tags(by hash) failed: %s", db.Error)
			return db.Error
		}
	}
	return nil
}

func QueryRiskCount(db *gorm.DB, isRead string) (int64, error) {
	db = db.Model(&schema.Risk{})
	// 未读
	if isRead == "false" {
		db = db.Where("is_read = false OR is_read IS NULL ")
	}
	db = db.Where("waiting_verified = false")
	var count int64
	db.Count(&count)
	if db.Error != nil {
		return 0, utils.Errorf("QueryRiskCount failed: %s", db.Error)
	}
	return count, nil
}
