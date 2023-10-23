package yak

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/binx"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"path/filepath"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/authhack"
	"github.com/yaklang/yaklang/common/chaosmaker"
	"github.com/yaklang/yaklang/common/crawler"
	"github.com/yaklang/yaklang/common/crawlerx"
	"github.com/yaklang/yaklang/common/cve"
	"github.com/yaklang/yaklang/common/facades"
	"github.com/yaklang/yaklang/common/hids"
	"github.com/yaklang/yaklang/common/iiop"
	"github.com/yaklang/yaklang/common/ja3"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/openai"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/rpa"
	"github.com/yaklang/yaklang/common/sca"
	"github.com/yaklang/yaklang/common/simulator"
	"github.com/yaklang/yaklang/common/systemd"
	"github.com/yaklang/yaklang/common/t3"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/comparer"
	"github.com/yaklang/yaklang/common/utils/htmlquery"
	"github.com/yaklang/yaklang/common/xhtml"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yak/yaklang/lib/builtin"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yserx"
	"github.com/yaklang/yaklang/common/yso"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/tevino/abool"

	_ "github.com/yaklang/yaklang/common/yak/yaklang/lib/builtin" // 导入 builtin 包
)

var (
	CRYPTO_KEY_SIZE    = 16
	initYaklangLibOnce sync.Once
	naslExports        interface{}
)

func SetNaslExports(lib map[string]interface{}) {
	naslExports = lib
}

func init() {
	initYaklangLib()
}
func InitYaklangLib() {
	initYaklangLibOnce.Do(func() {
		initYaklangLib()
	})
}

type LibFuncWithFrameType func(*yakvm.Frame) interface{}

