package yakgrpc

import (
	"context"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/yakdocument"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/davecgh/go-spew/spew"
)

var (
	completionJsonCd  = utils.NewCoolDown(5 * time.Second)
	completionJsonRaw []byte
)

// sessionRequest 表示某个会话的请求信息
type sessionRequest struct {
	requestID  uint64
	cancelFunc context.CancelFunc
}

// staticAnalyzeManager 管理静态分析请求，按 SessionID 分组
type staticAnalyzeManager struct {
	mu              sync.Mutex
	globalRequestID uint64                     // 全局请求ID计数器
	sessions        map[string]*sessionRequest // SessionID -> 当前请求
}

var analyzeManager = &staticAnalyzeManager{
	globalRequestID: 0,
	sessions:        make(map[string]*sessionRequest),
}

// registerRequest 为指定 SessionID 注册新请求，取消该 Session 的旧请求
func (m *staticAnalyzeManager) registerRequest(sessionID string, cancel context.CancelFunc) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.globalRequestID++
	requestID := m.globalRequestID

	if oldReq, exists := m.sessions[sessionID]; exists {
		log.Infof("[StaticAnalyze] Session '%s': Cancelling old request (ID: %d)", sessionID, oldReq.requestID)
		oldReq.cancelFunc()
	}

	m.sessions[sessionID] = &sessionRequest{
		requestID:  requestID,
		cancelFunc: cancel,
	}

	log.Infof("[StaticAnalyze] Session '%s': Registered new request (ID: %d), total sessions: %d",
		sessionID, requestID, len(m.sessions))
	return requestID
}

// unregisterRequest 注销指定 Session 的请求
func (m *staticAnalyzeManager) unregisterRequest(sessionID string, requestID uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, exists := m.sessions[sessionID]
	if !exists {
		log.Infof("[StaticAnalyze] Session '%s': Request %d already unregistered", sessionID, requestID)
		return
	}

	if req.requestID == requestID {
		delete(m.sessions, sessionID)
		log.Infof("[StaticAnalyze] Session '%s': Unregistered request (ID: %d), remaining sessions: %d",
			sessionID, requestID, len(m.sessions))
	} else {
		log.Infof("[StaticAnalyze] Session '%s': Skipped unregister for old request (ID: %d, current: %d)",
			sessionID, requestID, req.requestID)
	}
}

func (s *Server) GetYakVMBuildInMethodCompletion(
	ctx context.Context,
	req *ypb.GetYakVMBuildInMethodCompletionRequest,
) (*ypb.GetYakVMBuildInMethodCompletionResponse, error) {
	if !yaklang.IsNew() {
		return &ypb.GetYakVMBuildInMethodCompletionResponse{}, nil
	}
	var sug []*ypb.MethodSuggestion
	stringBuildin := yakvm.GetStringBuildInMethod()
	if stringBuildin != nil && len(stringBuildin) > 0 {
		suggestion := make([]*ypb.SuggestionDescription, len(stringBuildin))
		index := 0
		for methodName, method := range stringBuildin {
			if ret, _ := method.VSCodeSnippets(); ret == "" {
				spew.Dump(method)
				continue
			}
			snippets, verbose := method.VSCodeSnippets()
			suggestion[index] = &ypb.SuggestionDescription{
				Label:             methodName,
				Description:       method.Description,
				InsertText:        snippets,
				DefinitionVerbose: verbose,
			}
			index++
		}
		sug = append(sug, &ypb.MethodSuggestion{
			FuzzKeywords: []string{
				"str", "host", "name", "word", "payload",
				"raw", "bytes", "packet", "packets",
			},
			ExactKeywords: []string{"s", "ss", "a", "b", "abc"},
			Suggestions:   suggestion,
			Verbose:       "(string)",
		})
	}

	if ret := yakvm.GetSliceBuildInMethod(); ret != nil && len(ret) > 0 {
		var descs []*ypb.SuggestionDescription
		for name, method := range ret {
			if s, _ := method.VSCodeSnippets(); s == "" {
				spew.Dump(method)
				continue
			}
			s, v := method.VSCodeSnippets()
			descs = append(descs, &ypb.SuggestionDescription{
				Label:             name,
				Description:       method.Description,
				InsertText:        s,
				DefinitionVerbose: v,
			})
		}
		sug = append(sug, &ypb.MethodSuggestion{
			FuzzKeywords: []string{
				"list", "slice", "all", "li",
				"raw", "names", "passwords", "payloads",
				"usernames", "dict", "ports", "hosts",
				"result", "numbers",
			},
			ExactKeywords: []string{"ll", "l", "a", "b", "li", "abc"},
			Suggestions:   descs,
			Verbose:       `(list)`,
		})
	}

	if ret := yakvm.GetMapBuildInMethod(); ret != nil && len(ret) > 0 {
		var descs []*ypb.SuggestionDescription
		for name, method := range ret {
			if s, _ := method.VSCodeSnippets(); s == "" {
				spew.Dump(method)
				continue
			}
			s, v := method.VSCodeSnippets()
			descs = append(descs, &ypb.SuggestionDescription{
				Label:             name,
				Description:       method.Description,
				InsertText:        s,
				DefinitionVerbose: v,
			})
		}
		sug = append(sug, &ypb.MethodSuggestion{
			FuzzKeywords: []string{
				"pair", "result", "raw", "map",
				"dict",
			},
			ExactKeywords: []string{"m", "dict", "d", "r"},
			Suggestions:   descs,
			Verbose:       `(map)`,
		})
	}

	return &ypb.GetYakVMBuildInMethodCompletionResponse{
		Suggestions: sug,
	}, nil
}

