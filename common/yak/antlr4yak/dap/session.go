package dap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-dap"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

// Flags for convertVariableWithOpts option.
type convertVariableFlags uint8

const (
	UnsupportedCommand int = 9999
	InternalError      int = 8888
	NotYetImplemented  int = 7777

	// Where applicable and for consistency only,
	// values below are inspired the original vscode-go debug adaptor.

	FailedToLaunch             = 3000
	FailedToAttach             = 3001
	FailedToInitialize         = 3002
	UnableToSetBreakpoints     = 2002
	UnableToDisplayThreads     = 2003
	UnableToProduceStackTrace  = 2004
	UnableToListLocals         = 2005
	UnableToListArgs           = 2006
	UnableToListGlobals        = 2007
	UnableToLookupVariable     = 2008
	UnableToEvaluateExpression = 2009
	UnableToHalt               = 2010
	UnableToGetExceptionInfo   = 2011
	UnableToSetVariable        = 2012
	UnableToDisassemble        = 2013
	UnableToListRegisters      = 2014
	UnableToRunDlvCommand      = 2015

	// Add more codes as we support more requests

	NoDebugIsRunning  = 3000
	DebuggeeIsRunning = 4000
	DisconnectError   = 5000
)

const (
	skipRef convertVariableFlags = 1 << iota
	showFullValue
)

type DebugSession struct {
	// config
	config *DAPServerConfig

	// launch config
	launchConfig *LaunchConfig

	// debugger
	debugger *DAPDebugger

	// conn save raw connection
	conn net.Conn

	// rw is used to read requests and write events/responses
	rw *bufio.ReadWriter
	// sendQueue is used to capture messages from multiple request
	// processing goroutines while writing them to the client connection
	// from a single goroutine via sendFromQueue. We must keep track of
	// the multiple channel senders with a wait group to make sure we do
	// not close this channel prematurely. Closing this channel will signal
	// the sendFromQueue goroutine that it can exit.
	// sendQueue chan dap.Message
	// sendWg    sync.WaitGroup
	sendingMu sync.Mutex

	// stopDebug is used to notify long-running handlers to stop processing.
	stopMu sync.Mutex

	breakpointIDCounter int32

	// wg
	LaunchWg sync.WaitGroup
}

func (ds *DebugSession) send(message dap.Message) {
	jsonmsg, _ := json.Marshal(message)
	log.Debugf("[-> to client] %v", string(jsonmsg))

	ds.sendingMu.Lock()
	defer ds.sendingMu.Unlock()
	err := dap.WriteProtocolMessage(ds.conn, message)
	if err != nil {
		log.Debug(err)
	}
}

func (ds *DebugSession) handleRequest(request dap.Message) {
	jsonmsg, _ := json.Marshal(request)
	log.Debugf("[<- from client] %v", string(jsonmsg))

	if _, ok := request.(dap.RequestMessage); !ok {
		ds.sendInternalErrorResponse(request.GetSeq(), fmt.Sprintf("Unable to process non-request %#v\n", request))
		return
	}

	ds.dispatchRequest(request)
}

func (ds *DebugSession) Close() {
	ds.stopMu.Lock()
	defer ds.stopMu.Unlock()

	ds.conn.Close()
}