func initYaklangLib() {
	importGlobal := func(table map[string]interface{}) {
		for k, v := range table {
			yaklang.Import(k, v)
		}
	}
	importGlobal(yaklib.GlobalExport)
	importGlobal(builtin.YaklangBaseLib)
	importGlobal(GlobalEvalExports)
	//yaklang.Import("", yaklib.GlobalExport)
	yaklang.Import("yakit", yaklib.YakitExports)
	// 基础库
	yaklang.Import("str", yaklib.StringsExport)
	yaklang.Import("math", yaklib.MathExport)
	yaklang.Import("os", yaklib.SystemExports)
	yaklang.Import("file", yaklib.FileExport)
	yaklang.Import("re", yaklib.RegexpExport)
	yaklang.Import("re2", yaklib.RegexpExport)
	yaklang.Import("regen", yaklib.RegenExports)
	yaklang.Import("env", yaklib.EnvExports)
	//yaklang.Import("grok", yaklib.GrokExports)
	yaklang.Import("sync", yaklib.SyncExport)
	yaklang.Import("io", yaklib.IoExports)
	yaklang.Import("bufio", yaklib.BufioExport)
	yaklang.Import("context", yaklib.ContextExports)
	yaklang.Import("time", yaklib.TimeExports)
	yaklang.Import("timezone", yaklib.TimeZoneExports)
	yaklang.Import("codec", yaklib.CodecExports) //编码解码
	yaklang.Import("log", yaklib.LogExports)
	//yaklang.Import("net", yaklib.Ne)
	yaklang.Import("hids", hids.Exports)
	yaklang.Import("systemd", systemd.Exports)

	//yaklang.Import("geojson", yaklib.GeoJsonExports)
	yaklang.Import("mmdb", yaklib.MmdbExports)

	yaklang.Import("crawler", crawler.Exports)
	yaklang.Import("mitm", yaklib.MitmExports)
	yaklang.Import("tls", yaklib.TlsExports)

	// CLI 解析
	yaklang.Import("cli", yaklib.CliExports)

	// Fuzz 模糊测试
	yaklang.Import("fuzz", yaklib.FuzzExports)

	// xhtml xss检测基础工具
	yaklang.Import("xhtml", xhtml.Exports)

	// 基础 http 网络请求的库
	yaklang.Import("http", yaklib.HttpExports)
	yaklang.Import("httpserver", yaklib.HttpServeExports)
	yaklang.Import("httpool", yaklib.HttpPoolExports)

	yaklang.Import("dictutil", yaklib.DictUtilExports)

	// 漏洞工具的库
	yaklang.Import("tools", tools.Exports)

	// 基础网络io库
	yaklang.Import("tcp", yaklib.TcpExports)
	yaklang.Import("udp", yaklib.UDPExport)

	// SYN 扫描库
	yaklang.Import("synscan", tools.SynPortScanExports)
	yaklang.Import("finscan", tools.FinPortScanExports)

	// 指纹扫描库
	yaklang.Import("servicescan", tools.FingerprintScanExports)

	// 子域名扫描库
	yaklang.Import("subdomain", tools.SubDomainExports)

	// 执行系统命令的库
	yaklang.Import("exec", yaklib.ExecExports)

	// 爆破
	yaklang.Import("brute", tools.BruterExports)

	// dns 库
	yaklang.Import("dns", yaklib.DnsExports)

	// ping
	yaklang.Import("ping", yaklib.PingExports)

	// shodan / quake / fofa 库
	yaklang.Import("spacengine", yaklib.SpaceEngineExports)

	// json
	yaklang.Import("json", yaklib.JsonExports)

	// yaml
	yaklang.Import("yaml", yaklib.YamlExports)

	// eval
	//yaklang.Import("", GlobalEvalExporst)
	yaklang.Import("dyn", EvalExports)
	// nuclei
	yaklang.Import("nuclei", httptpl.Exports)
	yaklang.Import("nasl", naslExports)

	// jwt
	yaklang.Import("jwt", authhack.JWTExports)

	// java
	yaklang.Import("java", yserx.Exports)

	// poc
	yaklang.Import("poc", yaklib.PoCExports)
	yaklang.Import("csrf", yaklib.CSRFExports)
	yaklang.Import("risk", yaklib.RiskExports)
	yaklang.Import("report", yakit.ReportExports)
	yaklang.Import("dnslog", yaklib.DNSLogExports)

	// xpath for html
	yaklang.Import("xpath", htmlquery.Exports)

	// xml
	yaklang.Import("xml", yaklib.XMLExports)

	// hooks // 负责加载插件中的插件机制
	yaklang.Import("hook", HooksExports)

	// 辅助函数库
	yaklang.Import("x", yaklib.FunkExports)

	// java 反序列化生成
	yaklang.Import("yso", yso.Exports)
	yaklang.Import("facades", facades.FacadesExports)

	// t3反序列化利用
	yaklang.Import("t3", t3.Exports)
	yaklang.Import("iiop", iiop.Exports)
	yaklang.Import("js", yaklib.JSOttoExports)

	yaklang.Import("db", yaklib.DatabaseExports)

	// 判别
	yaklang.Import("judge", comparer.Exports)

	//
	yaklang.Import("smb", yaklib.SambaExports)
	yaklang.Import("ldap", yaklib.LdapExports)

	// zip 解压包
	yaklang.Import("zip", yaklib.ZipExports)

	// gzip 增加 gzip 压缩
	yaklang.Import("gzip", yaklib.GzipExports)

	// redis
	yaklang.Import("redis", yaklib.RedisExports)

	//common.rpa
	yaklang.Import("rpa", rpa.Exports)

	// rdp
	yaklang.Import("rdp", yaklib.RdpExports)

	// bot
	yaklang.Import("bot", yaklib.BotExports)

	// simulator
	yaklang.Import("simulator", simulator.Exports)

	//crawlerX
	yaklang.Import("crawlerx", crawlerx.CrawlerXExports)

	//CVE
	yaklang.Import("cve", cve.CVEExports)
	yaklang.Import("cwe", cve.CWEExports)

	// openai
	yaklang.Import("openai", openai.Exports)

	// suricata
	yaklang.Import("suricata", chaosmaker.ChaosMakerExports)
	yaklang.Import("pcapx", pcapx.Exports)

	// ja3
	yaklang.Import("ja3", ja3.Exports)

	// sca
	yaklang.Import("sca", sca.Exports)

	// git
	yaklang.Import("git", yakgit.Exports)

	// binx
	yaklang.Import("bin", binx.Exports)
}

type ScriptEngine struct {
	swg    *utils.SizedWaitGroup
	logger **yaklib.YakLogger // 由于logger是要对client设置的，而client可以通过hook设置，所以这里用个二级指针，方便修改 logger
	tasks  *sync.Map

	// 设定几个 hook
	RegisterLogHook          yaklib.RegisterOutputFuncType
	UnregisterLogHook        yaklib.UnregisterOutputFuncType
	RegisterLogConsoleHook   yaklib.RegisterOutputFuncType
	UnregisterLogConsoleHook yaklib.UnregisterOutputFuncType
	RegisterOutputHook       yaklib.RegisterOutputFuncType
	UnregisterOutputHook     yaklib.UnregisterOutputFuncType
	RegisterFailedHook       yaklib.RegisterOutputFuncType
	UnregisterFailedHook     yaklib.UnregisterOutputFuncType
	RegisterFinishHook       yaklib.RegisterOutputFuncType
	UnregisterFinishHook     yaklib.UnregisterOutputFuncType
	RegisterAlertHook        yaklib.RegisterOutputFuncType
	UnregisterAlertHook      yaklib.UnregisterOutputFuncType

	engineHooks []func(engine *antlr4yak.Engine) error
	// 存储yakc密钥
	cryptoKey []byte
	// debug
	debug         bool
	debugInit     func(*yakvm.Debugger)
	debugCallback func(*yakvm.Debugger)
}

