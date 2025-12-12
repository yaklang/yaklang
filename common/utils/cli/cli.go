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
// Example:
// ```
// cli.help()
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
// Example:
// ```
// target = cli.String("target", cli.SetRequired(true))
// cli.check()
// ```
func (c *CliApp) Check() {
	c.CliCheckFactory(c.cliCheckCallback)()
}

// SetCliName 设置此命令行程序的名称
// 这会在命令行输入 --help 或执行`cli.check()`后参数非法时显示
// Example:
// ```
// cli.SetCliName("example-tools")
// ```
func (c *CliApp) SetCliName(name string) {
	c.appName = name
}

// SetDoc 设置此命令行程序的文档
// 这会在命令行输入 --help 或执行`cli.check()`后参数非法时显示
// Example:
// ```
// cli.SetDoc("example-tools is a tool for example")
// ```
func (c *CliApp) SetDoc(document string) {
	c.document = document
}

// setDefault 是一个选项函数，设置参数的默认值
// Example:
// ```
// cli.String("target", cli.SetDefault("yaklang.com"))
// ```
func (c *CliApp) SetDefault(i interface{}) SetCliExtraParam {
	return func(c *cliExtraParams) {
		c.defaultValue = i
	}
}

// setHelp 是一个选项函数，设置参数的帮助信息
// 这会在命令行输入 --help 或执行`cli.check()`后参数非法时显示
// Example:
// ```
// cli.String("target", cli.SetHelp("target host or ip"))
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
// Example:
// ```
// cli.String("target", cli.SetRequired(true))
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
// Example:
// ```
// Args = cli.Args()
// ```
func (c *CliApp) GetArgs() []string {
	return c.args
}

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
// Example:
// ```
// verbose = cli.Bool("verbose") // --verbose 则为true
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
// Example:
// ```
// verbose = cli.Have("verbose") // --verbose 则为true
// ```
func (c *CliApp) Have(name string, opts ...SetCliExtraParam) bool {
	return c.Bool(name, opts...)
}

// String 获取对应名称的命令行参数，并将其转换为 string 类型返回
// Example:
// ```
// target = cli.String("target") // --target yaklang.com 则 target 为 yaklang.com
// ```
func (c *CliApp) String(name string, opts ...SetCliExtraParam) string {
	s, p := c._cliFromString(name, opts...)
	p._type = "string"
	return s
}

// HTTPPacket 获取对应名称的命令行参数，并将其转换为 string 类型返回
// 其作为一个独立脚本运行时与 cli.String 没有区别，仅在 Yakit 图形化中展示为 HTTP 报文形式
// Example:
// ```
// target = cli.HTTPPacket("target") // --target yaklang.com 则 target 为 yaklang.com
// ```
func (c *CliApp) HTTPPacket(name string, opts ...SetCliExtraParam) string {
	return c.String(name, opts...)
}

// YakCode 获取对应名称的命令行参数，并将其转换为 string 类型返回
// 其作为一个独立脚本运行时与 cli.String 没有区别，仅在 Yakit 图形化中展示为 Yak 代码形式
// Example:
// ```
// target = cli.YakCode("target") // --target yaklang.com 则 target 为 yaklang.com
// ```
func (c *CliApp) YakCode(name string, opts ...SetCliExtraParam) string {
	return c.String(name, opts...)
}

// Text 获取对应名称的命令行参数，并将其转换为 string 类型返回
// 其作为一个独立脚本运行时与 cli.String 没有区别，仅在 Yakit 图形化中展示为文本框形式
// Example:
// ```
// target = cli.Text("target") // --target yaklang.com 则 target 为 yaklang.com
// ```
func (c *CliApp) Text(name string, opts ...SetCliExtraParam) string {
	return c.String(name, opts...)
}

// Int 获取对应名称的命令行参数，并将其转换为 int 类型返回
// Example:
// ```
// port = cli.Int("port") // --port 80 则 port 为 80
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
// Example:
// ```
// port = cli.Integer("port") // --port 80 则 port 为 80
// ```
func (c *CliApp) Integer(name string, opts ...SetCliExtraParam) int {
	return c.Int(name, opts...)
}

// Float 获取对应名称的命令行参数，并将其转换为 float 类型返回
// Example:
// ```
// percent = cli.Float("percent") // --percent 0.5 则 percent 为 0.5
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
// Example:
// ```
// percent = cli.Double("percent") // --percent 0.5 则 percent 为 0.5
// ```
func (c *CliApp) Double(name string, opts ...SetCliExtraParam) float64 {
	return c.Float(name, opts...)
}

