package yakit

import (
	"context"
	"sort"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func CreateOrUpdateDomain(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&schema.Domain{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&schema.Domain{}); db.Error != nil {
		return utils.Errorf("create/update Domain failed: %s", db.Error)
	}

	return nil
}

func SaveDomain(db *gorm.DB, domain string, ip string) error {
	host, err := GetHostByIP(db, ip)
	if err != nil {
		host, err = NewHost(ip)
		if err != nil {
			return utils.Errorf("create ip host failed: %s", err)
		}
	}
	domains := utils.PrettifyListFromStringSplited(host.Domains, ",")
	domains = append(domains, domain)
	sort.Strings(domains)

	host.Domains = strings.Join(domains, ",")
	_ = CreateOrUpdateHost(db, host.IP, host)

	d := &schema.Domain{
		Domain:    domain,
		IPAddr:    ip,
		IPInteger: host.IPInteger,
	}
	d.FillDomainHTTPInfo()
	_ = CreateOrUpdateDomain(db, d.CalcHash(), d)
	return nil
}

func GetDomain(db *gorm.DB, id int64) (*schema.Domain, error) {
	var req schema.Domain
	if db := db.Model(&schema.Domain{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Domain failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteDomainByID(db *gorm.DB, ids ...int64) error {
	if len(ids) == 1 {
		id := ids[0]
		if db := db.Model(&schema.Domain{}).Where(
			"id = ?", id,
		).Unscoped().Delete(&schema.Domain{}); db.Error != nil {
			return db.Error
		}
		return nil
	}
	if db = bizhelper.ExactQueryInt64ArrayOr(db, "id", ids).Unscoped().Delete(&schema.Domain{}); db.Error != nil {
		return utils.Errorf("delete id(s) failed: %v", db.Error)
	}
	return nil
}

func FilterDomain(db *gorm.DB, params *ypb.QueryDomainsRequest) *gorm.DB {
	db = bizhelper.FuzzQueryLike(db, "domain", params.GetDomainKeyword())
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", params.GetNetwork())
	db = bizhelper.FuzzQueryLike(db, "http_title", params.GetTitle())
	return db
}

func QueryDomain(db *gorm.DB, params *ypb.QueryDomainsRequest) (*bizhelper.Paginator, []*schema.Domain, error) {
	if params == nil {
		return nil, nil, utils.Errorf("empty params")
	}
	db = db.Model(&schema.Domain{}) // .Debug()
	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}

	p := params.Pagination
	db = FilterDomain(db, params)

	var ret []*schema.Domain
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func YieldDomains(db *gorm.DB, ctx context.Context) chan *schema.Domain {
	return bizhelper.YieldModel[*schema.Domain](ctx, db)
}