func (s *ScriptEngine) GetTaskByTaskID(id string) (*Task, error) {
	raw, ok := s.tasks.Load(id)
	if ok {
		return raw.(*Task), nil
	}
	return nil, errors.Errorf("no existed task[%s]", id)
}

func (s *ScriptEngine) SaveTask(task *Task) error {
	raw, ok := s.tasks.Load(task.TaskID)
	if ok {
		return errors.Errorf("existed task: %s", task.TaskID)
	}

	_ = raw

	s.tasks.Store(task.TaskID, task)
	return nil
}

func (e *ScriptEngine) init() {
	yaklib.RegisterLogHook("debug-log", func(taskId string, data string) {
		log.Infof("task: %v data: %v", taskId, data)

		t, _ := e.GetTaskByTaskID(taskId)
		if t != nil {
			t.Log = append(t.Log, data)
		}
	})
	yaklib.RegisterFinishHooks("debug-log", func(taskId string, data string) {
		t, _ := e.GetTaskByTaskID(taskId)
		if t != nil {
			t.Finished = append(t.Finished, data)
		}
	})
	yaklib.RegisterAlertHooks("debug-log", func(taskId string, data string) {
		t, _ := e.GetTaskByTaskID(taskId)
		if t != nil {
			t.Alert = append(t.Alert, data)
		}
	})
	yaklib.RegisterOutputHooks("debug-log", func(taskId string, data string) {
		t, _ := e.GetTaskByTaskID(taskId)
		if t != nil {
			t.Output = append(t.Output, data)
		}
	})
	yaklib.RegisterFailedHooks("debug-log", func(taskId string, data string) {
		t, _ := e.GetTaskByTaskID(taskId)
		if t != nil {
			t.Failed = append(t.Failed, data)
		}
	})
}

func (e *ScriptEngine) RegisterEngineHooks(f func(engine *antlr4yak.Engine) error) {
	e.engineHooks = append(e.engineHooks, f)
}

func (e *ScriptEngine) SetYakitClient(client *yaklib.YakitClient) {
	e.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		client.SetYakLog(*e.logger)
		log.Infof("set yakit client: %v", client)
		yaklib.SetEngineClient(engine, client)
		vm := engine.GetVM()
		if vm != nil {
			vm.RegisterMapMemberCallHandler("hook", "NewMixPluginCaller", func(i interface{}) interface{} {
				origin, ok := i.(func() (*MixPluginCaller, error))
				if !ok {
					return origin
				}
				return func() (*MixPluginCaller, error) {
					caller, err := origin()
					if err != nil {
						return nil, err
					}
					caller.SetFeedback(func(i *ypb.ExecResult) error {
						yaklib.RawHandlerToExecOutput(client.Output)(i)
						return nil
					})
					return caller, nil
				}
			})
		}
		return nil
	})
}

func (e *ScriptEngine) HookOsExit() {
	e.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		val, ok := engine.GetVar("os")
		if !ok {
			return nil
		}
		osLib, ok := val.(map[string]interface{})
		if !ok {
			return nil
		}
		osLib["Exit"] = func(i ...interface{}) {
			panic(fmt.Sprintf("exit current yak context with: %v", spew.Sdump(i)))
		}
		return nil
	})
}

func (e *ScriptEngine) Compile(code string) ([]byte, error) {
	engine := yaklang.New()
	code = utils.RemoveBOMForString(code)
	return engine.Marshal(code, e.cryptoKey)
}