// request handlers
func (ds *DebugSession) dispatchRequest(request dap.Message) {
	switch request := request.(type) {
	case *dap.InitializeRequest:
		ds.onInitializeRequest(request)
	case *dap.LaunchRequest:
		ds.onLaunchRequest(request)
	case *dap.AttachRequest:
		ds.onAttachRequest(request)
	case *dap.DisconnectRequest:
		ds.onDisconnectRequest(request)
	case *dap.TerminateRequest:
		ds.onTerminateRequest(request)
	case *dap.RestartRequest:
		ds.onRestartRequest(request)
	case *dap.SetBreakpointsRequest:
		ds.onSetBreakpointsRequest(request)
	case *dap.SetFunctionBreakpointsRequest:
		ds.onSetFunctionBreakpointsRequest(request)
	case *dap.SetExceptionBreakpointsRequest:
		ds.onSetExceptionBreakpointsRequest(request)
	case *dap.ConfigurationDoneRequest:
		ds.onConfigurationDoneRequest(request)
	case *dap.ContinueRequest:
		ds.onContinueRequest(request)
	case *dap.NextRequest:
		ds.onNextRequest(request)
	case *dap.StepInRequest:
		ds.onStepInRequest(request)
	case *dap.StepOutRequest:
		ds.onStepOutRequest(request)
	case *dap.StepBackRequest:
		ds.onStepBackRequest(request)
	case *dap.ReverseContinueRequest:
		ds.onReverseContinueRequest(request)
	case *dap.RestartFrameRequest:
		ds.onRestartFrameRequest(request)
	case *dap.GotoRequest:
		ds.onGotoRequest(request)
	case *dap.PauseRequest:
		ds.onPauseRequest(request)
	case *dap.StackTraceRequest:
		ds.onStackTraceRequest(request)
	case *dap.ScopesRequest:
		ds.onScopesRequest(request)
	case *dap.VariablesRequest:
		ds.onVariablesRequest(request)
	case *dap.SetVariableRequest:
		ds.onSetVariableRequest(request)
	case *dap.SetExpressionRequest:
		ds.onSetExpressionRequest(request)
	case *dap.SourceRequest:
		ds.onSourceRequest(request)
	case *dap.ThreadsRequest:
		ds.onThreadsRequest(request)
	case *dap.TerminateThreadsRequest:
		ds.onTerminateThreadsRequest(request)
	case *dap.EvaluateRequest:
		ds.onEvaluateRequest(request)
	case *dap.StepInTargetsRequest:
		ds.onStepInTargetsRequest(request)
	case *dap.GotoTargetsRequest:
		ds.onGotoTargetsRequest(request)
	case *dap.CompletionsRequest:
		ds.onCompletionsRequest(request)
	case *dap.ExceptionInfoRequest:
		ds.onExceptionInfoRequest(request)
	case *dap.LoadedSourcesRequest:
		ds.onLoadedSourcesRequest(request)
	case *dap.DataBreakpointInfoRequest:
		ds.onDataBreakpointInfoRequest(request)
	case *dap.SetDataBreakpointsRequest:
		ds.onSetDataBreakpointsRequest(request)
	case *dap.ReadMemoryRequest:
		ds.onReadMemoryRequest(request)
	case *dap.DisassembleRequest:
		ds.onDisassembleRequest(request)
	case *dap.CancelRequest:
		ds.onCancelRequest(request)
	case *dap.BreakpointLocationsRequest:
		ds.onBreakpointLocationsRequest(request)
	default:
		log.Fatalf("Unable to process %#v", request)
	}
}

