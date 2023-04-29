package yakit

import "github.com/jinzhu/gorm"

type EnterpriseDetails struct {
	gorm.Model

	// 社会信用识别码
	SocialCreditCode string `json:"social_credit_code" gorm:"unique_index"`
	TaxCode          string `json:"tax_code"` // 纳税人识别码
	OrgCode          string `json:"org_code"` // 企业机构代码
	BizCode          string `json:"biz_code"` // 工商注册码

	ControllerSocialCreditCode string `json:"controller_social_credit_code"`
	ControllerHoldingPercent   string `json:"controller_holding_percent"`

	//
	SearchKeyword string `json:"keyword"`
	DomainKeyword string `json:"domain_keyword"`

	ExtraJSON string `json:"extra_json"`
}