func (e *ScriptEngine) exec(ctx context.Context, id string, code string, params map[string]interface{}, cache bool) (*antlr4yak.Engine, error) {
	e.swg.Add()
	defer e.swg.Done()

	code = utils.RemoveBOMForString(code)

	t := &Task{
		TaskID:     id,
		Code:       code,
		isRunning:  abool.New(),
		isFinished: abool.New(),
	}
	err := e.SaveTask(t)
	if err != nil {
		return nil, err
	}
	defer func() {
		t.isRunning.UnSet()
		t.isFinished.Set()

		go func() {
			select {
			case <-time.After(3 * time.Second):
				e.tasks.Delete(id)
			}
		}()
	}()

	//log.Infof("recv code: %v", code)
	engine := yaklang.New()

	setCurrentCoreEngine(engine)
	defer func() {
		unsetCurrentCoreEngine(engine)
	}()

	// debug
	engine.SetDebugMode(e.debug)
	engine.SetDebugCallback(e.debugCallback)
	engine.SetDebugInit(e.debugInit)

	engine.SetVar("id", id)
	engine.SetVar("YAK_MAIN", false)
	engine.SetVar("YAK_FILENAME", "")
	engine.SetVar("YAK_DIR", "")

	// 设置参数获取函数
	paramGetter := func(key string) interface{} {
		if params == nil {
			return ""
		}

		result, ok := params[key]
		if !ok {
			return ""
		}
		return result
	}

	yakMainFlag, ok := params["YAK_MAIN"]
	if ok {
		res, _ := yakMainFlag.(bool)
		if res {
			engine.SetVar("YAK_MAIN", true)
		}
	}

	yakAbsFile, _ := params["YAK_FILENAME"]
	if yakAbsFile != nil {
		engine.SetSourceFilePath(fmt.Sprint(yakAbsFile))
		engine.SetVar("YAK_FILENAME", fmt.Sprint(yakAbsFile))
		engine.SetVar("YAK_DIR", filepath.Dir(fmt.Sprint(yakAbsFile)))
	}

	// getParam 和 param 获取参数内容
	engine.SetVar("getParam", paramGetter)
	engine.SetVar("getParams", paramGetter)
	engine.SetVar("param", paramGetter)
	yakFileAbsPath := "temporay_script.yak"
	if yakAbsFile != nil {
		yakFileAbsPath = fmt.Sprint(yakAbsFile)
	}
	*e.logger = yaklib.CreateYakLogger(yakFileAbsPath)
	engine.SetVar("log", *e.logger) // 设置 log 库
	(*e.logger).SetEngine(engine)
	clientIns := *yaklib.GetYakitClientInstance()
	client := &clientIns // 设置全局 client 的 log
	client.SetYakLog(*e.logger)
	yaklib.SetEngineClient(engine, client)

	for _, hook := range e.engineHooks {
		err = hook(engine)
		if err != nil {
			return nil, utils.Errorf("exec engine hooks failed: %s", err)
		}
	}

	t.isRunning.Set()
	if antlr4yak.IsYakc([]byte(code)) {
		return engine, engine.SafeExecYakc(ctx, []byte(code), e.cryptoKey, code)
	}

	if !e.debug && cache && !engine.HaveEvaluatedCode() {
		if yakcBytes, ok := antlr4yak.HaveYakcCache(code); ok && antlr4yak.IsYakc(yakcBytes) {
			return engine, engine.SafeExecYakcWithCode(ctx, yakcBytes, e.cryptoKey, code)
		}
	}
	return engine, engine.SafeEval(ctx, code)
}

func (e *ScriptEngine) ExecuteWithTaskID(taskId, code string) error {
	return e.ExecuteWithTaskIDAndParams(context.Background(), taskId, code, make(map[string]interface{}))
}

func (e *ScriptEngine) ExecuteWithTaskIDAndContext(ctx context.Context, taskId, code string) error {
	return e.ExecuteWithTaskIDAndParams(ctx, taskId, code, make(map[string]interface{}))
}

func (e *ScriptEngine) ExecuteWithTaskIDAndParams(ctx context.Context, taskId, code string, params map[string]interface{}) error {
	engine, err := e.exec(ctx, taskId, code, params, true)
	if err != nil {
		return err
	}
	_ = engine
	return nil
}
func (e *ScriptEngine) ExecuteWithoutCache(code string, params map[string]interface{}) (*antlr4yak.Engine, error) {
	var runtimeId = utils.MapGetStringByManyFields(params, "RUNTIME_ID", "RUNTIME_ID", "runtime_id")
	if runtimeId == "" {
		runtimeId = uuid.New().String()
	}
	return e.exec(context.Background(), runtimeId, code, params, false)
}
func (e *ScriptEngine) ExecuteEx(code string, params map[string]interface{}) (*antlr4yak.Engine, error) {
	var runtimeId = utils.MapGetStringByManyFields(params, "RUNTIME_ID", "RUNTIME_ID", "runtime_id")
	if runtimeId == "" {
		runtimeId = uuid.New().String()
	}
	return e.exec(context.Background(), runtimeId, code, params, true)
}
func (e *ScriptEngine) ExecuteExWithContext(ctx context.Context, code string, params map[string]interface{}) (_ *antlr4yak.Engine, fErr error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("execute ex with context error: %v", err)
			fErr = utils.Errorf("final error: %v", err)
		}
	}()
	var runtimeId = utils.MapGetStringByManyFields(params, "RUNTIME_ID", "RUNTIME_ID", "runtime_id")
	if runtimeId == "" {
		runtimeId = uuid.New().String()
	}
	return e.exec(ctx, runtimeId, code, params, true)
}