func (s *Server) GetYakitCompletionRaw(ctx context.Context, _ *ypb.Empty) (*ypb.YakitCompletionRawResponse, error) {
	if completionJsonRaw != nil {
		return &ypb.YakitCompletionRawResponse{RawJson: completionJsonRaw}, nil
	}

	completionJsonCd.Do(func() {
		libs := yak.EngineToLibDocuments(yaklang.New())
		completionJsonRaw, _ = yakdocument.LibDocsToCompletionJsonShort(libs...)
	})
	return &ypb.YakitCompletionRawResponse{RawJson: completionJsonRaw}, nil
}

func (s *Server) StaticAnalyzeError(ctx context.Context, r *ypb.StaticAnalyzeErrorRequest) (*ypb.StaticAnalyzeErrorResponse, error) {
	code := string(r.GetCode())
	pluginType := r.GetPluginType()
	sessionID := r.GetSessionID()

	if sessionID == "" {
		sessionID = "default"
		log.Warnf("[StaticAnalyze] No SessionID provided, using default session")
	}

	analyzeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	requestID := analyzeManager.registerRequest(sessionID, cancel)
	defer analyzeManager.unregisterRequest(sessionID, requestID)

	log.Infof("[StaticAnalyze] Session '%s', Request %d started (code length: %d, type: %s)",
		sessionID, requestID, len(code), pluginType)

	select {
	case <-analyzeCtx.Done():
		log.Infof("[StaticAnalyze] Session '%s', Request %d cancelled before start", sessionID, requestID)
		return &ypb.StaticAnalyzeErrorResponse{Result: nil}, nil
	default:
	}

	startTime := time.Now()
	tmpRes := yak.StaticAnalyze(code, yak.WithStaticAnalyzePluginType(pluginType), yak.WithStaticAnalyzeContext(analyzeCtx))
	duration := time.Since(startTime)

	select {
	case <-analyzeCtx.Done():
		log.Infof("[StaticAnalyze] Session '%s', Request %d cancelled after %v", sessionID, requestID, duration)
		return &ypb.StaticAnalyzeErrorResponse{Result: nil}, nil
	default:
	}

	log.Infof("[StaticAnalyze] Session '%s', Request %d completed successfully in %v, found %d issues",
		sessionID, requestID, duration, len(tmpRes))

	es := lo.Map(tmpRes, func(i *result.StaticAnalyzeResult, _ int) *ypb.StaticAnalyzeErrorResult {
		return &ypb.StaticAnalyzeErrorResult{
			Message:         []byte(i.Message),
			StartLineNumber: i.StartLineNumber,
			EndLineNumber:   i.EndLineNumber,
			StartColumn:     i.StartColumn,
			EndColumn:       i.EndColumn,
			// RawMessage:      []byte{},
			Severity: string(i.Severity),
			Tag:      string(i.Tag),
		}
	})
	return &ypb.StaticAnalyzeErrorResponse{Result: es}, nil
}
