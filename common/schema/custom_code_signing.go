package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type CustomCodeSigning struct {
	gorm.Model
	CustomCodeId    string                `gorm:"uniqueIndex;not null" json:"custom_code_id"`
	CustomCodeName  string                `gorm:"type:text;not null" json:"custom_code_name"`
	CustomCodeData  string                `gorm:"type:text;not null" json:"custom_code_data"`
	CustomCodeDesc  string                `gorm:"type:text" json:"custom_code_desc"`
	CustomCodeState CustomCodeSigningType `gorm:"type:text;not null" json:"custom_code_state"`
}

// TableName 指定表名
func (CustomCodeSigning) TableName() string {
	return "custom_code_signings"
}

type CustomCodeSigningType string

const (
	CustomCodeSigning_Type_None CustomCodeSigningType = "none"
	CustomCodeSigning_Type_Http CustomCodeSigningType = "http"
	CustomCodeSigning_Type_Yak  CustomCodeSigningType = "yak"
)

func SwitcCustomCodeSigningType(in string) CustomCodeSigningType {
	switch in {
	case "http":
		return CustomCodeSigning_Type_Http
	case "yak":
		return CustomCodeSigning_Type_Yak
	default:
		return CustomCodeSigning_Type_None
	}
}

func NewCustomCodeSigning(req *ypb.CustomCodeRequest) *CustomCodeSigning {
	customName := req.GetName()
	customData := req.GetCode()
	customDesc := req.GetDescription()
	customState := req.GetState()

	return &CustomCodeSigning{
		CustomCodeName:  customName,
		CustomCodeData:  customData,
		CustomCodeDesc:  customDesc,
		CustomCodeState: SwitcCustomCodeSigningType(customState),
	}
}
