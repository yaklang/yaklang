package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type CliApp struct {
	appName      string
	document     string
	errorMsg     string
	paramInvalid *utils.AtomicBool

	args        []string
	helpParam   *cliExtraParams
	extraParams []*cliExtraParams

	cliCheckCallback func()
}

func (c *CliApp) SetArgs(args []string) {
	c.args = args
}

func (c *CliApp) SetCliCheckCallback(f func()) {
	c.cliCheckCallback = f
}

func NewCliApp() *CliApp {
	helpParam := &cliExtraParams{
		optShortName: "h",
		optName:      "help",
		params:       []string{"-h", "--help"},
		defaultValue: false,
		helpInfo:     "Show help information",
		required:     false,
		_type:        "bool",
	}
	app := &CliApp{
		paramInvalid:     utils.NewBool(false),
		helpParam:        helpParam,
		extraParams:      []*cliExtraParams{helpParam},
		cliCheckCallback: DefaultExitFunc,
	}
	helpParam.cliApp = app
	return app
}

type cliExtraParams struct {
	envValue     *string
	optName      string
	optShortName string
	params       []string
	defaultValue interface{}
	helpInfo     string
	required     bool
	tempArgs     []string
	_type        string
	cliApp       *CliApp
}

var (
	OsArgs          []string
	DefaultCliApp   = NewCliApp()
	DefaultExitFunc = func() {
		os.Exit(1)
	}
	CliExportFuncNames []string
)

func init() {
	OsArgs = os.Args[:]
	if len(OsArgs) > 1 {
		filename := filepath.Base(os.Args[1])
		fileSuffix := path.Ext(filename)

		// 设置默认的命令行程序名称
		DefaultCliApp.appName = strings.TrimSuffix(filename, fileSuffix)
		DefaultCliApp.args = OsArgs[1:]
	}

	CliExportFuncNames = lo.Keys(CliExports)
}

func GetCliExportMapByCliApp(app *CliApp) map[string]any {
	upperFirst := func(s string) string {
		if len(s) == 0 {
			return s
		}
		return strings.ToUpper(s[:1]) + s[1:]
	}

	ret := make(map[string]any)
	funcMaps := make(map[string]any)

	refV := reflect.ValueOf(app)
	for i := 0; i < refV.NumMethod(); i++ {
		method := refV.Method(i)
		funcMaps[refV.Type().Method(i).Name] = method.Interface()
	}

	for _, name := range CliExportFuncNames {
		if f, ok := funcMaps[name]; ok {
			ret[name] = f
		} else if f, ok = funcMaps[upperFirst(name)]; ok {
			ret[name] = f
		} else {
			// log.Errorf("Cli Can't find function: %s", name)
		}
	}
	return ret
}

func InjectCliArgs(args []string) {
	OsArgs = args
}

func (param *cliExtraParams) foundArgsIndex() int {
	for _, opt := range param.params {
		if ret := utils.StringArrayIndex(param.cliApp.GetArgs(), opt); ret < 0 {
			continue
		} else {
			return ret
		}
	}
	return -1
}

func (param *cliExtraParams) GetDefaultValue(i interface{}) interface{} {
	if param.defaultValue != nil {
		return param.defaultValue
	}

	if !param.required {
		return i
	}

	param.cliApp.paramInvalid.Set()
	param.cliApp.errorMsg += fmt.Sprintf("\n  Parameter [%s] error: miss parameter", param.optName)
	return i
}

type (
	SetCliExtraParam func(c *cliExtraParams)
	UIParams         func() // not used yet
)

var defaultUIParams = func() {}

func parseInt(s string) int {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		// Try to parse scientific notation (e.g., 2e+09)
		f, floatErr := strconv.ParseFloat(s, 64)
		if floatErr == nil {
			return int(f)
		}
		log.Errorf("parse int[%s] failed: %s", s, err)
		return 0
	}
	return int(i)
}

func parseFloat(s string) float64 {
	i, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Errorf("parse float[%s] failed: %s", s, err)
		return 0
	}
	return float64(i)
}

// help 用于输出命令行程序的帮助信息
// 参数:
//   - w: 可选的输出 writer，默认为标准输出
//
// 返回值:
//   - 无
//
// Example:
// ```
// // 关键词: cli.help, 打印用法/参数列表
// cli.SetCliName("demo-tool")
// cli.SetDoc("a demo tool for cli usage")
// cli.String("target", cli.setHelp("target host"), cli.setDefault("yaklang.com"))
// cli.help() // 把 Usage 和参数说明打印到标准输出, 不会退出进程
// ```
func (c *CliApp) Help(w ...io.Writer) {
	var writer io.Writer = os.Stdout

	if len(w) > 0 {
		writer = w[0]
	}

	fmt.Fprintln(writer, "Usage: ")
	fmt.Fprintf(writer, "  %s [OPTIONS]\n", c.appName)
	fmt.Fprintln(writer)
	if len(c.document) > 0 {
		fmt.Fprintln(writer, c.document)
		fmt.Fprintln(writer)
	}
	fmt.Fprintln(writer, "Flags:")
	for _, param := range c.extraParams {
		paramType := param._type
		// bool类型不显示paramType
		if paramType == "bool" {
			paramType = ""
		}
		helpInfo := param.helpInfo
		if param.defaultValue != nil {
			helpInfo += fmt.Sprintf(" (default %v)", param.defaultValue)
		}
		flag := fmt.Sprintf("  %s %s", strings.Join(param.params, ", "), paramType)
		padding := ""
		if len(flag) < 30 {
			padding = strings.Repeat(" ", 30-len(flag))
		}

		fmt.Fprintf(writer, "%v%v%v\n", flag, padding, param.helpInfo)
	}
}

func (c *CliApp) CliCheckFactory(callback func()) func() {
	return func() {
		if c.helpParam.foundArgsIndex() != -1 {
			c.Help()
			callback()
		} else if c.paramInvalid.IsSet() {
			c.errorMsg = strings.TrimSpace(c.errorMsg)
			if len(c.errorMsg) > 0 {
				fmt.Printf("Error:\n  %s\n\n", c.errorMsg)
			}
			c.Help()
			callback()
		}
	}
}

// check 用于检查命令行参数是否合法，这主要检查必要参数是否传入与传入值是否合法
// 参数:
//   - 无
//
// 返回值:
//   - 无
//
// Example:
// ```
// // 关键词: cli.check, 校验必填参数
// cli.SetCliName("demo-tool")
// // 真实脚本里必填参数缺省时 cli.check() 会打印帮助并退出(os.Exit);
// // 这里给必填参数同时设置默认值, 使校验通过、便于演示读取流程
// target = cli.String("target", cli.setRequired(true), cli.setDefault("yaklang.com"))
// cli.check() // 必填参数已满足, 校验通过, 继续执行
// println("target:", target) // 预期输出: target: yaklang.com
// assert target == "yaklang.com", "required param with default passes the check"
// ```
func (c *CliApp) Check() {
	c.CliCheckFactory(c.cliCheckCallback)()
}

