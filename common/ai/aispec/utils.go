package aispec

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

func ShrinkAndSafeToFile(i any) string {
	var buf bytes.Buffer
	if utils.IsMap(i) {
		for k, v := range utils.InterfaceToGeneralMap(i) {
			buf.WriteString("# parameter: " + fmt.Sprint(k) + "\n")
			valString := utils.InterfaceToString(v)
			buf.WriteString(valString + "\n\n")
		}
	} else if funk.IsIteratee(i) {
		idx := 0
		funk.ForEach(i, func(element any) {
			idx++
			buf.WriteString("# parameter: " + fmt.Sprint(idx) + "\n")
			valString := utils.InterfaceToString(element)
			buf.WriteString(valString + "\n\n")
		})
	} else {
		buf.WriteString("# raw input " + "\n")
		buf.WriteString(utils.InterfaceToString(i))
	}
	results := strings.TrimRight(buf.String(), "\n")
	var promptString string
	if buf.Len() > 1024 {
		filename := consts.TempAIFileFast("huge-params-*.md", buf.String())
		promptString = utils.ShrinkString(results, 1000) + fmt.Sprintf(" [saved in %v]", filename)
	} else {
		promptString = results
	}
	return promptString
}
