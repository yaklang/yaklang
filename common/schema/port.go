package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
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

	// runtime id 运行时 ID
	RuntimeId string `json:"runtime_id"`
}

func (p *Port) CalcHash() string {
	return utils.CalcSha1(p.Host, p.Port, p.Proto, p.TaskName, p.RuntimeId)
}

func (p *Port) BeforeSave() error {
	if p.IPInteger <= 0 {
		ipInt, _ := utils.IPv4ToUint64(p.Host)
		p.IPInteger = int(ipInt)
	}
	p.Hash = p.CalcHash()
	return nil
}