func (ds *DebugSession) onInitializeRequest(request *dap.InitializeRequest) {
	response := &dap.InitializeResponse{}
	response.Response = *newResponse(request.Seq, request.Command)
	response.Body.SupportsEvaluateForHovers = true        // 鼠标悬停时是否支持求值
	response.Body.SupportsConditionalBreakpoints = true   // 条件断点
	response.Body.SupportsConfigurationDoneRequest = true // 是否支持检测配置是否完成的请求,如果支持,则客户端发送一个SupportsConfigurationDoneRequest请求,而适配器会在调试会话的配置已完成时返回configurationDone响应,告诉客户端可以开始执行调试操作（如运行、单步执行等）
	response.Body.SupportsDataBreakpoints = true          // todo: 是否支持数据断点(即监视和控制特定变量的值，并在变量的值满足特定条件时暂停程序的执行)(未完全支持)
	response.Body.SupportsStepInTargetsRequest = true     // 支持步入
	response.Body.SupportsDisassembleRequest = true       // 是否支持反汇编请求(输出opcode)
	response.Body.SupportTerminateDebuggee = true         // 在调试器终止时是否支持终止调试进程

	response.Body.SupportsFunctionBreakpoints = false        // 函数断点(可以考虑支持)
	response.Body.SupportsHitConditionalBreakpoints = false  // 在触发条件断点时到达断点但不满足条件的次数(可以考虑支持)
	response.Body.SupportsBreakpointLocationsRequest = false // 是否支持客户端向调试适配器查询特定源代码文件中可用的断点位置(可以考虑支持)
	response.Body.SupportsSetVariable = false                // 支持调试器设置变量的新值(可以考虑支持)
	response.Body.SupportsSetExpression = false              // 是否支持设置表达式的新值(可以考虑支持)
	response.Body.SupportsLogPoints = false                  // 是否支持断点不暂停,而是在断点处输出信息(可以考虑支持)

	response.Body.ExceptionBreakpointFilters = []dap.ExceptionBreakpointsFilter{} // 异常断点的过滤器
	response.Body.SupportsStepBack = false                                        // 步退
	response.Body.SupportsRestartFrame = false                                    // 支持调试器重启帧
	response.Body.SupportsGotoTargetsRequest = false                              // 支持获取跳转信息，例如函数的定义，派生类实现
	response.Body.SupportsCompletionsRequest = false                              // 支持补全
	response.Body.CompletionTriggerCharacters = []string{}                        // 补全触发字符
	response.Body.SupportsModulesRequest = false                                  // 模块级别的调试支持
	response.Body.AdditionalModuleColumns = []dap.ColumnDescriptor{}              // 附加的模块信息
	response.Body.SupportedChecksumAlgorithms = []dap.ChecksumAlgorithm{}         // 支持的校验算法,用于校验文件完整性
	response.Body.SupportsRestartRequest = false                                  // 是否支持重启正在调试的请求,如果不支持则需要重新启动调试器
	response.Body.SupportsExceptionOptions = false                                // 是否支持自定义异常行为
	response.Body.SupportsValueFormattingOptions = false                          // 是否支持格式化堆栈跟踪请求,变量请求和执行请求
	response.Body.SupportsExceptionInfoRequest = false                            // 是否支持输出异常信息请求
	response.Body.SupportsDelayedStackTraceLoading = false                        // 是否支持延迟加载堆栈跟踪信息
	response.Body.SupportsLoadedSourcesRequest = false                            // 是否支持获取已加载的源代码列表请求,获取有关已加载源代码的信息，例如文件路径、调试信息等
	response.Body.SupportsTerminateThreadsRequest = false                         // 是否支持终止线程请求
	response.Body.SupportsTerminateRequest = false                                // 是否支持终止调试进程请求
	response.Body.SupportsReadMemoryRequest = false                               // 是否支持读取内存请求
	response.Body.SupportsCancelRequest = false                                   // 是否支持取消请求,取消请求用于1. 表示客户端不再对先前发出的特定请求产生的结果感兴趣 2. 取消进度序列

	// e := &dap.InitializedEvent{Event: *newEvent("initialized")}
	// ds.send(e)
	ds.send(response)
}

func (ds *DebugSession) WaitProgramStart() {
	ds.debugger.WaitProgramStart()
}

