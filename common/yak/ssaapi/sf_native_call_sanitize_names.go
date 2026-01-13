package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

var filterFuncReg = regexp_utils.NewYakRegexpUtils(`(?i)(sanitiz|sanitis|encod(e|ing)|entit(y|ies)|escap(e|ing)|replace|regex|normaliz|canonical|anti|safe|secur|purif|purg|pure|valid(ate)?|strip|clean|clear|special|convert|remov|filter|whitelist|blacklist|render|encrypt|decrypt|hash|digest|xss|csrf|sql|protect|prevent|mitigat(e|ing))`)

var nativeCallSanitizeNames sfvm.NativeCallFunc = func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	var results []sfvm.ValueOperator
	v.Recursive(func(operator sfvm.ValueOperator) error {
		v, ok := operator.(*Value)
		if !ok {
			return nil
		}
		if !v.IsConstInst() {
			return nil
		}
		result := utils.InterfaceToString(v.GetConstValue())
		if res, _ := filterFuncReg.MatchString(result); res {
			results = append(results, operator)
		}
		return nil
	})
	if len(results) > 0 {
		return true, sfvm.NewValues(results...), nil
	}
	return false, nil, utils.Error("no sanitize name value found")
}