// SetCliName 设置此命令行程序的名称
// 这会在命令行输入 --help 或执行`cli.check()`后参数非法时显示
// 参数:
//   - name: 命令行程序名称
//
// 返回值:
//   - 无
//
// Example:
// ```
// // 关键词: cli.SetCliName, 设置命令行程序名(在 --help / 校验失败时展示)
// cli.SetCliName("example-tools")
// cli.help() // Usage 行会显示 example-tools, help 不会退出进程
// ```
func (c *CliApp) SetCliName(name string) {
	c.appName = name
}

// SetDoc 设置此命令行程序的文档
// 这会在命令行输入 --help 或执行`cli.check()`后参数非法时显示
// 参数:
//   - document: 程序文档说明
//
// 返回值:
//   - 无
//
// Example:
// ```
// // 关键词: cli.SetDoc, 设置程序文档说明(在 --help / 校验失败时展示)
// cli.SetDoc("example-tools is a tool for example")
// cli.help() // 该文档会出现在帮助信息中
// ```
func (c *CliApp) SetDoc(document string) {
	c.document = document
}

// setDefault 是一个选项函数，设置参数的默认值
// 参数:
//   - i: 参数默认值
//
// 返回值:
//   - 参数选项函数
//
// Example:
// ```
// // 关键词: cli.setDefault, 为参数设置默认值
// target = cli.String("target", cli.setDefault("yaklang.com"))
// println("target:", target) // 未传 --target 时取默认值: yaklang.com
// assert target == "yaklang.com", "setDefault provides the fallback value"
// ```
func (c *CliApp) SetDefault(i interface{}) SetCliExtraParam {
	return func(c *cliExtraParams) {
		c.defaultValue = i
	}
}

// setHelp 是一个选项函数，设置参数的帮助信息
// 这会在命令行输入 --help 或执行`cli.check()`后参数非法时显示
// 参数:
//   - i: 参数帮助信息
//
// 返回值:
//   - 参数选项函数
//
// Example:
// ```
// // 关键词: cli.setHelp, 设置参数帮助说明(在 --help 中展示)
// host = cli.String("host", cli.setHelp("target host or ip"), cli.setDefault("127.0.0.1"))
// println("host:", host) // 预期输出: host: 127.0.0.1
// assert host == "127.0.0.1", "setHelp only affects help text, not the value"
// ```
func (c *CliApp) SetHelp(i string) SetCliExtraParam {
	return func(c *cliExtraParams) {
		c.helpInfo = i
	}
}

func SetTempArgs(args []string) SetCliExtraParam {
	return func(c *cliExtraParams) {
		c.tempArgs = args
	}
}

// setRequired 是一个选项函数，设置参数是否必须
// 参数:
//   - t: 是否为必填参数
//
// 返回值:
//   - 参数选项函数
//
// Example:
// ```
// // 关键词: cli.setRequired, 声明参数为必填
// // 注意: 必填参数缺省且无默认值时, cli.check() 会打印帮助并退出进程(os.Exit)
// // 这里同时给默认值, 演示读取过程而不触发退出
// token = cli.String("token", cli.setRequired(true), cli.setDefault("demo-token"))
// println("token:", token) // 预期输出: token: demo-token
// assert token == "demo-token", "a required param with a default is satisfied"
// ```
func (c *CliApp) SetRequired(t bool) SetCliExtraParam {
	return func(c *cliExtraParams) {
		c.required = t
	}
}

func (c *CliApp) _getExtraParams(name string, opts ...SetCliExtraParam) *cliExtraParams {
	optName, optShortName := name, ""
	if strings.Contains(name, ",") {
		optName, optShortName, _ = strings.Cut(name, ",")
	} else if strings.Contains(name, " ") {
		optName, optShortName, _ = strings.Cut(name, " ")
	}
	if optShortName != "" && len(optShortName) > len(optName) {
		optName, optShortName = optShortName, optName
	}
	param := &cliExtraParams{
		optName:      optName,
		optShortName: optShortName,
		required:     false,
		defaultValue: nil,
		helpInfo:     "",
		cliApp:       c,
	}
	for _, opt := range opts {
		opt(param)
	}
	c.extraParams = append(c.extraParams, param)
	param.params = _getAvailableParams(param.optName, param.optShortName)
	return param
}

// ------------------------------------------------

func _getAvailableParams(optName, optShortName string) []string {
	optName, optShortName = strings.TrimLeft(optName, "-"), strings.TrimLeft(optShortName, "-")

	if optShortName == "" {
		return []string{fmt.Sprintf("--%v", optName)}
	}
	return []string{fmt.Sprintf("-%v", optShortName), fmt.Sprintf("--%v", optName)}
}

// Args 获取命令行参数
// 参数:
//   - 无
//
// 返回值:
//   - 命令行参数列表
//
// Example:
// ```
// Args = cli.Args()
// ```
func (c *CliApp) GetArgs() []string {
	return c.args
}

// Args 返回传给当前脚本的命令行参数列表（导出名为 cli.Args）
// 即运行 yak 脚本时跟在脚本名之后的原始位置参数
//
// 返回值:
//   - 命令行参数字符串切片
//
// Example:
// ```
// args = cli.Args()
// println(typeof(args).String())   // OUT: []string
// assert typeof(args).String() == "[]string", "cli.Args should return a string slice"
// assert len(args) >= 0, "args length should be non-negative"
// ```
func (c *CliApp) Args() []string {
	return c.args
}

func (c *CliApp) _cliFromString(name string, opts ...SetCliExtraParam) (string, *cliExtraParams) {
	param := c._getExtraParams(name, opts...)
	if param.envValue != nil {
		return *param.envValue, param
	}
	index := param.foundArgsIndex()
	if index < 0 {
		return utils.InterfaceToString(param.GetDefaultValue("")), param
	}
	args := c.GetArgs()
	if param.tempArgs != nil {
		args = param.tempArgs
	}
	if index+1 >= len(args) {
		// 防止数组越界
		return utils.InterfaceToString(param.GetDefaultValue("")), param
	}
	return args[index+1], param
}

