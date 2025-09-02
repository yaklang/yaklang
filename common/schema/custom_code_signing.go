package schema

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type CustomCodeSigning struct {
	gorm.Model
	CustomCodeId   string `gorm:"uniqueIndex;not null" json:"custom_code_id"`
	CustomCodeName string `gorm:"type:text;not null" json:"custom_code_name"`
	CustomCodeData string `gorm:"type:text;not null" json:"custom_code_data"`
	CustomCodeDesc string `gorm:"type:text" json:"custom_code_desc"`

	CustomCodeLevel CustomCodeSigningLevel `gorm:"type:text;not null" json:"custom_code_level"`
	CustomCodeState CustomCodeSigningType  `gorm:"type:text;not null" json:"custom_code_state"`
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

type CustomCodeSigningLevel string

const (
	CustomCodeSigning_Level_None          CustomCodeSigningLevel = "none"
	CustomCodeSigning_Level_Method        CustomCodeSigningLevel = "method"
	CustomCodeSigning_Level_Function      CustomCodeSigningLevel = "function"
	CustomCodeSigning_Level_Constructor   CustomCodeSigningLevel = "constructor"
	CustomCodeSigning_Level_Field         CustomCodeSigningLevel = "field"
	CustomCodeSigning_Level_Variable      CustomCodeSigningLevel = "variable"
	CustomCodeSigning_Level_Class         CustomCodeSigningLevel = "class"
	CustomCodeSigning_Level_Struct        CustomCodeSigningLevel = "struct"
	CustomCodeSigning_Level_Interface     CustomCodeSigningLevel = "interface"
	CustomCodeSigning_Level_Module        CustomCodeSigningLevel = "module"
	CustomCodeSigning_Level_Property      CustomCodeSigningLevel = "property"
	CustomCodeSigning_Level_Event         CustomCodeSigningLevel = "event"
	CustomCodeSigning_Level_Operator      CustomCodeSigningLevel = "operator"
	CustomCodeSigning_Level_Unit          CustomCodeSigningLevel = "unit"
	CustomCodeSigning_Level_Value         CustomCodeSigningLevel = "value"
	CustomCodeSigning_Level_Constant      CustomCodeSigningLevel = "constant"
	CustomCodeSigning_Level_Enum          CustomCodeSigningLevel = "enum"
	CustomCodeSigning_Level_EnumMember    CustomCodeSigningLevel = "enum_member"
	CustomCodeSigning_Level_Keyword       CustomCodeSigningLevel = "keyword"
	CustomCodeSigning_Level_Text          CustomCodeSigningLevel = "text"
	CustomCodeSigning_Level_Color         CustomCodeSigningLevel = "color"
	CustomCodeSigning_Level_File          CustomCodeSigningLevel = "file"
	CustomCodeSigning_Level_Reference     CustomCodeSigningLevel = "reference"
	CustomCodeSigning_Level_CustomColor   CustomCodeSigningLevel = "custom_color"
	CustomCodeSigning_Level_Folder        CustomCodeSigningLevel = "folder"
	CustomCodeSigning_Level_TypeParameter CustomCodeSigningLevel = "type_parameter"
	CustomCodeSigning_Level_User          CustomCodeSigningLevel = "user"
	CustomCodeSigning_Level_Issue         CustomCodeSigningLevel = "issue"
	CustomCodeSigning_Level_Snippet       CustomCodeSigningLevel = "snippet"
)

func SwitcCustomCodeSigningType(in string) CustomCodeSigningType {
	in = strings.ToLower(in)
	switch in {
	case "http":
		return CustomCodeSigning_Type_Http
	case "yak":
		return CustomCodeSigning_Type_Yak
	default:
		return CustomCodeSigning_Type_None
	}
}

func SwitcCustomCodeSigningLevel(in string) CustomCodeSigningLevel {
	in = strings.ToLower(in)
	switch in {
	case "method":
		return CustomCodeSigning_Level_Method
	case "function":
		return CustomCodeSigning_Level_Function
	case "constructor":
		return CustomCodeSigning_Level_Constructor
	case "field":
		return CustomCodeSigning_Level_Field
	case "variable":
		return CustomCodeSigning_Level_Variable
	case "class":
		return CustomCodeSigning_Level_Class
	case "struct":
		return CustomCodeSigning_Level_Struct
	case "interface":
		return CustomCodeSigning_Level_Interface
	case "module":
		return CustomCodeSigning_Level_Module
	case "property":
		return CustomCodeSigning_Level_Property
	case "event":
		return CustomCodeSigning_Level_Event
	case "operator":
		return CustomCodeSigning_Level_Operator
	case "unit":
		return CustomCodeSigning_Level_Unit
	case "value":
		return CustomCodeSigning_Level_Value
	case "constant":
		return CustomCodeSigning_Level_Constant
	case "enum":
		return CustomCodeSigning_Level_Enum
	case "enum_member":
		return CustomCodeSigning_Level_EnumMember
	case "keyword":
		return CustomCodeSigning_Level_Keyword
	case "text":
		return CustomCodeSigning_Level_Text
	case "color":
		return CustomCodeSigning_Level_Color
	case "file":
		return CustomCodeSigning_Level_File
	case "reference":
		return CustomCodeSigning_Level_Reference
	case "custom_color":
		return CustomCodeSigning_Level_CustomColor
	case "folder":
		return CustomCodeSigning_Level_Folder
	case "type_parameter":
		return CustomCodeSigning_Level_TypeParameter
	case "user":
		return CustomCodeSigning_Level_User
	case "issue":
		return CustomCodeSigning_Level_Issue
	case "snippet":
		return CustomCodeSigning_Level_Snippet
	default:
		return CustomCodeSigning_Level_None
	}
}

func NewCustomCodeSigning(req *ypb.CustomCodeRequest) *CustomCodeSigning {
	customName := req.GetName()
	customData := req.GetCode()
	customDesc := req.GetDescription()
	customState := req.GetState()
	customLevel := req.GetLevel()

	return &CustomCodeSigning{
		CustomCodeName:  customName,
		CustomCodeData:  customData,
		CustomCodeDesc:  customDesc,
		CustomCodeState: SwitcCustomCodeSigningType(customState),
		CustomCodeLevel: SwitcCustomCodeSigningLevel(customLevel),
	}
}
