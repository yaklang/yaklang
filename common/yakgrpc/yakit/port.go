package yakit

import (
	"context"
	"github.com/jinzhu/gorm"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/bizhelper"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

type Port struct {
	gorm.Model

	Host        string `json:"host" gorm:"index"`
	IPInteger   int    `json:"ip_integer" gorm:"column:ip_integer" json:"ip_integer"`
	Port        int    `json:"port" gorm:"index"`
	Proto       string `json:"proto"`
	ServiceType string `json:"service_type"`
	State       string `json:"state"`
	Reason      string `json:"reason"`
	Fingerprint string `json:"fingerprint"`
	CPE         string `json:"cpe"`
	HtmlTitle   string `json:"html_title"`
	From        string `json:"from"`
	Hash        string `json:"hash"`
	TaskName    string `json:"task_name"`
}

func (p *Port) CalcHash() string {
	return utils.CalcSha1(p.Host, p.Port, p.TaskName)
}

func (p *Port) BeforeSave() error {
	if p.IPInteger <= 0 {
		ipInt, _ := utils.IPv4ToUint64(p.Host)
		p.IPInteger = int(ipInt)
	}
	p.Hash = p.CalcHash()
	return nil
}

func CreateOrUpdatePort(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&Port{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&Port{}); db.Error != nil {
		return utils.Errorf("create/update Port failed: %s", db.Error)
	}

	return nil
}

func GetPort(db *gorm.DB, id int64) (*Port, error) {
	var req Port
	if db := db.Model(&Port{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Port failed: %s", db.Error)
	}

	return &req, nil
}

func DeletePortByID(db *gorm.DB, id int64) error {
	if db := db.Model(&Port{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&Port{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func QueryPorts(db *gorm.DB, params *ypb.QueryPortsRequest) (*bizhelper.Paginator, []*Port, error) {
	if params == nil {
		return nil, nil, utils.Errorf("empty params")
	}
	db = db.Model(&Port{}) // .Debug()
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
	db = FilterPort(db, params)
	/*db = bizhelper.QueryBySpecificPorts(db, "port", params.GetPorts())
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", params.GetHosts())
	db = bizhelper.FuzzQueryLike(db, "service_type", params.GetService())
	db = bizhelper.FuzzQueryLike(db, "html_title", params.GetTitle())

	if params.GetState() == "" {
		db = bizhelper.ExactQueryString(db, "state", "open")
	} else {
		db = bizhelper.ExactQueryString(db, "state", params.GetState())
	}*/

	var ret []*Port
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func FilterPort(db *gorm.DB, params *ypb.QueryPortsRequest) *gorm.DB {
	db = bizhelper.QueryBySpecificPorts(db, "port", params.GetPorts())
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", params.GetHosts())
	db = bizhelper.FuzzQueryLike(db, "service_type", params.GetService())
	db = bizhelper.FuzzQueryLike(db, "html_title", params.GetTitle())

	if params.GetState() == "" {
		db = bizhelper.ExactQueryString(db, "state", "open")
	} else {
		db = bizhelper.ExactQueryString(db, "state", params.GetState())
	}
	return db
}

func YieldSimplePorts(db *gorm.DB, ctx context.Context) chan *SimplePort {
	outC := make(chan *SimplePort)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*SimplePort
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

func YieldPorts(db *gorm.DB, ctx context.Context) chan *Port {
	outC := make(chan *Port)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*Port
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

func FilterByQueryPorts(db *gorm.DB, params *ypb.QueryPortsRequest) (_ *gorm.DB, _ error) {
	db = db.Model(&Port{})
	db = bizhelper.QueryBySpecificPorts(db, "port", params.GetPorts())
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", params.GetHosts())
	db = bizhelper.FuzzQueryLike(db, "service_type", params.GetService())
	db = bizhelper.FuzzQueryLike(db, "html_title", params.GetTitle())

	if params.GetState() == "" {
		db = bizhelper.ExactQueryString(db, "state", "open")
	} else {
		db = bizhelper.ExactQueryString(db, "state", params.GetState())
	}
	return db, nil
}

func DeletePortsByID(db *gorm.DB, id int64) error {
	if db := db.Model(&Port{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&Port{}); db.Error != nil {
		return db.Error
	}
	return nil
}