// Bool 获取对应名称的命令行参数，并将其转换为 bool 类型返回
// 参数:
//   - name: 参数名
//   - opts: 参数选项，如 cli.setHelp 等
//
// 返回值:
//   - 参数对应的 bool 值（传入该 flag 时为 true）
//
// Example:
// ```
// // 关键词: cli.Bool, 开关型参数
// // 开关型参数不需要值: 命令行带 --verbose 即为 true, 不带则为 false
// verbose = cli.Bool("verbose")
// println("verbose:", verbose) // 预期输出(未传参数时): verbose: false
// assert verbose == false, "flag defaults to false when not provided"
// ```
func (c *CliApp) Bool(name string, opts ...SetCliExtraParam) bool {
	p := c._getExtraParams(name, opts...)
	p._type = "bool"
	p.required = false

	index := p.foundArgsIndex()
	if index < 0 {
		return false // c.GetDefaultValue(false).(bool)
	}
	return true
}

// Have 获取对应名称的命令行参数，并将其转换为 bool 类型返回
// 参数:
//   - name: 参数名
//   - opts: 参数选项，如 cli.setHelp 等
//
// 返回值:
//   - 参数对应的 bool 值（传入该 flag 时为 true）
//
// Example:
// ```
// // 关键词: cli.Have, cli.Bool 的别名
// hasDebug = cli.Have("debug")
// println("hasDebug:", hasDebug) // 预期输出: hasDebug: false
// assert hasDebug == false, "Have is an alias of cli.Bool"
// ```
func (c *CliApp) Have(name string, opts ...SetCliExtraParam) bool {
	return c.Bool(name, opts...)
}

// String 获取对应名称的命令行参数，并将其转换为 string 类型返回
// 参数:
//   - name: 参数名
//   - opts: 参数选项，如 cli.setRequired、cli.setDefault 等
//
// 返回值:
//   - 参数对应的字符串值
//
// Example:
// ```
// // 关键词: cli.String, 字符串参数
// // setDefault 提供默认值, 命令行未传 --target 时取默认值
// target = cli.String("target", cli.setDefault("yaklang.com"))
// println("target:", target) // 预期输出: target: yaklang.com
// assert target == "yaklang.com", "should fall back to default value"
// ```
func (c *CliApp) String(name string, opts ...SetCliExtraParam) string {
	s, p := c._cliFromString(name, opts...)
	p._type = "string"
	return s
}

// HTTPPacket 获取对应名称的命令行参数，并将其转换为 string 类型返回
// 其作为一个独立脚本运行时与 cli.String 没有区别，仅在 Yakit 图形化中展示为 HTTP 报文形式
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 参数对应的字符串值
//
// Example:
// ```
// // 关键词: cli.HTTPPacket, 独立运行时等价 cli.String, 仅 Yakit 中展示为 HTTP 报文输入框
// packet = cli.HTTPPacket("req", cli.setDefault("GET / HTTP/1.1\r\nHost: yaklang.com\r\n\r\n"))
// println(packet) // 默认值即一段 HTTP 报文
// assert packet.Contains("GET /"), "should return the default packet"
// ```
func (c *CliApp) HTTPPacket(name string, opts ...SetCliExtraParam) string {
	return c.String(name, opts...)
}

// YakCode 获取对应名称的命令行参数，并将其转换为 string 类型返回
// 其作为一个独立脚本运行时与 cli.String 没有区别，仅在 Yakit 图形化中展示为 Yak 代码形式
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 参数对应的字符串值
//
// Example:
// ```
// // 关键词: cli.YakCode, 等价 cli.String, 仅 Yakit 中展示为 Yak 代码编辑器
// code = cli.YakCode("code", cli.setDefault(`println("hello")`))
// println(code) // 预期输出: println("hello")
// assert code.Contains("println"), "should return the default code"
// ```
func (c *CliApp) YakCode(name string, opts ...SetCliExtraParam) string {
	return c.String(name, opts...)
}

// Text 获取对应名称的命令行参数，并将其转换为 string 类型返回
// 其作为一个独立脚本运行时与 cli.String 没有区别，仅在 Yakit 图形化中展示为文本框形式
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 参数对应的字符串值
//
// Example:
// ```
// // 关键词: cli.Text, 等价 cli.String, 仅 Yakit 中展示为多行文本框
// note = cli.Text("note", cli.setDefault("hello yak"))
// println("note:", note) // 预期输出: note: hello yak
// assert note == "hello yak", "should return the default text"
// ```
func (c *CliApp) Text(name string, opts ...SetCliExtraParam) string {
	return c.String(name, opts...)
}

// Int 获取对应名称的命令行参数，并将其转换为 int 类型返回
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 参数对应的整数值
//
// Example:
// ```
// // 关键词: cli.Int, 整数参数
// port = cli.Int("port", cli.setDefault(80))
// println("port:", port) // 预期输出: port: 80
// assert port == 80, "should return the default int"
// ```
func (c *CliApp) Int(name string, opts ...SetCliExtraParam) int {
	s, p := c._cliFromString(name, opts...)
	p._type = "int"
	if s == "" {
		return 0
	}
	return parseInt(s)
}

// Integer 获取对应名称的命令行参数，并将其转换为 int 类型返回
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 参数对应的整数值
//
// Example:
// ```
// // 关键词: cli.Integer, cli.Int 的别名
// port = cli.Integer("port", cli.setDefault(443))
// println("port:", port) // 预期输出: port: 443
// assert port == 443, "Integer is an alias of cli.Int"
// ```
func (c *CliApp) Integer(name string, opts ...SetCliExtraParam) int {
	return c.Int(name, opts...)
}

// Float 获取对应名称的命令行参数，并将其转换为 float 类型返回
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 参数对应的浮点值
//
// Example:
// ```
// // 关键词: cli.Float, 浮点数参数
// percent = cli.Float("percent", cli.setDefault(0.5))
// println("percent:", percent) // 预期输出: percent: 0.5
// assert percent == 0.5, "should return the default float"
// ```
func (c *CliApp) Float(name string, opts ...SetCliExtraParam) float64 {
	s, p := c._cliFromString(name, opts...)
	p._type = "float"
	if s == "" {
		return 0.0
	}
	return parseFloat(s)
}

// Double 获取对应名称的命令行参数，并将其转换为 float 类型返回
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 参数对应的浮点值
//
// Example:
// ```
// // 关键词: cli.Double, cli.Float 的别名
// ratio = cli.Double("ratio", cli.setDefault(1.5))
// println("ratio:", ratio) // 预期输出: ratio: 1.5
// assert ratio == 1.5, "Double is an alias of cli.Float"
// ```
func (c *CliApp) Double(name string, opts ...SetCliExtraParam) float64 {
	return c.Float(name, opts...)
}