func (ds *DebugSession) onLaunchRequest(request *dap.LaunchRequest) {
	var args LaunchConfig
	if ds.debugger != nil {
		ds.sendShowUserErrorResponse(request.Request, FailedToLaunch, "Failed to launch",
			"debug session already in progress - use remote attach mode to connect to a server with an active debug session")
		return
	}

	// default mode
	if args.Mode == "" {
		args.Mode = "exec"
	}

	if err := unmarshalLaunchConfig(request.Arguments, &args); err != nil {
		ds.sendShowUserErrorResponse(request.Request,
			FailedToLaunch, "Failed to launch", fmt.Sprintf("invalid debug configuration - %v", err))
		log.Debugf("Parse launch config error: %v", pretty(args))
		return
	}

	// env
	for k, v := range args.Env {
		if v != nil {
			if err := os.Setenv(k, *v); err != nil {
				ds.sendShowUserErrorResponse(request.Request, FailedToLaunch, "Failed to launch", fmt.Sprintf("failed to setenv(%v) - %v", k, err))
				return
			}
		} else {
			if err := os.Unsetenv(k); err != nil {
				ds.sendShowUserErrorResponse(request.Request, FailedToLaunch, "Failed to launch", fmt.Sprintf("failed to unsetenv(%v) - %v", k, err))
				return
			}
		}
	}
	// cwd
	if args.Cwd != "" {
		os.Chdir(args.Cwd)
	}

	// save launch config
	ds.launchConfig = &args

	// todo: handle args.Mode, "debug" and "exec"

	ds.send(&dap.InitializedEvent{Event: *newEvent("initialized")})
	ds.send(&dap.LaunchResponse{Response: *newResponse(request.Seq, request.Command)})

	// 等待launch
	ds.LaunchWg.Add(1)
	go ds.RunProgramInDebugMode(!args.NoDebug, args.Program, args.Args)
}

func (ds *DebugSession) onAttachRequest(request *dap.AttachRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "AttachRequest is not yet supported"))
}

func (ds *DebugSession) onDisconnectRequest(request *dap.DisconnectRequest) {
	defer ds.config.triggerServerStop()
	// ? unset debugger
	// ds.debugger = nil

	ds.logToConsole("Detaching")
	ds.send(&dap.DisconnectResponse{Response: *newResponse(request.Seq, request.Command)})
	ds.send(&dap.TerminatedEvent{Event: *newEvent("terminated")})
}

func (ds *DebugSession) onTerminateRequest(request *dap.TerminateRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "TerminateRequest is not yet supported"))
}

func (ds *DebugSession) onRestartRequest(request *dap.RestartRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "RestartRequest is not yet supported"))
}

func (ds *DebugSession) onSetBreakpointsRequest(request *dap.SetBreakpointsRequest) {
	debugger := ds.debugger

	// 等待launch完成
	ds.LaunchWg.Wait()
	// 等待init完成
	debugger.WaitInit()

	source := debugger.source

	response := &dap.SetBreakpointsResponse{Response: *newResponse(request.Seq, request.Command), Body: dap.SetBreakpointsResponseBody{}}

	// todo: 多文件调试,处理arguments.Source

	// todo: supportsHitConditionalBreakpoints,处理arguments.Breakpoints.HitCondition
	// todo: supportsLogPoints,处理arguments.Breakpoints.LogMessage

	response.Body.Breakpoints = make([]dap.Breakpoint, len(request.Arguments.Breakpoints))
	for i, b := range request.Arguments.Breakpoints {
		bp := &response.Body.Breakpoints[i]

		atomic.AddInt32(&ds.breakpointIDCounter, 1)
		bp.Source = &dap.Source{Path: source.AbsPath, Name: source.Name}
		bp.Id = int(ds.breakpointIDCounter)
		bp.Line = b.Line

		err := debugger.SetBreakPoint(bp.Line, b.Condition, b.HitCondition)

		bp.Verified = (err == nil)
		// todo: 当存在error的时候,是否需要设置breakpoint?
	}

	ds.send(response)
}

func (ds *DebugSession) onSetFunctionBreakpointsRequest(request *dap.SetFunctionBreakpointsRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "SetFunctionBreakpointsRequest is not yet supported"))
}

// Unlike what DAP documentation claims, this request is always sent
// even though we specified no filters at initialization. Handle as no-op.
func (ds *DebugSession) onSetExceptionBreakpointsRequest(request *dap.SetExceptionBreakpointsRequest) {
	ds.send(&dap.SetExceptionBreakpointsResponse{Response: *newResponse(request.Seq, request.Command)})
}

