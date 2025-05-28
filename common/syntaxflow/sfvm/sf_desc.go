package sfvm

import (
	"github.com/samber/lo"
	"strings"
)

type SFDescKeyType string

const (
	SFDescKeyType_Unknown   SFDescKeyType = "unknown"
	SFDescKeyType_Title     SFDescKeyType = "title"
	SFDescKeyType_Title_ZH  SFDescKeyType = "title_zh"
	SFDescKeyType_Desc      SFDescKeyType = "desc"
	SFDescKeyType_Type      SFDescKeyType = "type"
	SFDescKeyType_Lib       SFDescKeyType = "lib"
	SFDescKeyType_Level     SFDescKeyType = "level"
	SFDescKeyType_Lang      SFDescKeyType = "language"
	SFDescKeyType_CVE       SFDescKeyType = "cve"
	SFDescKeyType_Risk      SFDescKeyType = "risk"
	SFDescKeyType_Solution  SFDescKeyType = "solution"
	SFDescKeyType_Rule_Id   SFDescKeyType = "rule_id"
	SFDescKeyType_Reference SFDescKeyType = "reference"
)

func ValidDescItemKeyType(key string) SFDescKeyType {
	switch strings.ToLower(key) {
	case "title":
		return SFDescKeyType_Title
	case "title_zh":
		return SFDescKeyType_Title_ZH
	case "description", "desc", "note":
		return SFDescKeyType_Desc
	case "type", "purpose":
		return SFDescKeyType_Type
	case "lib", "allow_include", "as_library", "as_lib", "library_name":
		return SFDescKeyType_Lib
	case "level", "severity", "sev":
		return SFDescKeyType_Level
	case "language", "lang":
		return SFDescKeyType_Lang
	case "cve":
		return SFDescKeyType_CVE
	case "risk_type", "risk":
		return SFDescKeyType_Risk
	case "solution", "fix":
		return SFDescKeyType_Solution
	case "rule_id", "id":
		return SFDescKeyType_Rule_Id
	case "reference", "ref":
		return SFDescKeyType_Reference
	default:
		return SFDescKeyType_Unknown
	}
}

// GetSupplyInfoDescKeyType 拿到所有desc item中，
// 用于给规则扩充提示信息的key
func GetSupplyInfoDescKeyType() []SFDescKeyType {
	return []SFDescKeyType{
		SFDescKeyType_Title,
		SFDescKeyType_Title_ZH,
		SFDescKeyType_Desc,
		SFDescKeyType_Solution,
		SFDescKeyType_Reference,
	}
}

func GetBasisSupplyInfoDescKeyType() []SFDescKeyType {
	keys := GetSupplyInfoDescKeyType()
	return lo.Filter(keys, func(item SFDescKeyType, index int) bool {
		if IsComplexInfoDescType(item) {
			return false
		} else {
			return true
		}
	})
}

func GetComplexSupplyInfoDescKeyType() []SFDescKeyType {
	keys := GetSupplyInfoDescKeyType()
	return lo.Filter(keys, func(item SFDescKeyType, index int) bool {
		if IsComplexInfoDescType(item) {
			return true
		} else {
			return false
		}
	})
}

func IsComplexInfoDescType(typ SFDescKeyType) bool {
	switch typ {
	case SFDescKeyType_Desc, SFDescKeyType_Solution, SFDescKeyType_Reference:
		return true
	default:
		return false
	}
}
