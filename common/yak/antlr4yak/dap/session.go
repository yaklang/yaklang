package dap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/google/go-dap"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
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

	sendingMu sync.Mutex

	// stopDebug is used to notify long-running handlers to stop processing.
	stopMu sync.Mutex

	// wg
	LaunchWg sync.WaitGroup

	// variablesMap  ref -> dap.Variable
	variablesMap map[int]dap.Variable
}

func NewDebugSession(conn net.Conn, config *DAPServerConfig) *DebugSession {
	return &DebugSession{
		config:       config,
		conn:         conn,
		rw:           bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn)),
		LaunchWg:     sync.WaitGroup{},
		variablesMap: make(map[int]dap.Variable),
	}
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
	response.Response = *newResponse(request.Request)
	response.Body.SupportsEvaluateForHovers = true        // 鼠标悬停时是否支持求值
	response.Body.SupportsConditionalBreakpoints = true   // 条件断点
	response.Body.SupportsConfigurationDoneRequest = true // 是否支持检测配置是否完成的请求,如果支持,则客户端发送一个SupportsConfigurationDoneRequest请求,而适配器会在调试会话的配置已完成时返回configurationDone响应,告诉客户端可以开始执行调试操作（如运行、单步执行等）
	response.Body.SupportsDataBreakpoints = false         // 某块内存(变量)被读写时触发的断点
	response.Body.SupportsStepInTargetsRequest = true     // 支持步入
	response.Body.SupportsDisassembleRequest = true       // 是否支持反汇编请求(输出opcode)
	response.Body.SupportTerminateDebuggee = true         // 在调试器终止时是否支持终止调试进程

	response.Body.SupportsFunctionBreakpoints = false        // 函数断点(可以考虑支持)
	response.Body.SupportsHitConditionalBreakpoints = false  // 在触发条件断点时到达断点但不满足条件的次数(可以考虑支持)
	response.Body.SupportsBreakpointLocationsRequest = false // 是否支持客户端向调试适配器查询特定源代码文件中可用的断点位置(可以考虑支持)
	response.Body.SupportsSetVariable = true                 // 支持调试器设置变量的新值(可以考虑支持)
	response.Body.SupportsSetExpression = true               // 是否支持设置表达式的新值(可以考虑支持)
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
	args := defaultArgs
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
	ds.send(&dap.LaunchResponse{Response: *newResponse(request.Request)})

	// 等待launch
	ds.LaunchWg.Add(1)
	go ds.RunProgramInDebugMode(request, !args.NoDebug, args.Program, args.Args)
}

