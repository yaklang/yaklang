package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"io/ioutil"
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