func (ds *DebugSession) onConfigurationDoneRequest(request *dap.ConfigurationDoneRequest) {

	// 如果stopOnEtry,则在入口时回调
	if ds.launchConfig.StopOnEntry {
		e := &dap.StoppedEvent{
			Event: *newEvent("stopped"),
			Body:  dap.StoppedEventBody{Reason: "entry", ThreadId: 0, AllThreadsStopped: true},
		}
		ds.send(e)

		ds.logToConsole(fmt.Sprintf("Yak version: %s\nType 'help' for help info.\n", consts.GetYakVersion()))
	} else {
		// 等待调试器初始化完成
		ds.debugger.WaitInit()

		ds.debugger.Continue()
	}

	ds.send(&dap.ConfigurationDoneResponse{Response: *newResponse(request.Seq, request.Command)})
}

func (ds *DebugSession) onContinueRequest(request *dap.ContinueRequest) {
	// 等待调试器初始化完成
	ds.debugger.WaitInit()

	// ? 不支持单个线程继续运行,整个程序继续执行
	// ? 不需要处理request.threadId
	ds.debugger.Continue()

	ds.send(&dap.ContinueResponse{Response: *newResponse(request.Seq, request.Command), Body: dap.ContinueResponseBody{AllThreadsContinued: true}})
}

func (ds *DebugSession) onNextRequest(request *dap.NextRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "NextRequest is not yet supported"))
}

func (ds *DebugSession) onStepInRequest(request *dap.StepInRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "StepInRequest is not yet supported"))
}

func (ds *DebugSession) onStepOutRequest(request *dap.StepOutRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "StepOutRequest is not yet supported"))
}

func (ds *DebugSession) onStepBackRequest(request *dap.StepBackRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "StepBackRequest is not yet supported"))
}

func (ds *DebugSession) onReverseContinueRequest(request *dap.ReverseContinueRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "ReverseContinueRequest is not yet supported"))
}

func (ds *DebugSession) onRestartFrameRequest(request *dap.RestartFrameRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "RestartFrameRequest is not yet supported"))
}

func (ds *DebugSession) onGotoRequest(request *dap.GotoRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "GotoRequest is not yet supported"))
}

func (ds *DebugSession) onPauseRequest(request *dap.PauseRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "PauseRequest is not yet supported"))
}

func (ds *DebugSession) onStackTraceRequest(request *dap.StackTraceRequest) {
	// 等待程序启动
	ds.WaitProgramStart()

	var stackFrames []dap.StackFrame

	threadID := uint64(request.Arguments.ThreadId)
	startFrame := request.Arguments.StartFrame
	maxLevels := request.Arguments.Levels
	found := false

	for _, stackTraces := range ds.debugger.GetStackTraces() {
		if stackTraces.ThreadID == threadID {
			found = true
			for count := startFrame; count < len(stackTraces.StackTraces); count++ {
				stackTrace := stackTraces.StackTraces[count]
				source := *stackTrace.Source
				stackFrames = append(stackFrames, dap.StackFrame{
					Id:        stackTrace.ID, // stackTrace.ID 就是 frameId
					Name:      stackTrace.Name,
					Source:    &dap.Source{Name: filepath.Base(source), Path: source},
					Line:      stackTrace.Line,
					Column:    stackTrace.Column,
					EndLine:   stackTrace.EndLine,
					EndColumn: stackTrace.EndColumn,
				})
				// if maxLevels is 0, then return all stackFrames
				// otherwise, return maxLevels stackFrames
				if maxLevels > 0 && count >= maxLevels {
					break
				}
			}
			break
		}
	}

	if !found {
		ds.sendErrorResponse(request.Request, UnableToProduceStackTrace, "Unable to produce stack trace", fmt.Sprintf("Can't found Goroutine %d stack trace", threadID))
		return
	}

	response := &dap.StackTraceResponse{}
	response.Response = *newResponse(request.Seq, request.Command)
	response.Body = dap.StackTraceResponseBody{
		StackFrames: stackFrames,
		TotalFrames: len(stackFrames),
	}
	ds.send(response)
}