// Urls 获取对应名称的命令行参数，根据","切割并尝试将其转换为符合URL格式并返回 []string 类型
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 解析后的 URL 列表
//
// Example:
// ```
// // 关键词: cli.Urls, 将逗号分隔的目标解析为标准 URL 列表(会自动补全 www 变体)
// urls = cli.Urls("urls", cli.setDefault("yaklang.com:443,example.com:443"))
// println(urls) // 端口 443 解析为 https, 并自动补全 www 变体
// assert urls[0] == "https://yaklang.com", "port 443 should be parsed as https"
// assert len(urls) >= 2, "comma-separated targets should be parsed"
// ```
func (c *CliApp) Urls(name string, opts ...SetCliExtraParam) []string {
	s, p := c._cliFromString(name, opts...)
	p._type = "urls"
	ret := utils.ParseStringToUrlsWith3W(utils.ParseStringToHosts(s)...)
	if ret == nil {
		return []string{}
	}
	return ret
}

// Url 获取对应名称的命令行参数，根据","切割并尝试将其转换为符合URL格式并返回 []string 类型
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 解析后的 URL 列表
//
// Example:
// ```
// // 关键词: cli.Url, cli.Urls 的别名
// urls = cli.Url("urls", cli.setDefault("yaklang.com"))
// println(urls) // 未指定端口时同时给出 http/https 以及 www 变体
// assert urls[0] == "https://yaklang.com", "the first url should be the https form"
// assert len(urls) >= 1, "Url is an alias of cli.Urls"
// ```
func (c *CliApp) Url(name string, opts ...SetCliExtraParam) []string {
	return c.Urls(name, opts...)
}

// Ports 获取对应名称的命令行参数，根据","与"-"切割并尝试解析端口并返回 []int 类型
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 解析后的端口列表
//
// Example:
// ```
// // 关键词: cli.Ports, 解析端口范围/列表为 []int
// ports = cli.Ports("ports", cli.setDefault("10086-10088,23333"))
// println(ports) // 预期: [10086, 10087, 10088, 23333]
// assert len(ports) == 4, "range and list should be expanded"
// ```
func (c *CliApp) Ports(name string, opts ...SetCliExtraParam) []int {
	s, p := c._cliFromString(name, opts...)
	p._type = "port"
	ret := utils.ParseStringToPorts(s)
	if ret == nil {
		return []int{}
	}
	return ret
}

// Port 获取对应名称的命令行参数，根据","与"-"切割并尝试解析端口并返回 []int 类型
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 解析后的端口列表
//
// Example:
// ```
// // 关键词: cli.Port, cli.Ports 的别名
// ports = cli.Port("ports", cli.setDefault("80,443"))
// println(ports) // 预期: [80, 443]
// assert len(ports) == 2, "Port is an alias of cli.Ports"
// ```
func (c *CliApp) Port(name string, opts ...SetCliExtraParam) []int {
	return c.Ports(name, opts...)
}

// Hosts 获取对应名称的命令行参数，根据","切割并尝试解析CIDR网段并返回 []string 类型
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 解析后的主机 IP 列表
//
// Example:
// ```
// // 关键词: cli.Hosts, 解析 CIDR 网段 / 逗号分隔为主机 IP 列表
// hosts = cli.Hosts("hosts", cli.setDefault("192.168.0.0/30,172.17.0.1"))
// println(hosts) // 192.168.0.0/30 段内的 IP 加上 172.17.0.1
// assert len(hosts) >= 2, "cidr should be expanded into multiple hosts"
// ```
func (c *CliApp) Hosts(name string, opts ...SetCliExtraParam) []string {
	s, p := c._cliFromString(name, opts...)
	p._type = "hosts"
	ret := utils.ParseStringToHosts(s)
	if ret == nil {
		c.paramInvalid.Set()
		c.errorMsg += fmt.Sprintf("\n  Parameter [%s] error: Parse string to host error: %s", p.optName, s)
		return []string{}
	}
	return ret
}

// Host 获取对应名称的命令行参数，根据","切割并尝试解析CIDR网段并返回 []string 类型
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 解析后的主机 IP 列表
//
// Example:
// ```
// // 关键词: cli.Host, cli.Hosts 的别名
// hosts = cli.Host("hosts", cli.setDefault("127.0.0.1"))
// println(hosts) // 预期: ["127.0.0.1"]
// assert len(hosts) == 1, "Host is an alias of cli.Hosts"
// ```
func (c *CliApp) Host(name string, opts ...SetCliExtraParam) []string {
	return c.Hosts(name, opts...)
}

// NetWork 获取对应名称的命令行参数，根据","切割并尝试解析CIDR网段并返回 []string 类型
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 解析后的主机 IP 列表
//
// Example:
// ```
// // 关键词: cli.Network, cli.Hosts 的别名
// hosts = cli.Network("hosts", cli.setDefault("10.0.0.1"))
// println(hosts) // 预期: ["10.0.0.1"]
// assert len(hosts) == 1, "Network is an alias of cli.Hosts"
// ```
func (c *CliApp) Network(name string, opts ...SetCliExtraParam) []string {
	return c.Hosts(name, opts...)
}

// Net 获取对应名称的命令行参数，根据","切割并尝试解析CIDR网段并返回 []string 类型
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 解析后的主机 IP 列表
//
// Example:
// ```
// // 关键词: cli.Net, cli.Hosts 的别名
// hosts = cli.Net("hosts", cli.setDefault("10.0.0.2"))
// println(hosts) // 预期: ["10.0.0.2"]
// assert len(hosts) == 1, "Net is an alias of cli.Hosts"
// ```
func (c *CliApp) Net(name string, opts ...SetCliExtraParam) []string {
	return c.Hosts(name, opts...)
}

// File 获取对应名称的命令行参数，根据其传入的值读取其对应文件内容并返回 []byte 类型
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 文件内容字节
//
// Example:
// ```
// // 关键词: cli.File, 读取参数指向的文件内容
// p = file.Join(os.TempDir(), "yak-cli-file-demo.txt")
// file.Save(p, "hello yak")~ // 先准备一个文件, 真实使用时由命令行传入路径
// content = cli.File("cfg", cli.setDefault(p))
// println("content:", string(content)) // 预期输出: content: hello yak
// assert string(content) == "hello yak", "should read the file content"
// file.Remove(p)
// ```
func (c *CliApp) File(name string, opts ...SetCliExtraParam) []byte {
	s, p := c._cliFromString(name, opts...)
	p._type = "file"

	if s == "" {
		return []byte{}
	}

	if utils.GetFirstExistedPath(s) == "" && !c.paramInvalid.IsSet() {
		c.paramInvalid.Set()
		c.errorMsg += fmt.Sprintf("\n  Parameter [%s] error: No such file: %s", p.optName, s)
		return []byte{}
	}
	raw, err := ioutil.ReadFile(s)
	if err != nil {
		c.paramInvalid.Set()
		c.errorMsg += fmt.Sprintf("\n  Parameter [%s] error: %s", p.optName, err.Error())
		return []byte{}
	}

	return raw
}

