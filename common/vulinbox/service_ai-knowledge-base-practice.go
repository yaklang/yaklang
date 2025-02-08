package vulinbox

import (
	_ "embed"
	"encoding/json"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"io"
	"os"

	"net/http"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed html/ai-knowledge-base-practice.html
var aiKbPractice []byte

func (r *VulinServer) registerAIKBPractice() {
	var router = r.router
	router.HandleFunc("/_/ai-analyze", func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			writer.WriteHeader(500)
			return
		}
		var data map[string]any
		if err := json.Unmarshal(body, &data); err != nil {
			writer.WriteHeader(500)
			return
		}

		code := utils.MapGetStringByManyFields(data, "codeContent", "codeText", "code")
		/*
		   {
		       "id": "example_001",
		       "category": "basic_syntax",
		       "title": "变量声明示例",
		       "description": "展示如何声明和使用变量",
		       "code": "var x = 42;\nprint(x);",
		       "explanation": "这个例子展示了变量声明和打印的基本用法",
		       "tags": ["variables", "basic", "printing"],
		       "difficulty": "beginner",
		       "use_cases": ["教学", "入门"],
		       "related_examples": ["example_002", "example_003"]
		   }
		*/
		results, err := ai.FunctionCall(`# 数据源

我提供一段 YAK 语言的代码（这是一门新语言，动态强类型）：

+ `+"```\n"+code+"\n"+"```"+`

## 任务

蒸馏 / 总结关键指标，帮助我构建一些训练集

`, map[string]any{
			"category":    "作为训练集数据来讲，分类一下这段代码的用途",
			"description": "分析和理解这段代码使用的场景，描述这段代码的功能",
			"difficulty":  "描述一下这段代码的难度",
			"tags":        "为这段代码打上标记，尽量简短",
			"code":        code,
		}, aispec.WithDebugStream(true))
		if err != nil {
			log.Errorf("ai analyze code failed: %v", err)
			writer.WriteHeader(500)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(200)
		_ = json.NewEncoder(writer).Encode(results)
		return
	})
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