func (ds *DebugSession) onScopesRequest(request *dap.ScopesRequest) {
	response := &dap.ScopesResponse{}
	response.Response = *newResponse(request.Seq, request.Command)
	response.Body = dap.ScopesResponseBody{
		Scopes: []dap.Scope{
			{Name: "Local", VariablesReference: 1000, Expensive: false},
			{Name: "Global", VariablesReference: 1001, Expensive: true},
		},
	}
	ds.send(response)
}

func (ds *DebugSession) onVariablesRequest(request *dap.VariablesRequest) {
	select {
	case <-ds.config.stopped:
		return
	// simulate long-running processing to make this handler
	// respond to this request after the next request is received
	case <-time.After(100 * time.Millisecond):
		response := &dap.VariablesResponse{}
		response.Response = *newResponse(request.Seq, request.Command)
		response.Body = dap.VariablesResponseBody{
			Variables: []dap.Variable{{Name: "i", Value: "18434528", EvaluateName: "i", VariablesReference: 0}},
		}
		ds.send(response)
	}
}

func (ds *DebugSession) onSetVariableRequest(request *dap.SetVariableRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "setVariableRequest is not yet supported"))
}

func (ds *DebugSession) onSetExpressionRequest(request *dap.SetExpressionRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "SetExpressionRequest is not yet supported"))
}

func (ds *DebugSession) onSourceRequest(request *dap.SourceRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "SourceRequest is not yet supported"))
}

func (ds *DebugSession) onThreadsRequest(request *dap.ThreadsRequest) {
	// 需要等待程序启动
	ds.WaitProgramStart()

	var threads []dap.Thread
	// todo: update selected frame

	yakThreads := ds.debugger.GetThreads()
	threads = lo.Map(yakThreads, func(item *Thread, index int) dap.Thread {
		return dap.Thread{
			Id:   item.Id,
			Name: item.Name,
		}
	})

	response := &dap.ThreadsResponse{}
	response.Response = *newResponse(request.Seq, request.Command)
	response.Body = dap.ThreadsResponseBody{Threads: threads}
	ds.send(response)

}

func (ds *DebugSession) onTerminateThreadsRequest(request *dap.TerminateThreadsRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "TerminateRequest is not yet supported"))
}

func (ds *DebugSession) onEvaluateRequest(request *dap.EvaluateRequest) {
	// 等待程序启动
	ds.WaitProgramStart()

	// todo: 处理context类型
	ctxt := request.Arguments.Context
	showErrorToUser := ctxt != "watch" && ctxt != "repl" && ctxt != "hover"

	value, err := ds.debugger.EvalExpression(request.Arguments.Expression, request.Arguments.FrameId)
	if err != nil {
		ds.sendErrorResponseWithOpts(request.Request, UnableToEvaluateExpression, "Unable to evaluate expression", err.Error(), showErrorToUser)
		return
	}

	response := &dap.EvaluateResponse{Response: *newResponse(request.Seq, request.Command)}
	response.Body = dap.EvaluateResponseBody{Result: value.String(), Type: value.TypeVerbose}

	ref, ok := value.GetVarRef()
	if ok {
		response.Body.VariablesReference = ref
		response.Body.IndexedVariables = value.GetIndexedVariableCount()
		response.Body.NamedVariables = value.GetNamedVariableCount()
	}

	ds.send(response)
}

func (ds *DebugSession) onStepInTargetsRequest(request *dap.StepInTargetsRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "StepInTargetRequest is not yet supported"))
}

func (ds *DebugSession) onGotoTargetsRequest(request *dap.GotoTargetsRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "GotoTargetRequest is not yet supported"))
}