// FileNames 获取对应名称的命令行参数，获得选中的所有文件路径，并返回 []string 类型
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 文件路径列表
//
// Example:
// ```
// // 关键词: cli.FileNames, 解析逗号分隔的多个文件路径(仅切割路径, 不读取内容)
// names = cli.FileNames("files", cli.setDefault("/etc/passwd,/etc/hosts"))
// println(names) // 预期: ["/etc/passwd", "/etc/hosts"]
// assert len(names) == 2, "should split file names by comma"
// ```
func (c *CliApp) FileNames(name string, opts ...SetCliExtraParam) []string {
	rawStr, p := c._cliFromString(name, opts...)
	p._type = "file-names"

	if rawStr == "" {
		return []string{}
	}

	return utils.PrettifyListFromStringSplited(rawStr, ",")
}

// FolderName 获取对应名称的命令行参数，获得选中的文件夹路径，并返回 string 类型
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 文件夹路径
//
// Example:
// ```
// // 关键词: cli.FolderName, 文件夹路径参数
// folder = cli.FolderName("folder", cli.setDefault("/tmp"))
// println("folder:", folder) // 预期输出: folder: /tmp
// assert folder == "/tmp", "should return the folder path"
// ```
func (c *CliApp) FolderName(name string, opts ...SetCliExtraParam) string {
	rawStr, p := c._cliFromString(name, opts...)
	p._type = "folder-name"
	if rawStr == "" {
		return ""
	}
	return rawStr
}

// FileOrContent 获取对应名称的命令行参数
// 根据其传入的值尝试读取其对应文件内容，如果无法读取则直接返回，最后返回 []byte 类型
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 文件内容或原始内容字节
//
// Example:
// ```
// // 关键词: cli.FileOrContent, 传入路径则读文件, 否则把值本身当作内容
// // 传入的不是存在的路径时, 原样作为内容返回
// data = cli.FileOrContent("data", cli.setDefault("inline-content"))
// println("data:", string(data)) // 预期输出: data: inline-content
// assert string(data) == "inline-content", "non-path value is used as content directly"
// ```
func (c *CliApp) FileOrContent(name string, opts ...SetCliExtraParam) []byte {
	s, p := c._cliFromString(name, opts...)
	p._type = "file_or_content"
	ret := utils.StringAsFileParams(s)
	if ret == nil {
		c.paramInvalid.Set()
		c.errorMsg += fmt.Sprintf("\n  Parameter [%s] error: Empty file or content: %s", p.optName, s)
		return []byte{}
	}
	return ret
}

// LineDict 获取对应名称的命令行参数
// 根据其传入的值尝试读取其对应文件内容，如果无法读取则作为字符串，最后根据换行符切割，返回 []string 类型
// 参数:
//   - name: 参数名
//   - opts: 参数选项
//
// 返回值:
//   - 按行切割后的字符串列表
//
// Example:
// ```
// // 关键词: cli.LineDict, 传文件则按行读取, 否则把内容按换行切割
// lines = cli.LineDict("dict", cli.setDefault("admin\nroot\nguest"))
// println(lines) // 预期: ["admin", "root", "guest"]
// assert len(lines) == 3, "content should be split into lines"
// ```
func (c *CliApp) LineDict(name string, opts ...SetCliExtraParam) []string {
	s, p := c._cliFromString(name, opts...)
	p._type = "file-or-content"
	raw := utils.StringAsFileParams(s)
	if raw == nil {
		c.paramInvalid.Set()
		c.errorMsg += fmt.Sprintf("\n  Parameter [%s] error: Empty file or content: %s", p.optName, s)
		return []string{}
	}

	return utils.ParseStringToLines(string(raw))
}

// Json 获取对应名称的命令行参数, 与cli.JsonSchema一起使用以构建复杂参数
// 详情参考:
// 1. https://json-schema.org/docs
// 2. https://rjsf-team.github.io/react-jsonschema-form/
// Example:
// ```
// // 关键词: cli.Json, 解析 JSON 参数为 map
// // 独立运行时按 JSON 字符串解析; 在 Yakit 图形化中可配合 cli.setJsonSchema 渲染复杂表单
// info = cli.Json("info", cli.setDefault(`{"name": "Chuck", "age": 18}`))
// println("name:", info["name"]) // 预期输出: name: Chuck
// assert info["name"] == "Chuck", "should parse the json default value"
// ```
//
// 参数:
//   - name: 参数名
//   - opts: 参数选项，通常配合 cli.setJsonSchema 使用
//
// 返回值:
//   - 解析后的 JSON 对象（map）
func (c *CliApp) Json(name string, opts ...SetCliExtraParam) map[string]any {
	s, p := c._cliFromString(name, opts...)
	p._type = "json"
	var result map[string]any
	err := json.Unmarshal([]byte(s), &result)
	if err != nil {
		c.paramInvalid.Set()
		c.errorMsg += fmt.Sprintf("\n  Parameter [%s] error: Invalid JSON: %s", p.optName, err.Error())
		return nil
	}
	return result
}

// YakitPlugin 获取名称为 yakit-plugin-file 的命令行参数
// 根据其传入的值读取其对应文件内容并根据"|"切割并返回 []string 类型，表示各个插件名
// 参数:
//   - options: 参数选项
//
// 返回值:
//   - 插件名列表
//
// Example:
// ```
// // 关键词: cli.YakitPlugin, 读取 yakit-plugin-file 文件并按 | 切割插件名
// p = file.Join(os.TempDir(), "yak-cli-plugins.txt")
// file.Save(p, "plugin-a|plugin-b|plugin-c")~ // 真实使用时由 Yakit 选择插件后生成
// plugins = cli.YakitPlugin(cli.setDefault(p))
// println(plugins) // 预期: ["plugin-a", "plugin-b", "plugin-c"]
// assert len(plugins) == 3, "should split plugin names by |"
// file.Remove(p)
// ```
func (c *CliApp) YakitPlugin(options ...SetCliExtraParam) []string {
	paramName := "yakit-plugin-file"
	filename, p := c._cliFromString(paramName, options...)
	p._type = "yakit-plugin"

	if filename == "" {
		return []string{}
	}

	raw, err := ioutil.ReadFile(filename)
	if err != nil {
		c.paramInvalid.Set()
		c.errorMsg += fmt.Sprintf("\n  Parameter [%s] error: %s", p.optName, err.Error())
		return []string{}
	}
	if raw == nil {
		c.paramInvalid.Set()
		c.errorMsg += fmt.Sprintf("\n  Parameter [%s] error: Can't read file: %s", p.optName, filename)
		return []string{}
	}
	return utils.PrettifyListFromStringSplited(string(raw), "|")
}