// Urls 获取对应名称的命令行参数，根据","切割并尝试将其转换为符合URL格式并返回 []string 类型
// Example:
// ```
// urls = cli.Urls("urls")
// // --urls yaklang.com:443,google.com:443 则 urls 为 ["https://yaklang.com", "https://google.com"]
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
// Example:
// ```
// urls = cli.Url("urls")
// // --urls yaklang.com:443,google.com:443 则 urls 为 ["https://yaklang.com", "https://google.com"]
// ```
func (c *CliApp) Url(name string, opts ...SetCliExtraParam) []string {
	return c.Urls(name, opts...)
}

// Ports 获取对应名称的命令行参数，根据","与"-"切割并尝试解析端口并返回 []int 类型
// Example:
// ```
// ports = cli.Ports("ports")
// // --ports 10086-10088,23333 则 ports 为 [10086, 10087, 10088, 23333]
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
// Example:
// ```
// ports = cli.Port("ports")
// // --ports 10086-10088,23333 则 ports 为 [10086, 10087, 10088, 23333]
// ```
func (c *CliApp) Port(name string, opts ...SetCliExtraParam) []int {
	return c.Ports(name, opts...)
}

// Hosts 获取对应名称的命令行参数，根据","切割并尝试解析CIDR网段并返回 []string 类型
// Example:
// ```
// hosts = cli.Hosts("hosts")
// // --hosts 192.168.0.0/24,172.17.0.1 则 hosts 为 192.168.0.0/24对应的所有IP和172.17.0.1
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
// Example:
// ```
// hosts = cli.Host("hosts")
// // --hosts 192.168.0.0/24,172.17.0.1 则 hosts 为 192.168.0.0/24对应的所有IP和172.17.0.1
// ```
func (c *CliApp) Host(name string, opts ...SetCliExtraParam) []string {
	return c.Hosts(name, opts...)
}

// NetWork 获取对应名称的命令行参数，根据","切割并尝试解析CIDR网段并返回 []string 类型
// Example:
// ```
// hosts = cli.NetWork("hosts")
// // --hosts 192.168.0.0/24,172.17.0.1 则 hosts 为 192.168.0.0/24对应的所有IP和172.17.0.1
// ```
func (c *CliApp) Network(name string, opts ...SetCliExtraParam) []string {
	return c.Hosts(name, opts...)
}

// Net 获取对应名称的命令行参数，根据","切割并尝试解析CIDR网段并返回 []string 类型
// Example:
// ```
// hosts = cli.Net("hosts")
// // --hosts 192.168.0.0/24,172.17.0.1 则 hosts 为 192.168.0.0/24对应的所有IP和172.17.0.1
// ```
func (c *CliApp) Net(name string, opts ...SetCliExtraParam) []string {
	return c.Hosts(name, opts...)
}

// File 获取对应名称的命令行参数，根据其传入的值读取其对应文件内容并返回 []byte 类型
// Example:
// ```
// file = cli.File("file")
// // --file /etc/passwd 则 file 为 /etc/passwd 文件中的内容
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
// Example:
// ```
// file = cli.FileNames("file")
// // --file /etc/passwd,/etc/hosts 则 file 为 ["/etc/passwd", "/etc/hosts"]
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
// Example:
// ```
// folder = cli.FolderName("folder")
// // --folder /etc 则 folder 为 "/etc"
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
// Example:
// ```
// foc = cli.FileOrContent("foc")
// // --foc /etc/passwd 则 foc 为 /etc/passwd 文件中的内容
// // --file "asd" 则 file 为 "asd"
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
// Example:
// ```
// dict = cli.LineDict("dict")
// // --dict /etc/passwd 则 dict 为 /etc/passwd 文件中的逐行的内容
// // --dict "asd" 则 dict 为 ["asd"]
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
// info = cli.Json("info",
// cli.setVerboseName("项目信息"),
// cli.setJsonSchema(<<<JSON
// {"title":"A registration form","description":"A simple form example.","type":"object","required":["firstName","lastName"],"properties":{"name":{"type":"string","title":"Name","default":"Chuck"},"password":{"type":"string","title":"Password","minLength":3},"telephone":{"type":"string","title":"Telephone","minLength":10}}}
// JSON,cli.setUISchema()),
// )
// cli.check()
// ```
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
// Example:
// ```
// plugins = cli.YakitPlugin()
// // --yakit-plugin-file plugins.txt 则 plugins 为 plugins.txt 文件中的各个插件名
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
// Example:
// ```
// targets = cli.StringSlice("targets")
// // --targets yaklang.com,google.com 则 targets 为 ["yaklang.com", "google.com"]
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
// Example:
// ```
// ports = cli.IntSlice("ports")
// // --ports 80,443,8080 则 ports 为 [80, 443, 8080]
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
// Example:
// ```
// cli.String("target", cli.setVerboseName("目标"))
// ```
func (c *CliApp) SetVerboseName(verboseName string) SetCliExtraParam {
	return func(cep *cliExtraParams) {}
}

