package vulinbox

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/orderedmap"

	"net/http"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed html/ai-knowledge-base-practice.html
var aiKbPractice []byte

//go:embed static/sample-yak-tutorial-data.txt
var sampleData string

type aiKbPracticeCtx struct {
	Ctx     context.Context
	Cancel  context.CancelFunc
	TextBuf *orderedmap.OrderedMap
}

func (r *VulinServer) registerAIKBPractice() {
	var router = r.router

	ctxMap := sync.Map{}
	createCtx := func() (context.Context, string) {
		id := uuid.New().String()
		ctx, cancel := context.WithCancel(context.Background())
		ctxMap.Store(id, aiKbPracticeCtx{
			Ctx:     ctx,
			Cancel:  cancel,
			TextBuf: orderedmap.NewOrderMap(nil, nil, false),
		})
		return ctx, id
	}
	removeCtx := func(id string) {
		ctx, ok := ctxMap.Load(id)
		if !ok {
			return
		}
		ctx.(aiKbPracticeCtx).Cancel()
		ctxMap.Delete(id)
	}
	getCtx := func(id string) (aiKbPracticeCtx, bool) {
		ctx, ok := ctxMap.Load(id)
		if !ok {
			return aiKbPracticeCtx{}, false
		}
		return ctx.(aiKbPracticeCtx), true
	}

	router.HandleFunc("/_/cancel-ai-practice", func(writer http.ResponseWriter, request *http.Request) {
		id := request.URL.Query().Get("id")
		if id == "" {
			writer.WriteHeader(500)
			return
		}
		removeCtx(id)
		writer.Write([]byte("ok"))
	})

	router.HandleFunc("/_/ai-practice-result", func(writer http.ResponseWriter, request *http.Request) {
		id := request.URL.Query().Get("id")
		if id == "" {
			writer.WriteHeader(500)
			return
		}
		aiCtx, ok := getCtx(id)
		if !ok {
			writer.WriteHeader(500)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		var result = map[string]any{}
		aiCtx.TextBuf.ForEach(func(key string, value any) {
			result[key] = value
		})
		writer.Write(utils.Jsonify(result))
	})

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

		apiKey := utils.MapGetStringByManyFields(data, "apiKey", "api_key", "api-key")
		if apiKey == "" {
			writer.WriteHeader(500)
			writer.Write([]byte("apiKey is required"))
			return
		}

		code := utils.MapGetStringByManyFields(data, "codeContent", "codeText", "code")
		ctx, ctxId := createCtx()
		results, err := ai.StructuredStream(
			"我的目标是根据一段代码，生成他的训练材料和教程，我给你这段代码，你帮我合理扩展他的教程: \n\n<code>\n"+
				code+"\n"+"<code>\n\n"+sampleData, aispec.WithDebugStream(true),
			aispec.WithAPIKey(apiKey), aispec.WithType("yaklang-writer"),
			aispec.WithContext(ctx),
		)
		if err != nil {
			log.Errorf("ai analyze code failed: %v", err)
			writer.WriteHeader(500)
			return
		}
		aiCtx, ok := getCtx(ctxId)
		if !ok {
			writer.WriteHeader(500)
			return
		}
		for result := range results {
			log.Infof("ai analyze code result: %v", utils.ShrinkString(result.DataRaw, 200))
			originTextRaw, ok := aiCtx.TextBuf.Get(result.OutputNodeId)
			var originText = fmt.Sprint(originTextRaw)
			if !ok {
				originText = result.OutputText
			} else {
				originText += result.OutputText
			}
			aiCtx.TextBuf.Set(result.OutputNodeId, originText)

			if _, err := writer.Write([]byte(string(utils.Jsonify(map[string]any{
				"context_id":     ctxId,
				"output_node_id": result.OutputNodeId,
				"output_text":    result.OutputText,
				"output_reason":  result.OutputReason,
			})) + "\r\n")); err != nil {
				log.Errorf("write response failed: %v", err)
				removeCtx(ctxId)
				return
			}
			utils.FlushWriter(writer)
		}
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
