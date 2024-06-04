package yakgrpc

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

// GetNaslFamilys get nasl families
func (s *Server) GetNaslFamilies(ctx context.Context, req *ypb.GetNaslFamiliesRequest) (*ypb.GetNaslFamiliesResponse, error) {
	family := req.GetName()
	db := consts.GetGormProfileDatabase()
	var res []struct {
		Family string
		Count  int
	}
	var scriptP *schema.NaslScript
	db.Table(scriptP.TableName()).Select("family,COUNT(*) as count").Where("family like ?", "%"+family+"%").Group("family").Find(&res)
	var familys []*ypb.NaslFamily
	for _, info := range res {
		familys = append(familys, &ypb.NaslFamily{
			Name:        info.Family,
			ScriptCount: int32(info.Count),
		})
	}
	return &ypb.GetNaslFamiliesResponse{Families: familys}, nil
}

// QueryNaslScript query nasl script
func (s *Server) QueryNaslScript(ctx context.Context, req *ypb.QueryNaslScriptRequest) (*ypb.QueryNaslScriptResponse, error) {
	db := consts.GetGormProfileDatabase()
	db = db.Debug()
	if req.Pagination == nil {
		req.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	p := req.Pagination
	if !utils.StringArrayContains([]string{
		"desc", "asc", "",
	}, strings.ToLower(req.GetPagination().GetOrder())) {
		return nil, utils.Error("invalid order")
	}

	orderOrdinary := "updated_at desc"
	if utils.StringArrayContains([]string{
		"created_at", "updated_at", "id", "script_name",
		"author",
	}, strings.ToLower(req.GetPagination().GetOrderBy())) {
		orderOrdinary = fmt.Sprintf("%v %v", req.GetPagination().GetOrderBy(), req.GetPagination().GetOrder())
		orderOrdinary = strings.TrimSpace(orderOrdinary)
	}

	if orderOrdinary != "" {
		db = db.Order(orderOrdinary)
	} else {
		db = db.Order("updated_at desc")
	}

	if req.GetKeyword() != "" {
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
			"script",
		}, strings.Split(req.GetKeyword(), ","), false)
	}

	if req.GetOID() != "" {
		db = db.Where("o_id = ?", req.GetOID())
	}

	if req.GetScriptName() != "" {
		db = db.Where("script_name like ?", "%"+req.GetScriptName()+"%")
	}
	if req.GetCategory() != "" {
		db = db.Where("category like ?", "%"+req.GetCategory()+"%")
	}
	if req.GetFamily() != "" {
		db = db.Where("family like ?", "%"+req.GetFamily()+"%")
	}
	if req.GetCVE() != "" {
		db = db.Where("cve like ?", "%"+req.GetCVE()+"%")
	}

	var ret []*schema.NaslScript
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return &ypb.QueryNaslScriptResponse{
		Pagination: req.Pagination,
		Total:      int64(paging.TotalRecord),
		Data:       NaslDbModelToGrpcModel(ret),
	}, nil
}

// NaslDbModelToGrpcModel convert NaslScript to ypb.NaslScript
func NaslDbModelToGrpcModel(scripts []*schema.NaslScript) (res []*ypb.NaslScript) {
	for _, script := range scripts {
		res = append(res, &ypb.NaslScript{
			OriginFileName:  script.OriginFileName,
			Hash:            script.Hash,
			OID:             script.OID,
			CVE:             script.CVE,
			ScriptName:      script.ScriptName,
			Script:          utils.EscapeInvalidUTF8Byte([]byte(script.Script)),
			Tags:            script.Tags,
			Version:         script.Version,
			Category:        script.Category,
			Family:          script.Family,
			Copyright:       script.Copyright,
			Dependencies:    script.Dependencies,
			RequirePorts:    script.RequirePorts,
			RequireKeys:     script.RequireKeys,
			ExcludeKeys:     script.ExcludeKeys,
			RequireUdpPorts: script.RequireUdpPorts,
			Xref:            script.Xref,
			Preferences:     script.Preferences,
			BugtraqID:       script.BugtraqId,
			MandatoryKeys:   script.MandatoryKeys,
			Timeout:         int32(script.Timeout),
		})
	}
	return
}