func (ds *DebugSession) onAttachRequest(request *dap.AttachRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onDisconnectRequest(request *dap.DisconnectRequest) {
	restart := false
	args := request.Arguments
	if args != nil {
		restart = args.Restart
	}
	if !restart {
		defer ds.config.triggerServerStop()
	}

	ds.logToConsole("Detaching")
	ds.send(&dap.DisconnectResponse{Response: *newResponse(request.Request)})
	if !restart {
		ds.send(&dap.TerminatedEvent{Event: *newEvent("terminated")})
	} else {
		ds.debugger.SetRestart(true)
	}
	// ? unset debugger
	// ds.debugger = nil

}

func (ds *DebugSession) onTerminateRequest(request *dap.TerminateRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onRestartRequest(request *dap.RestartRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onSetBreakpointsRequest(request *dap.SetBreakpointsRequest) {
	// 等待launch完成
	ds.LaunchWg.Wait()

	debugger := ds.debugger
	// 等待init完成
	debugger.WaitInit()

	source := debugger.source

	response := &dap.SetBreakpointsResponse{Response: *newResponse(request.Request), Body: dap.SetBreakpointsResponseBody{}}

	// todo: 多文件调试,处理arguments.Source

	// todo: supportsLogPoints,处理arguments.Breakpoints.LogMessage

	response.Body.Breakpoints = make([]dap.Breakpoint, len(request.Arguments.Breakpoints))
	for i, b := range request.Arguments.Breakpoints {
		bp := &response.Body.Breakpoints[i]

		ref, err := debugger.SetBreakPoint(b.Line, b.Condition, b.HitCondition)
		bp.Id = ref
		bp.Line = b.Line
		bp.Source = &dap.Source{Path: source.AbsPath, Name: source.Name}
		bp.Verified = (err == nil)
		// todo: 当存在error的时候,是否需要设置breakpoint?
	}

	ds.send(response)
}

func (ds *DebugSession) onSetFunctionBreakpointsRequest(request *dap.SetFunctionBreakpointsRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

// Unlike what DAP documentation claims, this request is always sent
// even though we specified no filters at initialization. Handle as no-op.
func (ds *DebugSession) onSetExceptionBreakpointsRequest(request *dap.SetExceptionBreakpointsRequest) {
	ds.send(&dap.SetExceptionBreakpointsResponse{Response: *newResponse(request.Request)})
}

func (ds *DebugSession) onConfigurationDoneRequest(request *dap.ConfigurationDoneRequest) {

	// 如果stopOnEntry,则在入口时回调
	if ds.launchConfig.StopOnEntry {
		e := &dap.StoppedEvent{
			Event: *newEvent("stopped"),
			Body:  dap.StoppedEventBody{Reason: "entry", ThreadId: 0, AllThreadsStopped: true},
		}
		ds.send(e)

	} else {
		// 等待launch完成
		ds.LaunchWg.Wait()
		// 等待调试器初始化完成
		ds.debugger.WaitInit()

		ds.debugger.Continue()
	}
	ds.logToConsole(fmt.Sprintf("Yak version: %s\nType 'dbg help' for help info.\n", consts.GetYakVersion()))

	ds.send(&dap.ConfigurationDoneResponse{Response: *newResponse(request.Request)})
}

func (ds *DebugSession) onContinueRequest(request *dap.ContinueRequest) {
	// 等待launch完成
	ds.LaunchWg.Wait()
	// 等待调试器初始化完成
	ds.debugger.WaitInit()

	// ? 不支持单个线程继续运行,整个程序继续执行
	// ? 不需要处理request.threadId
	ds.debugger.Continue()

	ds.send(&dap.ContinueResponse{Response: *newResponse(request.Request), Body: dap.ContinueResponseBody{AllThreadsContinued: true}})
}

func (ds *DebugSession) onNextRequest(request *dap.NextRequest) {
	// 等待程序启动
	ds.WaitProgramStart()
	ds.sendStepResponse(request.Arguments.ThreadId, &dap.NextResponse{Response: *newResponse(request.Request)})
	ds.debugger.StepNext()
}

func (ds *DebugSession) onStepInRequest(request *dap.StepInRequest) {
	// 等待程序启动
	ds.WaitProgramStart()
	ds.sendStepResponse(request.Arguments.ThreadId, &dap.StepInResponse{Response: *newResponse(request.Request)})
	ds.debugger.StepIn()
}

func (ds *DebugSession) onStepOutRequest(request *dap.StepOutRequest) {
	// 等待程序启动
	ds.WaitProgramStart()

	ds.sendStepResponse(request.Arguments.ThreadId, &dap.StepOutResponse{Response: *newResponse(request.Request)})
	err := ds.debugger.StepOut()
	if err != nil {
		ds.sendErrorResponse(request.Request, UnableToHalt, "Unable to halt execution", err.Error())
		return
	}
}

func (ds *DebugSession) onStepBackRequest(request *dap.StepBackRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onReverseContinueRequest(request *dap.ReverseContinueRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onRestartFrameRequest(request *dap.RestartFrameRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onGotoRequest(request *dap.GotoRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onPauseRequest(request *dap.PauseRequest) {
	// 等待程序启动
	ds.WaitProgramStart()
	err := ds.debugger.Halt()
	if err != nil {
		ds.sendErrorResponse(request.Request, UnableToHalt, "Unable to halt execution", err.Error())
		return
	}
	ds.send(&dap.PauseResponse{Response: *newResponse(request.Request)})
}

func (ds *DebugSession) onStackTraceRequest(request *dap.StackTraceRequest) {
	// 等待程序启动
	ds.WaitProgramStart()

	var (
		frames      []yakvm.StackTrace
		stackFrames []dap.StackFrame
	)

	threadID := request.Arguments.ThreadId
	start := request.Arguments.StartFrame
	if start < 0 {
		start = 0
	}
	levels := ds.launchConfig.StackTraceDepth
	if request.Arguments.Levels > 0 {
		levels = request.Arguments.Levels
	}

	found := false
	total := 0

	for _, stackTraces := range ds.debugger.GetStackTraces() {
		if stackTraces.ThreadID == threadID {
			found = true
			frames = stackTraces.StackTraces
			total = len(frames)

			for i := 0; i < levels && start+i < total; i++ {
				stackTrace := frames[start+i]
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

			}
			break
		}
	}

	if !found {
		ds.sendErrorResponse(request.Request, UnableToProduceStackTrace, "Unable to produce stack trace", fmt.Sprintf("Can't found Goroutine %d stack trace", threadID))
		return
	}

	response := &dap.StackTraceResponse{}
	response.Response = *newResponse(request.Request)
	if len(frames) >= start+levels && frames[len(frames)-1].ID != 0 {
		// We don't know the exact number of available stack frames, so
		// add an arbitrary number so the client knows to request additional
		// frames.
		total += ds.launchConfig.StackTraceDepth
	}

	response.Body = dap.StackTraceResponseBody{
		StackFrames: stackFrames,
		TotalFrames: total,
	}
	ds.send(response)
}

func (ds *DebugSession) onScopesRequest(request *dap.ScopesRequest) {
	// 等待程序启动
	ds.WaitProgramStart()

	debugger := ds.debugger

	scopes := lo.MapToSlice(debugger.GetScopes(request.Arguments.FrameId), func(id int, scope *yakvm.Scope) dap.Scope {
		name := "Locals"
		if scope.IsRoot() {
			name = "Globals"
		}
		ref := ds.ConvertVariable(scope)

		return dap.Scope{
			Name:               name,
			VariablesReference: ref,              // 通过scope id来找到一组变量,用于VariablesRequest
			Expensive:          scope.Len() > 50, // 如果变量数量大于50,则认为是expensive
		}
	})
	// 按照VariablesReference排序,这样保证旧的local scope在前面
	sort.SliceStable(scopes, func(i, j int) bool {
		return scopes[i].VariablesReference < scopes[j].VariablesReference
	})

	// 修改重复的local scope名字,同时添加到ref中
	suffix := 0
	for i, scope := range scopes {
		if scope.Name == "Locals" {
			suffix++
			scopes[i].Name = fmt.Sprintf("Locals%d", suffix)
		}
	}

	response := &dap.ScopesResponse{Response: *newResponse(request.Request), Body: dap.ScopesResponseBody{Scopes: scopes}}
	ds.send(response)
}

func (ds *DebugSession) onVariablesRequest(request *dap.VariablesRequest) {
	// 等待程序启动
	ds.WaitProgramStart()

	debugger := ds.debugger

	ref := request.Arguments.VariablesReference
	filtered := request.Arguments.Filter
	start, count := request.Arguments.Start, request.Arguments.Count

	v, ok := debugger.GetVariablesByReference(ref)
	if !ok {
		ds.sendErrorResponse(request.Request, UnableToLookupVariable, "Unable to lookup variable", fmt.Sprintf("unknown reference %d", ref))
		return
	}

	children := []dap.Variable{}
	if filtered == "named" || filtered == "" {
		named := ds.namedToDAPVariables(v, start)
		children = append(children, named...)
	}

	if filtered == "indexed" || filtered == "" {
		indexed := ds.childrenToDAPVariables(v, start)
		children = append(children, indexed...)
	}

	if count > 0 {
		children = children[:count]
	}

	response := &dap.VariablesResponse{
		Response: *newResponse(request.Request),
		Body:     dap.VariablesResponseBody{Variables: children},
	}
	ds.send(response)
}

func (ds *DebugSession) onSetVariableRequest(request *dap.SetVariableRequest) {
	arg := request.Arguments
	ref := arg.VariablesReference
	frameID := ds.debugger.CurrentFrameID()

	v, ok := ds.GetConvertedVariable(ref)
	if !ok {
		ds.sendErrorResponse(request.Request, UnableToSetVariable, "Unable to lookup variable", fmt.Sprintf("unknown reference %d", ref))
		return
	}
	if scope, ok := v.(*yakvm.Scope); ok {
		id := scope.GetIdByName(arg.Name)
		if id == 0 {
			ds.sendErrorResponse(request.Request, UnableToSetVariable, "Unable to set variable", fmt.Sprintf("unknown reference %d", ref))
		}
		// use eval to get yakvm.Value
		value, err := ds.debugger.EvalExpression(arg.Value, frameID)
		if err != nil {
			ds.sendErrorResponse(request.Request, UnableToSetVariable, "Unable to set variable", err.Error())
			return
		}
		scope.NewValueByID(id, value)
		ds.debugger.ForceSetVariableRef(ref, value.Value)
	} else {
		// use eval to assign
		_, err := ds.debugger.EvalExpression(fmt.Sprintf("%s=%s", arg.Name, arg.Value), frameID)
		if err != nil {
			ds.sendErrorResponse(request.Request, UnableToSetVariable, "Unable to set variable", err.Error())
			return
		}
	}

	response := &dap.SetVariableResponse{Response: *newResponse(request.Request)}
	response.Body.Value = arg.Value
	ds.send(response)
}

func (ds *DebugSession) onSetExpressionRequest(request *dap.SetExpressionRequest) {
	arg := request.Arguments
	frameID := arg.FrameId

	// return value is not used
	_, err := ds.debugger.EvalExpression(fmt.Sprintf("%s=%s", arg.Expression, arg.Value), frameID)
	if err != nil {
		ds.sendErrorResponse(request.Request, UnableToSetVariable, "Unable to set variable", err.Error())
		return
	}
	response := &dap.SetExpressionResponse{Response: *newResponse(request.Request)}
	response.Body.Value = arg.Value
	ds.send(response)
}

func (ds *DebugSession) onSourceRequest(request *dap.SourceRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
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
	response.Response = *newResponse(request.Request)
	response.Body = dap.ThreadsResponseBody{Threads: threads}
	ds.send(response)

}

func (ds *DebugSession) onTerminateThreadsRequest(request *dap.TerminateThreadsRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onEvaluateRequest(request *dap.EvaluateRequest) {
	// 等待程序启动
	ds.WaitProgramStart()

	// todo: 处理context类型
	ctxt := request.Arguments.Context
	showErrorToUser := ctxt != "watch" && ctxt != "repl" && ctxt != "hover"
	expr := request.Arguments.Expression

	response := &dap.EvaluateResponse{Response: *newResponse(request.Request)}

	// 处理用户命令
	if ctxt == "repl" && strings.HasPrefix(expr, "dbg ") {
		cmd := strings.TrimPrefix(expr, "dbg ")
		result, err := ds.dbgCommand(cmd)
		if err != nil {
			ds.sendErrorResponseWithOpts(request.Request, UnableToRunDlvCommand, "Unable to run dbg command", err.Error(), showErrorToUser)
			return
		}
		response.Body = dap.EvaluateResponseBody{
			Result: result,
		}
	} else {
		value, err := ds.debugger.EvalExpression(expr, request.Arguments.FrameId)
		if err != nil {
			ds.sendErrorResponseWithOpts(request.Request, UnableToEvaluateExpression, "Unable to evaluate expression", err.Error(), showErrorToUser)
			return
		}

		response.Body = dap.EvaluateResponseBody{Result: AsDebugString(value.Value), Type: value.TypeVerbose}

		ref := ds.ConvertVariable(value.Value)
		response.Body.VariablesReference = ref
		response.Body.IndexedVariables = value.GetIndexedVariableCount()
		response.Body.NamedVariables = value.GetNamedVariableCount()
	}

	ds.send(response)
}

func (ds *DebugSession) onStepInTargetsRequest(request *dap.StepInTargetsRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onGotoTargetsRequest(request *dap.GotoTargetsRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onCompletionsRequest(request *dap.CompletionsRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onExceptionInfoRequest(request *dap.ExceptionInfoRequest) {
	// 等待launch完成
	ds.LaunchWg.Wait()
	// 等待程序启动
	ds.WaitProgramStart()

	// todo: 处理goroutineID
	goroutineID := request.Arguments.ThreadId

	var body dap.ExceptionInfoResponseBody

	p := ds.debugger.VMPanic()
	if p == nil {
		ds.sendErrorResponse(request.Request, UnableToGetExceptionInfo, "Unable to get exception info", fmt.Sprintf("could not find goroutine %d", goroutineID))
		return
	}
	body.ExceptionId = "panic"
	body.Description = fmt.Sprintf("%v", p.GetDataDescription())
	body.Details = &dap.ExceptionDetails{
		StackTrace: p.Error(),
	}

	ds.send(&dap.ExceptionInfoResponse{
		Response: *newResponse(request.Request),
		Body:     body,
	})
}

func (ds *DebugSession) onLoadedSourcesRequest(request *dap.LoadedSourcesRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onDataBreakpointInfoRequest(request *dap.DataBreakpointInfoRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onSetDataBreakpointsRequest(request *dap.SetDataBreakpointsRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onReadMemoryRequest(request *dap.ReadMemoryRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onDisassembleRequest(request *dap.DisassembleRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onCancelRequest(request *dap.CancelRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) onBreakpointLocationsRequest(request *dap.BreakpointLocationsRequest) {
	ds.sendNotYetImplementedErrorResponse(request.Request)
}

func (ds *DebugSession) sendStepResponse(threadId int, message dap.Message) {
	ds.send(&dap.ContinuedEvent{
		Event: *newEvent("continued"),
		Body: dap.ContinuedEventBody{
			ThreadId:            threadId,
			AllThreadsContinued: true,
		},
	})
	ds.send(message)
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

func (ds *DebugSession) namedToDAPVariables(i interface{}, start int) []dap.Variable {
	children := []dap.Variable{} // must return empty array, not null, if no children
	// metadata
	switch v := i.(type) {
	case *yakvm.Value:
		length := v.GetIndexedVariableCount()
		if length > 0 {
			children = append(children, dap.Variable{
				Name:         "len()",
				Value:        fmt.Sprintf("%d", length),
				Type:         "int",
				EvaluateName: fmt.Sprintf("len(%s)", v.Literal),
			})
		}

		if v.IsBytesOrRunes() {
			children = append(children, dap.Variable{
				Name:  "string()",
				Value: AsDebugString(v.Value),
				Type:  "string",
			})
		}
		return children
	case *yakvm.Scope:
		// ? scope没有metadata
		return children
	}

	refV := reflect.ValueOf(i)

	switch refV.Kind() {
	case reflect.Ptr:
		refV = refV.Elem()
		if refV.Kind() != reflect.Struct {
			return children
		}
		fallthrough
	case reflect.Struct:
		length := refV.NumField()
		children = make([]dap.Variable, length)
		refT := refV.Type()
		varname := ds.GetEvaluateName(i)

		for i := start; i < length; i++ {
			fieldName := refT.Field(i).Name

			value := refV.Field(i)
			iValue := value.Interface()
			ref := ds.ConvertVariable(iValue)

			vari := dap.Variable{
				Name:               fmt.Sprintf("%s", fieldName),
				EvaluateName:       fmt.Sprintf("%s.%s", varname, fieldName),
				Type:               value.Type().String(),
				Value:              AsDebugString(iValue),
				VariablesReference: ref,
				IndexedVariables:   yakvm.GetIndexedVariableCount(iValue),
				NamedVariables:     yakvm.GetNamedVariableCount(iValue),
			}

			// 保存到varibalesMap中
			ds.variablesMap[ref] = vari

			children[i] = vari
		}
	}

	return children
}

func (ds *DebugSession) childrenToDAPVariables(i interface{}, start int) []dap.Variable {
	children := []dap.Variable{} // must return empty array, not null, if no children
	var (
		handleValue interface{} = nil
	)

	switch v := i.(type) {
	case *yakvm.Scope:
		valuesMap := v.GetAllNameAndValueInScopes()

		// 排序保证顺序一致
		values := lo.MapToSlice(valuesMap, func(key string, value *yakvm.Value) *ScopeKV {
			return &ScopeKV{key, value}
		})
		sort.SliceStable(values, func(i, j int) bool {
			return values[i].Key < values[j].Key
		})

		children = make([]dap.Variable, len(valuesMap))
		index := 0
		for _, kv := range values {
			name, value := kv.Key, kv.Value

			ref := ds.ConvertVariable(value.Value)

			indexed := value.GetIndexedVariableCount()
			named := value.GetNamedVariableCount()
			vari := dap.Variable{
				Name:               name,
				EvaluateName:       name,
				Value:              AsDebugString(value.Value),
				Type:               value.TypeStr(),
				VariablesReference: ref,
				IndexedVariables:   indexed,
				NamedVariables:     named,
			}

			// 保存到varibalesMap中
			ds.variablesMap[ref] = vari

			children[index] = vari

			index++
		}
	case *yakvm.Value:
		handleValue = v.Value
	default:
		handleValue = v
	}

	if handleValue != nil {
		refV := reflect.ValueOf(handleValue)
		varname := ds.GetEvaluateName(handleValue)

		switch refV.Kind() {
		case reflect.Map:
			keys := refV.MapKeys()
			mapKeys := lo.Map(keys, func(item reflect.Value, index int) *MapKey {
				iKey := item.Interface()
				return &MapKey{item, iKey, AsDebugString(iKey)}
			})
			// 排序保证顺序一致
			sort.SliceStable(mapKeys, func(i, j int) bool {
				return mapKeys[i].KeyStr < mapKeys[j].KeyStr
			})

			cap := (len(keys) - start) * 2
			if cap < 0 {
				cap = 0
			}
			children = make([]dap.Variable, 0, cap)
			for i := start; i < len(mapKeys); i++ {
				mKey := mapKeys[i]
				key := mKey.Key
				iKey := mKey.IKey
				keyRef := ds.ConvertVariable(iKey)
				keyStr := mKey.KeyStr

				keyVar := dap.Variable{
					Name:               fmt.Sprintf("[key %d]", start+i),
					Type:               key.Type().String(),
					Value:              keyStr,
					VariablesReference: keyRef,
					IndexedVariables:   yakvm.GetIndexedVariableCount(iKey),
					NamedVariables:     yakvm.GetNamedVariableCount(iKey),
				}

				value := refV.MapIndex(key)
				iValue := value.Interface()
				valueRef := ds.ConvertVariable(iValue)
				valueStr := AsDebugString(iValue)

				valueVar := dap.Variable{
					Name:               fmt.Sprintf("[value %d]", start+i),
					EvaluateName:       fmt.Sprintf("%s[%s]", varname, keyStr),
					Type:               value.Type().String(),
					Value:              valueStr,
					VariablesReference: valueRef,
					IndexedVariables:   yakvm.GetIndexedVariableCount(iValue),
					NamedVariables:     yakvm.GetNamedVariableCount(iValue),
				}

				// 保存到varibalesMap中
				ds.variablesMap[keyRef] = keyVar
				ds.variablesMap[valueRef] = valueVar

				children = append(children, keyVar, valueVar)
			}
		case reflect.Array, reflect.Slice:
			length := refV.Len()
			children = make([]dap.Variable, length)
			for i := start; i < length; i++ {
				idx := start + i
				value := refV.Index(i)
				iValue := value.Interface()
				ref := ds.ConvertVariable(iValue)

				vari := dap.Variable{
					Name:               fmt.Sprintf("[%d]", idx),
					EvaluateName:       fmt.Sprintf("%s[%d]", varname, idx),
					Type:               value.Type().String(),
					Value:              AsDebugString(iValue),
					VariablesReference: ref,
					IndexedVariables:   yakvm.GetIndexedVariableCount(iValue),
					NamedVariables:     yakvm.GetNamedVariableCount(iValue),
				}

				// 保存到varibalesMap中
				ds.variablesMap[ref] = vari

				children[i] = vari
			}

		}
	}

	return children
}

// 获取handleValue对应的ref,然后在从保存的dap.Variable中拿到变量名
func (ds *DebugSession) GetEvaluateName(v interface{}) string {
	ref := ds.GetConvertedVariableRef(v)
	if v, ok := ds.variablesMap[ref]; ok {
		return v.EvaluateName
	}
	return ""
}

func (ds *DebugSession) GetConvertedVariable(ref int) (interface{}, bool) {
	debugger := ds.debugger
	return debugger.GetVariablesByReference(ref)
}

func (ds *DebugSession) GetConvertedVariableRef(v interface{}) int {
	i, ok := ds.debugger.GetVariablesReference(v)
	if !ok {
		return -1
	}
	return i
}

func (ds *DebugSession) ConvertVariable(v interface{}) int {
	if _, ok := v.(*yakvm.Function); ok {
		return 0
	}

	refV := reflect.ValueOf(v)
	switch refV.Kind() {
	case reflect.Ptr:
		refV = refV.Elem()
		if refV.Kind() != reflect.Struct {
			return 0
		}
	case reflect.Map, reflect.Array, reflect.Slice, reflect.Struct:
	default:
		return 0
	}

	debugger := ds.debugger
	i, ok := debugger.GetVariablesReference(v)
	if !ok {
		return debugger.AddVariableRef(v)
	}

	return i
}

func newResponse(request dap.Request) *dap.Response {
	return &dap.Response{
		ProtocolMessage: dap.ProtocolMessage{
			Seq:  0,
			Type: "response",
		},
		Command:    request.Command,
		RequestSeq: request.Seq,
		Success:    true,
	}
}

func newErrorResponse(requestSeq int, command string, message string) *dap.ErrorResponse {
	// todo: remove this function
	er := &dap.ErrorResponse{}
	er.Response = *newResponse(dap.Request{ProtocolMessage: dap.ProtocolMessage{Seq: requestSeq}, Command: command})
	er.Success = false
	er.Message = "unsupported"
	er.Body = dap.ErrorResponseBody{Error: &dap.ErrorMessage{Format: message, Id: 12345}}
	return er
}

func (ds *DebugSession) sendNotYetImplementedErrorResponse(request dap.Request) {
	ds.sendErrorResponse(request, NotYetImplemented, "Not yet implemented",
		fmt.Sprintf("cannot process %q request", request.Command))
}