// StringSlice 获取对应名称的命令行参数，将其字符串根据","切割返回 []string 类型
// 参数:
//   - name: 参数名
//   - options: 参数选项
//
// 返回值:
//   - 字符串列表
//
// Example:
// ```
// // 关键词: cli.StringSlice, 逗号分隔的字符串列表
// targets = cli.StringSlice("targets", cli.setDefault("yaklang.com,example.com"))
// println(targets) // 预期: ["yaklang.com", "example.com"]
// assert len(targets) == 2, "should split by comma"
// ```
func (c *CliApp) StringSlice(name string, options ...SetCliExtraParam) []string {
	rawStr, p := c._cliFromString(name, options...)
	p._type = "string-slice"

	if rawStr == "" {
		return []string{}
	}

	return utils.PrettifyListFromStringSplited(rawStr, ",")
}

// IntSlice 获取对应名称的命令行参数，将其字符串根据","切割并尝试转换为 int 类型返回 []int 类型
// 参数:
//   - name: 参数名
//   - options: 参数选项
//
// 返回值:
//   - 整数列表
//
// Example:
// ```
// // 关键词: cli.IntSlice, 逗号分隔的整数列表
// ports = cli.IntSlice("ports", cli.setDefault("80,443,8080"))
// println(ports) // 预期: [80, 443, 8080]
// assert len(ports) == 3, "should parse three ints"
// ```
func (c *CliApp) IntSlice(name string, options ...SetCliExtraParam) []int {
	rawStr, p := c._cliFromString(name, options...)
	p._type = "int-slice"

	if rawStr == "" {
		return []int{}
	}

	var ret []int
	for _, s := range utils.PrettifyListFromStringSplited(rawStr, ",") {
		ret = append(ret, parseInt(s))
	}
	return ret
}

// setVerboseName 是一个选项函数，设置参数的中文名
// 参数:
//   - verboseName: 参数中文名
//
// 返回值:
//   - 参数选项函数
//
// Example:
// ```
// // 关键词: cli.setVerboseName, 设置参数中文名(仅 Yakit 图形化展示)
// target = cli.String("target", cli.setVerboseName("目标"), cli.setDefault("yaklang.com"))
// println("target:", target) // 预期输出: target: yaklang.com
// assert target == "yaklang.com", "verbose name only changes the UI label"
// ```
func (c *CliApp) SetVerboseName(verboseName string) SetCliExtraParam {
	return func(cep *cliExtraParams) {}
}

// setCliGroup 是一个选项函数，设置参数的分组
// Example:
// ```
// // 关键词: cli.setCliGroup, 把多个参数归入同一分组(仅 Yakit 图形化布局)
// host = cli.String("host", cli.setCliGroup("common"), cli.setDefault("127.0.0.1"))
// port = cli.Int("port", cli.setCliGroup("common"), cli.setDefault(80))
// println("host:", host, "port:", port) // 预期输出: host: 127.0.0.1 port: 80
// assert host == "127.0.0.1" && port == 80, "group only affects UI layout"
// ```
//
// 参数:
//   - group: 参数分组名
//
// 返回值:
//   - 参数选项函数
func (c *CliApp) SetCliGroup(group string) SetCliExtraParam {
	return func(cep *cliExtraParams) {}
}

// setYakitPayload 是一个选项函数，设置参数建议值为Yakit payload的字典名列表
// 参数:
//   - b: 是否启用 Yakit payload 建议值
//
// 返回值:
//   - 参数选项函数
//
// Example:
// ```
// // 关键词: cli.setYakitPayload, 提示该参数可用 Yakit 字典(仅图形化建议值)
// dictName = cli.String("dictName", cli.setYakitPayload(true), cli.setDefault("top100"))
// println("dictName:", dictName) // 预期输出: dictName: top100
// assert dictName == "top100", "payload hint only affects UI suggestion"
// ```
func (c *CliApp) SetYakitPayload(b bool) SetCliExtraParam {
	return func(c *cliExtraParams) {
	}
}

// setShortName 是一个选项函数，设置参数的短名称
// Example:
// ```
// // 关键词: cli.setShortName, 设置参数短名, 命令行可用 -t 代替 --target
// target = cli.String("target", cli.setShortName("t"), cli.setDefault("yaklang.com"))
// println("target:", target) // 预期输出: target: yaklang.com
// assert target == "yaklang.com", "short name adds a -t alias"
// ```
// 在命令行可以使用`-t`代替`--target`
//
// 参数:
//   - shortName: 参数短名称
//
// 返回值:
//   - 参数选项函数
func (c *CliApp) SetShortName(shortName string) SetCliExtraParam {
	return func(c *cliExtraParams) {
		c.optShortName = shortName
	}
}

// SetMultipleSelect 是一个选项函数，设置参数是否可以多选
// 此选项仅在`cli.StringSlice`中生效
// 参数:
//   - multiSelect: 是否允许多选
//
// 返回值:
//   - 参数选项函数
//
// Example:
// ```
// // 关键词: cli.setMultipleSelect, 允许多选(仅对 cli.StringSlice 生效, 图形化)
// targets = cli.StringSlice("targets", cli.setMultipleSelect(true), cli.setDefault("a,b"))
// println(targets) // 预期: ["a", "b"]
// assert len(targets) == 2, "multiple-select only affects the UI"
// ```
func (c *CliApp) SetMultipleSelect(multiSelect bool) SetCliExtraParam {
	return func(cep *cliExtraParams) {}
}

// setSelectOption 是一个选项函数，设置参数的下拉框选项
// 此选项仅在`cli.StringSlice`中生效
// 参数:
//   - name: 下拉框选项显示名
//   - value: 下拉框选项值
//
// 返回值:
//   - 参数选项函数
//
// Example:
// ```
// // 关键词: cli.setSelectOption, 为下拉框增加可选项(仅对 cli.StringSlice 生效, 图形化)
// targets = cli.StringSlice("targets", cli.setSelectOption("选项A", "a"), cli.setDefault("a"))
// println(targets) // 预期: ["a"]
// assert len(targets) == 1, "select option only affects the UI dropdown"
// ```
func (c *CliApp) SetSelectOption(name, value string) SetCliExtraParam {
	return func(cep *cliExtraParams) {}
}

