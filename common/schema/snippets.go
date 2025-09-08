package schema

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type Snippets struct {
	gorm.Model
	SnippetId   string `gorm:"uniqueIndex;not null" json:"snippet_id"`
	SnippetName string `gorm:"type:text;not null" json:"snippet_name"`
	SnippetBody string `gorm:"type:text;not null" json:"snippet_data"`
	SnippetDesc string `gorm:"type:text" json:"snippet_desc"`

	SnippetLevel SnippetsLevel `gorm:"type:text;not null" json:"snippet_level"`
	SnippetState SnippetsType  `gorm:"type:text;not null" json:"snippet_state"`
}

// TableName 指定表名
func (Snippets) TableName() string {
	return "snippets"
}

type SnippetsType string

const (
	Snippets_Type_None SnippetsType = "none"
	Snippets_Type_Http SnippetsType = "http"
	Snippets_Type_Yak  SnippetsType = "yak"
)

type SnippetsLevel string

const (
	Snippets_Level_None          SnippetsLevel = "none"
	Snippets_Level_Method        SnippetsLevel = "method"
	Snippets_Level_Function      SnippetsLevel = "function"
	Snippets_Level_Constructor   SnippetsLevel = "constructor"
	Snippets_Level_Field         SnippetsLevel = "field"
	Snippets_Level_Variable      SnippetsLevel = "variable"
	Snippets_Level_Class         SnippetsLevel = "class"
	Snippets_Level_Struct        SnippetsLevel = "struct"
	Snippets_Level_Interface     SnippetsLevel = "interface"
	Snippets_Level_Module        SnippetsLevel = "module"
	Snippets_Level_Property      SnippetsLevel = "property"
	Snippets_Level_Event         SnippetsLevel = "event"
	Snippets_Level_Operator      SnippetsLevel = "operator"
	Snippets_Level_Unit          SnippetsLevel = "unit"
	Snippets_Level_Value         SnippetsLevel = "value"
	Snippets_Level_Constant      SnippetsLevel = "constant"
	Snippets_Level_Enum          SnippetsLevel = "enum"
	Snippets_Level_EnumMember    SnippetsLevel = "enum_member"
	Snippets_Level_Keyword       SnippetsLevel = "keyword"
	Snippets_Level_Text          SnippetsLevel = "text"
	Snippets_Level_Color         SnippetsLevel = "color"
	Snippets_Level_File          SnippetsLevel = "file"
	Snippets_Level_Reference     SnippetsLevel = "reference"
	Snippets_Level_CustomColor   SnippetsLevel = "custom_color"
	Snippets_Level_Folder        SnippetsLevel = "folder"
	Snippets_Level_TypeParameter SnippetsLevel = "type_parameter"
	Snippets_Level_User          SnippetsLevel = "user"
	Snippets_Level_Issue         SnippetsLevel = "issue"
	Snippets_Level_Snippet       SnippetsLevel = "snippet"
)

func SwitcSnippetsType(in string) SnippetsType {
	in = strings.ToLower(in)
	switch in {
	case "http":
		return Snippets_Type_Http
	case "yak":
		return Snippets_Type_Yak
	default:
		return Snippets_Type_None
	}
}

func SwitcSnippetsLevel(in string) SnippetsLevel {
	in = strings.ToLower(in)
	switch in {
	case "method":
		return Snippets_Level_Method
	case "function":
		return Snippets_Level_Function
	case "constructor":
		return Snippets_Level_Constructor
	case "field":
		return Snippets_Level_Field
	case "variable":
		return Snippets_Level_Variable
	case "class":
		return Snippets_Level_Class
	case "struct":
		return Snippets_Level_Struct
	case "interface":
		return Snippets_Level_Interface
	case "module":
		return Snippets_Level_Module
	case "property":
		return Snippets_Level_Property
	case "event":
		return Snippets_Level_Event
	case "operator":
		return Snippets_Level_Operator
	case "unit":
		return Snippets_Level_Unit
	case "value":
		return Snippets_Level_Value
	case "constant":
		return Snippets_Level_Constant
	case "enum":
		return Snippets_Level_Enum
	case "enum_member":
		return Snippets_Level_EnumMember
	case "keyword":
		return Snippets_Level_Keyword
	case "text":
		return Snippets_Level_Text
	case "color":
		return Snippets_Level_Color
	case "file":
		return Snippets_Level_File
	case "reference":
		return Snippets_Level_Reference
	case "custom_color":
		return Snippets_Level_CustomColor
	case "folder":
		return Snippets_Level_Folder
	case "type_parameter":
		return Snippets_Level_TypeParameter
	case "user":
		return Snippets_Level_User
	case "issue":
		return Snippets_Level_Issue
	case "snippet":
		return Snippets_Level_Snippet
	default:
		return Snippets_Level_None
	}
}

func NewSnippets(req *ypb.SnippetsRequest) *Snippets {

	return &Snippets{
		SnippetName:  req.GetName(),
		SnippetBody:  req.GetCode(),
		SnippetDesc:  req.GetDescription(),
		SnippetState: SwitcSnippetsType(req.GetState()),
		SnippetLevel: SwitcSnippetsLevel(req.GetLevel()),
	}
}
