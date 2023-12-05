package crawler

import (
	"github.com/yaklang/yaklang/common/log"
	js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func HandleJS(isHttps bool, req []byte, code string) {
	prog := js2ssa.ParseSSA(code, nil)
	js := ssaapi.NewProgram(prog)

	// handle fetch
	js.Ref("fetch").GetUsers().Filter(func(value *ssaapi.Value) bool {
		return value.IsCall() || value.IsField()
	}).ShowWithSource(true).ForEach(func(value *ssaapi.Value) {
		switch {
		case value.IsCall():
			log.Infof("fetch value: %v", value.String())
			args := value.GetCallArgs()
			targetUrl := args.Get(0).GetConstValue()
			if opt := args.Get(1); opt.IsMake() {
				// args.GetMakeSliceArgs()
				for _, field := range opt.GetMakeObjectFields() {
					switch field.GetFieldName().GetConstValue() {
					case "method":
						log.Infof("fetch method: %v", field.GetLatestFieldValue().GetConstValue())
					case "headers":
						log.Infof("fetch headers: %v", field.GetFirstFieldValue().GetConstValue())
					}
				}
			}
			log.Infof("fetch targetUrl: %v", targetUrl)
		}
	})
}