// setJsonSchema 是一个选项参数,用于在cli.Json中使用JsonSchema构建复杂参数
// 详情参考:
// 1. https://json-schema.org/docs
// 2. https://rjsf-team.github.io/react-jsonschema-form/
// Example:
// ```
// // 关键词: cli.setJsonSchema, 用 JSON Schema 在 Yakit 中渲染复杂表单
// // 独立运行时仍按 JSON 解析; 这里用默认值演示解析过程(图形化中由 schema 驱动表单)
// info = cli.Json("info",
//     cli.setVerboseName("项目信息"),
//     cli.setJsonSchema(`{"type":"object","properties":{"name":{"type":"string"}}}`),
//     cli.setDefault(`{"name": "Chuck"}`),
// )
// println("name:", info["name"]) // 预期输出: name: Chuck
// assert info["name"] == "Chuck", "json default should be parsed"
// ```
//
// 参数:
//   - schema: JSON Schema 字符串
//   - uis: 可选的 UI Schema
//
// 返回值:
//   - 参数选项函数
func (c *CliApp) SetJsonSchema(schema string, uis ...*UISchema) SetCliExtraParam {
	return func(c *cliExtraParams) {}
}

// setPluginEnv 是一个选项函数，设置参数从插件环境中取值
// 参数:
//   - key: 插件环境变量的键
//
// 返回值:
//   - 参数选项函数
//
// Example:
// ```
// // 关键词: cli.setPluginEnv, 让参数从插件环境变量(数据库)中取值
// apiKey = cli.String("key", cli.setPluginEnv("api-key"))
// println("api key from plugin env:", apiKey) // 环境中存在该 key 时取其值, 否则为空
// // 无法本地验证: 真实取值依赖已写入插件环境数据库的记录
// ```
func (c *CliApp) SetPluginEnv(key string) SetCliExtraParam {
	return func(cep *cliExtraParams) {
		var env schema.PluginEnv
		if db := consts.GetGormProfileDatabase().Select("value").Where("key = ?", key).First(&env); db.Error != nil {
			log.Errorf("GetPluginEnvByKey error: %s", db.Error)
		}
		cep.envValue = &env.Value
	}
}

// UI 用于组合一组 UI 联动规则，仅在 Yakit 图形化中生效（导出名为 cli.UI）
// 参数:
//   - opts: 一个或多个 UI 联动规则，如 cli.showGroup、cli.whenTrue 等
//
// 返回值:
//   - 无
//
// Example:
// ```
// // 关键词: cli.UI, 组合 UI 联动规则(仅 Yakit 图形化生效)
// cli.UI(cli.showGroup("advanced"), cli.whenTrue("enableAdvanced"))
// println("ui rule registered") // 规则只在图形化界面生效, 独立运行时是空操作
// ```
func (c *CliApp) UI(opts ...UIParams) {
}

// showGroup 当条件满足时显示指定参数分组的 UI 规则（导出名为 cli.showGroup）
// 参数:
//   - group: 参数分组名
//
// 返回值:
//   - UI 联动规则
//
// Example:
// ```
// // 关键词: cli.showGroup, 条件满足时显示某分组(仅图形化)
// cli.UI(cli.whenTrue("adv"), cli.showGroup("advanced"))
// println("show-group rule registered")
// ```
func (c *CliApp) showGroup(group string) UIParams {
	return defaultUIParams
}

// showParams 当条件满足时显示指定参数的 UI 规则（导出名为 cli.showParams）
// 参数:
//   - params: 一个或多个参数名
//
// 返回值:
//   - UI 联动规则
//
// Example:
// ```
// // 关键词: cli.showParams, 条件满足时显示指定参数(仅图形化)
// cli.UI(cli.whenTrue("adv"), cli.showParams("threads", "timeout"))
// println("show-params rule registered")
// ```
func (c *CliApp) showParams(params ...string) UIParams {
	return defaultUIParams
}

// hideGroup 当条件满足时隐藏指定参数分组的 UI 规则（导出名为 cli.hideGroup）
// 参数:
//   - group: 参数分组名
//
// 返回值:
//   - UI 联动规则
//
// Example:
// ```
// // 关键词: cli.hideGroup, 条件满足时隐藏某分组(仅图形化)
// cli.UI(cli.whenFalse("adv"), cli.hideGroup("advanced"))
// println("hide-group rule registered")
// ```
func (c *CliApp) hideGroup(group string) UIParams {
	return defaultUIParams
}

// hideParams 当条件满足时隐藏指定参数的 UI 规则（导出名为 cli.hideParams）
// 参数:
//   - params: 一个或多个参数名
//
// 返回值:
//   - UI 联动规则
//
// Example:
// ```
// // 关键词: cli.hideParams, 条件满足时隐藏指定参数(仅图形化)
// cli.UI(cli.whenFalse("adv"), cli.hideParams("threads"))
// println("hide-params rule registered")
// ```
func (c *CliApp) hideParams(params ...string) UIParams {
	return defaultUIParams
}

// whenTrue 构造一个“当指定布尔参数为真时”的 UI 联动条件（导出名为 cli.whenTrue）
// 参数:
//   - param: 参数名
//
// 返回值:
//   - UI 联动规则
//
// Example:
// ```
// // 关键词: cli.whenTrue, 当某布尔参数为真时触发(仅图形化)
// cli.UI(cli.whenTrue("adv"), cli.showGroup("advanced"))
// println("when-true rule registered")
// ```
func (c *CliApp) whenTrue(param string) UIParams {
	return defaultUIParams
}

// whenFalse 构造一个“当指定布尔参数为假时”的 UI 联动条件（导出名为 cli.whenFalse）
// 参数:
//   - param: 参数名
//
// 返回值:
//   - UI 联动规则
//
// Example:
// ```
// // 关键词: cli.whenFalse, 当某布尔参数为假时触发(仅图形化)
// cli.UI(cli.whenFalse("adv"), cli.hideGroup("advanced"))
// println("when-false rule registered")
// ```
func (c *CliApp) whenFalse(param string) UIParams {
	return defaultUIParams
}

// whenEqual 构造一个“当指定参数等于某值时”的 UI 联动条件（导出名为 cli.whenEqual）
// 参数:
//   - param: 参数名
//   - value: 比较值
//
// 返回值:
//   - UI 联动规则
//
// Example:
// ```
// // 关键词: cli.whenEqual, 当某参数等于指定值时触发(仅图形化)
// cli.UI(cli.whenEqual("mode", "advanced"), cli.showGroup("advanced"))
// println("when-equal rule registered")
// ```
func (c *CliApp) whenEqual(param string, value string) UIParams {
	return defaultUIParams
}

// whenNotEqual 构造一个“当指定参数不等于某值时”的 UI 联动条件（导出名为 cli.whenNotEqual）
// 参数:
//   - param: 参数名
//   - value: 比较值
//
// 返回值:
//   - UI 联动规则
//
// Example:
// ```
// // 关键词: cli.whenNotEqual, 当某参数不等于指定值时触发(仅图形化)
// cli.UI(cli.whenNotEqual("mode", "simple"), cli.showGroup("advanced"))
// println("when-not-equal rule registered")
// ```
func (c *CliApp) whenNotEqual(param string, value string) UIParams {
	return defaultUIParams
}

