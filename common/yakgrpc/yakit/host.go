package yakit

import (
	"context"
	"net"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func NewHost(ip string) (*schema.Host, error) {
	host := &schema.Host{}
	ipInstance := net.ParseIP(utils.FixForParseIP(ip))
	if ipInstance == nil {
		return nil, utils.Errorf("parse ip[%s] failed", ip)
	}

	host.IPInteger = utils.InetAtoN(ipInstance)
	host.IP = ip

	if utils.IsPrivateIP(ipInstance) {
		host.IsInPublicNet = false
	} else {
		host.IsInPublicNet = true
	}
	return host, nil
}
func CreateOrUpdateHost(db *gorm.DB, ip string, i interface{}) error {
	db = db.Model(&schema.Host{})

	if db := db.Where("ip = ?", ip).Assign(i).FirstOrCreate(&schema.Host{}); db.Error != nil {
		return utils.Errorf("create/update Host failed: %s", db.Error)
	}

	return nil
}

func GetHost(db *gorm.DB, id int64) (*schema.Host, error) {
	var req schema.Host
	if db := db.Model(&schema.Host{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Host failed: %s", db.Error)
	}

	return &req, nil
}

func GetHostByIP(db *gorm.DB, ip string) (*schema.Host, error) {
	var req schema.Host
	if db := db.Model(&schema.Host{}).Where("ip = ?", ip).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Host failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteHostByID(db *gorm.DB, id int64) error {
	if db := db.Model(&schema.Host{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&schema.Host{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func QueryHost(db *gorm.DB, params *ypb.QueryHostsRequest) (*bizhelper.Paginator, []*schema.Host, error) {
	if params == nil {
		return nil, nil, utils.Errorf("empty params")
	}
	db = db.Model(&schema.Host{})
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
	db = bizhelper.FuzzQueryLike(db, "domains", params.GetDomainKeyword())
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", params.GetNetwork())

	var ret []*schema.Host
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func YieldHosts(db *gorm.DB, ctx context.Context) chan *schema.Host {
	return bizhelper.YieldModel[*schema.Host](ctx, db)
}