func (e *ScriptEngine) Execute(code string) error {
	return e.ExecuteWithTaskID(uuid.New().String(), code)
}
func (e *ScriptEngine) ExecuteWithContext(ctx context.Context, code string) error {
	return e.ExecuteWithTaskIDAndContext(ctx, uuid.New().String(), code)
}

func (e *ScriptEngine) ExecuteMain(code string, AbsFile string) error {
	return e.ExecuteWithTaskIDAndParams(context.Background(), uuid.New().String(), code, map[string]interface{}{
		"YAK_MAIN":     true,
		"YAK_FILENAME": AbsFile,
	})
}
func (e *ScriptEngine) ExecuteMainWithContext(ctx context.Context, code string, AbsFile string) error {
	return e.ExecuteWithTaskIDAndParams(ctx, uuid.New().String(), code, map[string]interface{}{
		"YAK_MAIN":     true,
		"YAK_FILENAME": AbsFile,
	})
}

func (e *ScriptEngine) ExecuteWithTemplate(codeTmp string, i map[string][]string) error {
	results, err := mutate.QuickMutate(codeTmp, nil, mutate.MutateWithExtraParams(i))
	if err != nil {
		return err
	}

	wg := new(sync.WaitGroup)
	for _, result := range results {
		result := result
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := e.ExecuteWithTaskID(uuid.New().String(), result)
			if err != nil {
				log.Errorf("failed: %s", err)
			}
		}()
	}
	wg.Wait()
	return nil
}

func NewYakitVirtualClientScriptEngine(client *yaklib.YakitClient) *ScriptEngine {
	e := NewScriptEngine(20)
	e.SetYakitClient(client)
	return e
}

func Execute(code string, params ...map[string]any) (*antlr4yak.Engine, error) {
	var mergedParams = make(map[string]any)
	if len(params) > 0 {
		for _, param := range params {
			for k, v := range param {
				mergedParams[k] = v
			}
		}
	}
	engine := NewScriptEngine(1)
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		if mergedParams != nil {
			for k, v := range mergedParams {
				engine.SetVar(k, v)
			}
		}
		return nil
	})
	return engine.ExecuteEx(code, mergedParams)
}

func NewScriptEngine(maxConcurrent int) *ScriptEngine {
	var logger *yaklib.YakLogger
	engine := &ScriptEngine{
		swg:                      utils.NewSizedWaitGroup(maxConcurrent),
		tasks:                    new(sync.Map),
		RegisterLogHook:          yaklib.RegisterLogHook,
		RegisterLogConsoleHook:   yaklib.RegisterLogConsoleHook,
		RegisterAlertHook:        yaklib.RegisterAlertHooks,
		RegisterFailedHook:       yaklib.RegisterFailedHooks,
		RegisterFinishHook:       yaklib.RegisterFinishHooks,
		RegisterOutputHook:       yaklib.RegisterOutputHooks,
		UnregisterLogHook:        yaklib.UnregisterLogHook,
		UnregisterLogConsoleHook: yaklib.UnregisterLogConsoleHook,
		UnregisterAlertHook:      yaklib.UnregisterAlertHooks,
		UnregisterFailedHook:     yaklib.UnregisterFailedHooks,
		UnregisterFinishHook:     yaklib.UnregisterFinishHooks,
		UnregisterOutputHook:     yaklib.UnregisterOutputHooks,
		logger:                   &logger,
	}
	engine.init()
	return engine
}

func (s *ScriptEngine) SetDebugInit(callback func(*yakvm.Debugger)) {
	s.debugInit = callback
}

func (s *ScriptEngine) SetDebugCallback(callback func(*yakvm.Debugger)) {
	s.debugCallback = callback
}

func (s *ScriptEngine) SetDebug(debug bool) {
	s.debug = debug
}

func (s *ScriptEngine) Status() map[string]*Task {
	m := make(map[string]*Task)
	s.tasks.Range(func(key, value interface{}) bool {
		k := key.(string)
		v := value.(*Task)
		m[k] = v
		return true
	})
	return m
}

func (s *ScriptEngine) SetCryptoKey(key []byte) error {
	if key == nil {
		return nil
	}
	key = bytes.TrimSpace(key)
	if len(key) != CRYPTO_KEY_SIZE {
		return utils.Errorf("invalid crypto key size: %d", len(key))
	}
	s.cryptoKey = key
	return nil
}
