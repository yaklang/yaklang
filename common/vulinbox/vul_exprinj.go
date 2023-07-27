package vulinbox

import (
	"bytes"
	"encoding/json"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"net/http"
)

func (s *VulinServer) registerExprInj() {
	r := s.router
	exprGroup := r.PathPrefix("/expr").Name("表达式注入或 SSTI 仿真").Subrouter()

	handler := func(writer http.ResponseWriter, request *http.Request) {
		raw, _ := utils.HttpDumpWithBody(request, true)
		println(string(raw))

		var buf bytes.Buffer

		buf.WriteString("-----------------ORIGIN PACKET---------------\n")
		buf.Write(raw)
		buf.WriteString("-----------------Handled---------------\n")

		for _, paramName := range []string{"a", "b", "c"} {
			expr1 := request.URL.Query().Get(paramName)
			buf.WriteString(paramName + "[" + expr1 + "]: ")
			sanbox := httptpl.NewNucleiDSLYakSandbox()

			if paramName == "b" {
				var mapRaw = make(map[string]interface{})
				err := json.Unmarshal([]byte(expr1), &mapRaw)
				if err != nil {
					buf.WriteString("\n\nb params is should be JSON!!!!!!!!!!!!!!!!!\n\n")
					log.Errorf("json unmarshal failed: %v", err)
					continue
				}
				expr1 = utils.MapGetString(mapRaw, "a")
			}

			aResult, err := sanbox.Execute(expr1, nil)
			if err != nil {
				buf.WriteString(err.Error())
			} else {
				buf.WriteString(utils.InterfaceToString(aResult))
			}
			buf.WriteByte('\n')
			buf.WriteByte('\n')
		}
		writer.Write(buf.Bytes())
	}

	var vuls = []*VulInfo{
		{
			Path:         "/injection",
			DefaultQuery: "a=1",
			Title:        "表达式注入GET参数基础",
			Handler:      handler,
		},
		{
			Path:         "/injection",
			DefaultQuery: "b={\"a\":1}",
			Title:        "JSON 中表达式注入参数",
			Handler:      handler,
		},
		{
			Path:         "/injection",
			DefaultQuery: "c=abc",
			Title:        "表达式注入GET参数基础（非数字）",
			Handler:      handler,
		},
	}
	for _, v := range vuls {
		addRouteWithVulInfo(exprGroup, v)
	}
}
