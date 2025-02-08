package vulinbox

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/log"
	"os"

	"net/http"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed html/ai-knowledge-base-practice.html
var aiKbPractice []byte

func (r *VulinServer) registerAIKBPractice() {
	var router = r.router
	router.HandleFunc("/_/submit-ai-practice", func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case "GET":
			var submitForm = aiKbPractice
			if consts.IsDevMode() {
				log.Info("debugmode, try to use local html")
				pathExisted := utils.GetFirstExistedFile(
					"common/vulinbox/html/ai-knowledge-base-practice.html",
					"html/ai-knowledge-base-practice.html",
				)
				if pathExisted != "" {
					raw, _ := os.ReadFile(pathExisted)
					if len(raw) > 0 {
						submitForm = raw
					}
				}
			}
			writer.Write(submitForm)
		}
	})
}
