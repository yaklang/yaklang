package yakit

import (
	"context"
	"github.com/jinzhu/gorm"
	"io/ioutil"
	"sort"
	"strings"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/bizhelper"
	"yaklang/common/yakgrpc/ypb"
)

type Domain struct {
	gorm.Model

	Domain    string `json:"domain" gorm:"index"`
	IPAddr    string `json:"ip_addr"`
	IPInteger int64  `json:"ip_integer"`

	HTTPTitle string

	Hash string `json:"hash" gorm:"unique_index"`

	Tags string `json:"tags"`
}

func (d *Domain) CalcHash() string {
	return utils.CalcSha1(d.Domain, d.IPAddr)
}

func (d *Domain) BeforeSave() error {
	d.Hash = d.CalcHash()
	return nil
}

var (
	saveDomainSWG = utils.NewSizedWaitGroup(50)
)

func (d *Domain) FillDomainHTTPInfo() {
	saveDomainSWG.Add()
	defer saveDomainSWG.Done()
	if d.Domain == "" {
		return
	}

	httpClient := utils.NewDefaultHTTPClient()
	updateStatus := func(urlStr string) error {
		rsp, err := httpClient.Get(urlStr)
		if err != nil {
			return err
		}
		raw, err := ioutil.ReadAll(rsp.Body)
		if err != nil {
			return err
		}
		title := utils.ExtractTitleFromHTMLTitle(utils.EscapeInvalidUTF8Byte(raw), "")
		d.HTTPTitle = title
		return nil
	}

	for _, url := range utils.ParseStringToUrls(d.Domain) {
		url := url
		err := updateStatus(url)
		if err != nil {
			continue
		}
		break
	}
}

func CreateOrUpdateDomain(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&Domain{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&Domain{}); db.Error != nil {
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

	d := &Domain{
		Domain:    domain,
		IPAddr:    ip,
		IPInteger: host.IPInteger,
	}
	d.FillDomainHTTPInfo()
	_ = CreateOrUpdateDomain(db, d.CalcHash(), d)
	return nil
}

func GetDomain(db *gorm.DB, id int64) (*Domain, error) {
	var req Domain
	if db := db.Model(&Domain{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Domain failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteDomainByID(db *gorm.DB, ids ...int64) error {
	if len(ids) == 1 {
		id := ids[0]
		if db := db.Model(&Domain{}).Where(
			"id = ?", id,
		).Unscoped().Delete(&Domain{}); db.Error != nil {
			return db.Error
		}
		return nil
	}
	if db = bizhelper.ExactQueryInt64ArrayOr(db, "id", ids).Unscoped().Delete(&Domain{}); db.Error != nil {
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

func QueryDomain(db *gorm.DB, params *ypb.QueryDomainsRequest) (*bizhelper.Paginator, []*Domain, error) {
	if params == nil {
		return nil, nil, utils.Errorf("empty params")
	}
	db = db.Model(&Domain{}) // .Debug()
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

	var ret []*Domain
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func YieldDomains(db *gorm.DB, ctx context.Context) chan *Domain {
	outC := make(chan *Domain)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*Domain
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