func (ds *DebugSession) onCompletionsRequest(request *dap.CompletionsRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "CompletionRequest is not yet supported"))
}

func (ds *DebugSession) onExceptionInfoRequest(request *dap.ExceptionInfoRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "ExceptionRequest is not yet supported"))
}

func (ds *DebugSession) onLoadedSourcesRequest(request *dap.LoadedSourcesRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "LoadedRequest is not yet supported"))
}

func (ds *DebugSession) onDataBreakpointInfoRequest(request *dap.DataBreakpointInfoRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "DataBreakpointInfoRequest is not yet supported"))
}

func (ds *DebugSession) onSetDataBreakpointsRequest(request *dap.SetDataBreakpointsRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "SetDataBreakpointsRequest is not yet supported"))
}

func (ds *DebugSession) onReadMemoryRequest(request *dap.ReadMemoryRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "ReadMemoryRequest is not yet supported"))
}

func (ds *DebugSession) onDisassembleRequest(request *dap.DisassembleRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "DisassembleRequest is not yet supported"))
}

func (ds *DebugSession) onCancelRequest(request *dap.CancelRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "CancelRequest is not yet supported"))
}

func (ds *DebugSession) onBreakpointLocationsRequest(request *dap.BreakpointLocationsRequest) {
	ds.send(newErrorResponse(request.Seq, request.Command, "BreakpointLocationsRequest is not yet supported"))
}

func (ds *DebugSession) sendErrorResponse(request dap.Request, id int, summary, details string) {
	ds.sendErrorResponseWithOpts(request, id, summary, details, false /*showUser*/)
}

func (ds *DebugSession) sendShowUserErrorResponse(request dap.Request, id int, summary, details string) {
	ds.sendErrorResponseWithOpts(request, id, summary, details, true /*showUser*/)
}

func (ds *DebugSession) sendErrorResponseWithOpts(request dap.Request, id int, summary, details string, showUser bool) {
	er := &dap.ErrorResponse{}
	er.Type = "response"
	er.Command = request.Command
	er.RequestSeq = request.Seq
	er.Success = false
	er.Message = summary
	er.Body.Error = &dap.ErrorMessage{
		Id:       id,
		Format:   fmt.Sprintf("%s: %s", summary, details),
		ShowUser: showUser,
	}
	log.Debug(er.Body.Error.Format)
	ds.send(er)
}

func (ds *DebugSession) logToConsole(msg string) {
	ds.send(&dap.OutputEvent{
		Event: *newEvent("output"),
		Body: dap.OutputEventBody{
			Output:   msg + "\n",
			Category: "console",
		}})
}

func newEvent(event string) *dap.Event {
	return &dap.Event{
		ProtocolMessage: dap.ProtocolMessage{
			Seq:  0,
			Type: "event",
		},
		Event: event,
	}
}

func (s *DebugSession) sendInternalErrorResponse(seq int, details string) {
	er := &dap.ErrorResponse{}
	er.Type = "response"
	er.RequestSeq = seq
	er.Success = false
	er.Message = "Internal Error"
	er.Body.Error = &dap.ErrorMessage{
		Id:     InternalError,
		Format: fmt.Sprintf("%s: %s", er.Message, details),
	}
	log.Debug(er.Body.Error.Format)
	s.send(er)
}

func newResponse(requestSeq int, command string) *dap.Response {
	return &dap.Response{
		ProtocolMessage: dap.ProtocolMessage{
			Seq:  0,
			Type: "response",
		},
		Command:    command,
		RequestSeq: requestSeq,
		Success:    true,
	}
}

func newErrorResponse(requestSeq int, command string, message string) *dap.ErrorResponse {
	er := &dap.ErrorResponse{}
	er.Response = *newResponse(requestSeq, command)
	er.Success = false
	er.Message = "unsupported"
	er.Body = dap.ErrorResponseBody{Error: &dap.ErrorMessage{Format: message, Id: 12345}}
	return er
}
