package sfweb

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"

	"github.com/gorilla/websocket"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

var chatTemplate *template.Template

func init() {
	var err error
	chatTemplate, err = template.New("chat_template").Parse(`"""
{{.Content}}
"""

你是一名漏洞与代码分析专家，擅长分析{{.Lang}}语言。我需要你：
1.解析上述扫描结果JSON文件，识别并提取与风险相关的信息
2. 判断这个规则扫描的风险是否存在，你需要严格回答关于此规则的内容，不要擅自对其进行风险判断
- 如果风险存在，则根据结果解释该风险，并提出修复方案
- 如果风险不存在，则解释该风险为何不存在

请以下述模板以中文进行回答：
# 规则信息
- 规则名称： <规则名称>
- 规则描述： <规则描述>
- 代码片段： <代码片段>

# 风险分析
- 风险是否存在： <只能填是或否，若否>
<若风险不存在，则后续不需要再回答>
- 风险： <根据代码片段与规则描述对风险进行解释与分析>

# 风险修复建议
<风险修复建议>`)
	if err != nil {
		panic(err)
	}
}

type SyntaxFlowAIAnalysisRequest struct {
	Lang     string `json:"lang"`
	VarName  string `json:"var_name"`
	ResultID int64  `json:"result_id"`
}

type SyntaxFlowAIAnalysisResponse struct {
	Message string `json:"message"`
	Error   string `json:"error"`
}

type SyntaxFlowAIAnalysisWriter struct {
	conn *websocket.Conn
}

func NewSyntaxFlowAIAnalysisWriter(conn *websocket.Conn) *SyntaxFlowAIAnalysisWriter {
	return &SyntaxFlowAIAnalysisWriter{conn: conn}
}

func (w *SyntaxFlowAIAnalysisWriter) Write(p []byte) (n int, err error) {
	err = WriteWebsocketJSON(w.conn, &SyntaxFlowAIAnalysisResponse{
		Message: string(p),
	})
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

var _ io.Writer = (*SyntaxFlowAIAnalysisWriter)(nil)

func (s *SyntaxFlowWebServer) registerAIAnalysisRoute() {
	s.router.HandleFunc("/ai_analysis", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			writeErrorJson(w, err)
			return
		}
		defer conn.Close()

		var req SyntaxFlowAIAnalysisRequest
		if err = ReadWebsocketJSON(conn, &req); err != nil {
			WriteWebsocketJSON(conn, &SyntaxFlowScanResponse{
				Error: fmt.Sprintf("invalid request: %v", err),
			})
			SfWebLogger.Errorf("unmarshal request failed: %v", err)
			return
		}
		result, err := ssaapi.LoadResultByID(uint(req.ResultID))
		if err != nil {
			WriteWebsocketJSON(conn, &SyntaxFlowAIAnalysisResponse{
				Error: fmt.Sprintf("load result error: %v", err),
			})
			SfWebLogger.Errorf("load result error: %v", err)
			return
		}

		client := ai.ChatGLM(aispec.WithModel("glm-4-flash"), aispec.WithAPIKey(s.config.ChatGLMAPIKey), aispec.WithHTTPErrorHandler(func(err error) {
			WriteWebsocketJSON(conn, &SyntaxFlowAIAnalysisResponse{
				Error: fmt.Sprintf("send http packet for chat error: %v", err),
			})
		}))

		var promptBuilder strings.Builder
		err = chatTemplate.Execute(&promptBuilder, map[string]string{
			"Content": result.DumpValuesJson(req.VarName),
			"Lang":    req.Lang,
		})
		if err != nil {
			WriteWebsocketJSON(conn, &SyntaxFlowAIAnalysisResponse{
				Error: fmt.Sprintf("execute template error: %v", err),
			})
			SfWebLogger.Errorf("execute template error: %v", err)
			return
		}
		SfWebLogger.Debugf("chat prompt: %s", promptBuilder.String())

		reader, err := client.ChatStream(promptBuilder.String())
		if err != nil {
			WriteWebsocketJSON(conn, &SyntaxFlowAIAnalysisResponse{
				Error: fmt.Sprintf("chat error: %v", err),
			})
			SfWebLogger.Errorf("chat error: %v", err)
			return
		}
		_, err = io.Copy(NewSyntaxFlowAIAnalysisWriter(conn), reader)
		if err != nil {
			SfWebLogger.Errorf("copy error: %v", err)
		}
	}).Name("ai analysis")
}
