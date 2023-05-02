package yakit

import (
	"context"
	"github.com/jinzhu/gorm"
	"net"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/bizhelper"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

type Host struct {
	gorm.Model

	IP        string `json:"ip" gorm:"unique_index"`
	IPInteger int64  `json:"ip_integer"`

	IsInPublicNet bool

	// splite by comma
	Domains string
}

func NewHost(ip string) (*Host, error) {
	host := &Host{}
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
	db = db.Model(&Host{})

	if db := db.Where("ip = ?", ip).Assign(i).FirstOrCreate(&Host{}); db.Error != nil {
		return utils.Errorf("create/update Host failed: %s", db.Error)
	}

	return nil
}

func GetHost(db *gorm.DB, id int64) (*Host, error) {
	var req Host
	if db := db.Model(&Host{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Host failed: %s", db.Error)
	}

	return &req, nil
}

func GetHostByIP(db *gorm.DB, ip string) (*Host, error) {
	var req Host
	if db := db.Model(&Host{}).Where("ip = ?", ip).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Host failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteHostByID(db *gorm.DB, id int64) error {
	if db := db.Model(&Host{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&Host{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func QueryHost(db *gorm.DB, params *ypb.QueryHostsRequest) (*bizhelper.Paginator, []*Host, error) {
	if params == nil {
		return nil, nil, utils.Errorf("empty params")
	}
	db = db.Model(&Host{})
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

	var ret []*Host
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func YieldHosts(db *gorm.DB, ctx context.Context) chan *Host {
	outC := make(chan *Host)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*Host
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