// setCliGroup 是一个选项函数，设置参数的分组
// Example:
// ```
// cli.String("target", cli.setCliGroup("common"))
// cli.Int("port", cli.setCliGroup("common"))
// cli.Int("threads", cli.setCliGroup("request"))
// cli.Int("retryTimes", cli.setCliGroup("request"))
// ```
func (c *CliApp) SetCliGroup(group string) SetCliExtraParam {
	return func(cep *cliExtraParams) {}
}

// setYakitPayload 是一个选项函数，设置参数建议值为Yakit payload的字典名列表
// Example:
// ```
// cli.String("dictName", cli.setYakitPayload(true))
// ```
func (c *CliApp) SetYakitPayload(b bool) SetCliExtraParam {
	return func(c *cliExtraParams) {
	}
}

// setShortName 是一个选项函数，设置参数的短名称
// Example:
// ```
// cli.String("target", cli.setShortName("t"))
// ```
// 在命令行可以使用`-t`代替`--target`
func (c *CliApp) SetShortName(shortName string) SetCliExtraParam {
	return func(c *cliExtraParams) {
		c.optShortName = shortName
	}
}

// SetMultipleSelect 是一个选项函数，设置参数是否可以多选
// 此选项仅在`cli.StringSlice`中生效
// Example:
// ```
// cli.StringSlice("targets", cli.SetMultipleSelect(true))
// ```
func (c *CliApp) SetMultipleSelect(multiSelect bool) SetCliExtraParam {
	return func(cep *cliExtraParams) {}
}

// setSelectOption 是一个选项函数，设置参数的下拉框选项
// 此选项仅在`cli.StringSlice`中生效
// Example:
// ```
// cli.StringSlice("targets", cli.setSelectOption("下拉框选项", "下拉框值"))
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
// info = cli.Json("info",
// cli.setVerboseName("项目信息"),
// cli.setJsonSchema(<<<JSON
// {"title":"A registration form","description":"A simple form example.","type":"object","required":["firstName","lastName"],"properties":{"name":{"type":"string","title":"Name","default":"Chuck"},"password":{"type":"string","title":"Password","minLength":3},"telephone":{"type":"string","title":"Telephone","minLength":10}}}
// JSON,cli.setUISchema()),
// )
// cli.check()
// ```
func (c *CliApp) SetJsonSchema(schema string, uis ...*UISchema) SetCliExtraParam {
	return func(c *cliExtraParams) {}
}

// setPluginEnv 是一个选项函数，设置参数从插件环境中取值
// Example:
// ```
// cli.String("key", cli.setPluginEnv("api-key"))
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

func (c *CliApp) UI(opts ...UIParams) {
}

func (c *CliApp) showGroup(group string) UIParams {
	return defaultUIParams
}

func (c *CliApp) showParams(params ...string) UIParams {
	return defaultUIParams
}

func (c *CliApp) hideGroup(group string) UIParams {
	return defaultUIParams
}

func (c *CliApp) hideParams(params ...string) UIParams {
	return defaultUIParams
}

func (c *CliApp) whenTrue(param string) UIParams {
	return defaultUIParams
}

func (c *CliApp) whenFalse(param string) UIParams {
	return defaultUIParams
}

func (c *CliApp) whenEqual(param string, value string) UIParams {
	return defaultUIParams
}

func (c *CliApp) whenNotEqual(param string, value string) UIParams {
	return defaultUIParams
}

func (c *CliApp) when(expression string) UIParams {
	return defaultUIParams
}

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