// when 构造一个基于表达式的 UI 联动条件（导出名为 cli.when）
// 参数:
//   - expression: 条件表达式
//
// 返回值:
//   - UI 联动规则
//
// Example:
// ```
// // 关键词: cli.when, 基于表达式的联动条件(仅图形化)
// cli.UI(cli.when("threads > 10"), cli.showParams("warning"))
// println("when-expression rule registered")
// ```
func (c *CliApp) when(expression string) UIParams {
	return defaultUIParams
}

// whenDefault 构造一个默认（兜底）UI 联动条件（导出名为 cli.whenDefault）
// 参数:
//   - 无
//
// 返回值:
//   - UI 联动规则
//
// Example:
// ```
// // 关键词: cli.whenDefault, 兜底联动条件(其他条件都不满足时, 仅图形化)
// cli.UI(cli.whenDefault(), cli.hideGroup("advanced"))
// println("when-default rule registered")
// ```
func (c *CliApp) whenDefault() UIParams {
	return defaultUIParams
}

var CliExports = map[string]interface{}{
	"Args":        DefaultCliApp.Args,
	"Bool":        DefaultCliApp.Bool,
	"Have":        DefaultCliApp.Have,
	"String":      DefaultCliApp.String,
	"HTTPPacket":  DefaultCliApp.HTTPPacket,
	"YakCode":     DefaultCliApp.YakCode,
	"Text":        DefaultCliApp.Text,
	"Int":         DefaultCliApp.Int,
	"Integer":     DefaultCliApp.Integer,
	"Float":       DefaultCliApp.Float,
	"Double":      DefaultCliApp.Double,
	"YakitPlugin": DefaultCliApp.YakitPlugin,
	"StringSlice": DefaultCliApp.StringSlice,
	"IntSlice":    DefaultCliApp.IntSlice,

	// 解析成 URL
	"Urls": DefaultCliApp.Urls,
	"Url":  DefaultCliApp.Url,

	// 解析端口
	"Ports": DefaultCliApp.Ports,
	"Port":  DefaultCliApp.Port,

	// 解析网络目标
	"Hosts":   DefaultCliApp.Hosts,
	"Host":    DefaultCliApp.Host,
	"Network": DefaultCliApp.Network,
	"Net":     DefaultCliApp.Net,

	// 解析文件之类的
	"File":          DefaultCliApp.File,
	"FileNames":     DefaultCliApp.FileNames,
	"FolderName":    DefaultCliApp.FolderName,
	"FileOrContent": DefaultCliApp.FileOrContent,
	"LineDict":      DefaultCliApp.LineDict,
	"Json":          DefaultCliApp.Json,

	// 设置param属性
	"setHelp":      DefaultCliApp.SetHelp,
	"setShortName": DefaultCliApp.SetShortName,
	"setDefault":   DefaultCliApp.SetDefault,
	"setRequired":  DefaultCliApp.SetRequired,
	"setPluginEnv": DefaultCliApp.SetPluginEnv,
	// 设置中文名
	"setVerboseName": DefaultCliApp.SetVerboseName,
	// 设置参数组名
	"setCliGroup": DefaultCliApp.SetCliGroup,
	// 设置Yakit payload
	"setYakitPayload": DefaultCliApp.SetYakitPayload,
	// 设置是否多选 (只支持`cli.StringSlice`)
	"setMultipleSelect": DefaultCliApp.SetMultipleSelect,
	// 设置下拉框选项 (只支持`cli.StringSlice`)
	"setSelectOption": DefaultCliApp.SetSelectOption,
	"setJsonSchema":   DefaultCliApp.SetJsonSchema,

	// UI Info
	"UI":           DefaultCliApp.UI,
	"showGroup":    DefaultCliApp.showGroup,
	"showParams":   DefaultCliApp.showParams,
	"hideGroup":    DefaultCliApp.hideGroup,
	"hideParams":   DefaultCliApp.hideParams,
	"whenTrue":     DefaultCliApp.whenTrue,
	"whenFalse":    DefaultCliApp.whenFalse,
	"whenEqual":    DefaultCliApp.whenEqual,
	"whenNotEqual": DefaultCliApp.whenEqual,
	"whenDefault":  DefaultCliApp.whenDefault,
	"when":         DefaultCliApp.when,

	// UI Schema
	"setUISchema":           DefaultCliApp.SetUISchema,
	"uiGlobalFieldPosition": DefaultCliApp.SetUISchemaGlobalFieldPosition,
	"uiGroups":              DefaultCliApp.SetUISchemaGroups,
	"uiGroup":               DefaultCliApp.NewUISchemaGroup,
	"uiTableField":          DefaultCliApp.NewUISchemaTableField,
	"uiField":               DefaultCliApp.NewUISchemaField,
	"uiFieldPosition":       DefaultCliApp.SetUISchemaFieldPosition,
	"uiFieldComponentStyle": DefaultCliApp.SetUISchemaFieldComponentStyle,
	"uiFieldWidget":         DefaultCliApp.SetUISchemaFieldWidget,
	"uiFieldGroups":         DefaultCliApp.SetUISchemaInnerGroups,
	"uiPosDefault":          UISchemaFieldPosDefault,
	"uiPosHorizontal":       UISchemaFieldPosHorizontal,
	"uiWidgetTable":         UISchemaWidgetTable,
	"uiWidgetRadio":         UISchemaWidgetRadio,
	"uiWidgetSelect":        UISchemaWidgetSelect,
	"uiWidgetCheckbox":      UISchemaWidgetCheckbox,
	"uiWidgetTextarea":      UISchemaWidgetTextArea,
	"uiWidgetPassword":      UISchemaWidgetPassword,
	// "uiWidgetColor":         UISchemaWidgetColor,
	// "uiWidgetEmail":         UISchemaWidgetEmail,
	// "uiWidgetUri":           UISchemaWidgetUri,
	// "uiWidgetDate":          UISchemaWidgetDate,
	// "uiWidgetDateTime":      UISchemaWidgetDateTime,
	// "uiWidgetTime":          UISchemaWidgetTime,
	"uiWidgetUpdown": UISchemaWidgetUpdown,
	// "uiWidgetRange":         UISchemaWidgetRange,
	"uiWidgetFile":   UISchemaWidgetFile,
	"uiWidgetFiles":  UISchemaWidgetFiles,
	"uiWidgetFolder": UISchemaWidgetFolder,

	// 设置cli属性
	"SetCliName": DefaultCliApp.SetCliName,
	"SetDoc":     DefaultCliApp.SetDoc,

	// 通用函数
	"help":  DefaultCliApp.Help,
	"check": DefaultCliApp.Check,
}
