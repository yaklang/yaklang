package yaklib

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklang/spec"
	"github.com/yaklang/yaklang/common/yakdocument"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
)

// f 用于对字符串进行格式化
// Example:
// ```
//
// str.f("hello %s", "yak") // hello yak
// ```
func _sfmt(f string, items ...interface{}) string {
	return fmt.Sprintf(f, items...)
}

// assert 用于判断传入的布尔值是否为真，如果为假则崩溃并打印错误信息
// ! 已弃用，可以使用 assert 语句代替
// Example:
// ```
// assert(code == 200, "code != 200") // 如果code不等于200则会崩溃并打印错误信息
// // 其相当于 assert code == 200, "code != 200"
// ```
func _assert(b bool, reason ...interface{}) {
	if !b {
		panic(spew.Sdump(reason))
	}
}

// assertf 用于判断传入的布尔值是否为真，如果为假则崩溃并打印错误信息
// ! 已弃用，可以使用 assert 语句代替
// Example:
// ```
// assertf(code == 200, "code != %d", 200) // 如果code不等于200则会崩溃并打印错误信息
// // 其相当于 assert code == 200, sprintf("code != %d", 200)
// ```
func _assertf(b bool, f string, items ...any) {
	if !b {
		panic(_sfmt(f, items...))
	}
}

// assertEmpty 用于判断传入的值是否为空，如果为空则崩溃并打印错误信息
// ! 已弃用，可以使用 assert 语句代替
// Example:
// ```
// assertEmpty(nil, "nil is not empty") // 如果nil不为空则会崩溃并打印错误信息，这里不会崩溃
// ```
func _assertEmpty(i interface{}) {
	if i == nil || i == spec.Undefined {
		return
	}
	panic(_sfmt("expect nil but got %v", spew.Sdump(i)))
}

// fail 崩溃并打印错误信息，其实际上几乎等价于panic
// Example:
// ```
// try{
// 1/0
// } catch err {
// fail("exit code:", 1, "because:", err)
// }
// ```
func _failed(msg ...interface{}) {
	if msg == nil {
		panic("exit")
	}

	var msgs []string
	for _, i := range msg {
		if err, ok := i.(error); ok {
			msgs = append(msgs, err.Error())
		} else if s, ok := i.(string); ok {
			msgs = append(msgs, s)
		} else {
			msgs = append(msgs, spew.Sdump(i))
		}
	}
	panic(strings.Join(msgs, "\n"))
}

func yakitOutputHelper(i interface{}) {
	if yakitClientInstance != nil {
		yakitClientInstance.Output(i)
	}
}

// die 判断传入的错误是否为空，如果不为空则崩溃并打印错误信息，其实际上相当于 if err != nil { panic(err) }
// Example:
// ```
// die(err)
// ```
func _diewith(err interface{}) {
	if err == nil {
		return
	}
	yakitOutputHelper(fmt.Sprintf("YakVM Code DIE With Data: %v", spew.Sdump(err)))
	_failed(err)
}

// logdiscard 用于丢弃所有日志，即不再显示任何日志
// Example:
// ```
// logdiscard()
// ```
func _logDiscard() {
	log.SetOutput(io.Discard)
}

// logquiet 用于丢弃所有日志，即不再显示任何日志，它是logdiscard的别名
// Example:
// ```
// logquiet()
// ```
func _logQuiet() {
	log.SetOutput(io.Discard)
}

// logrecover 用于恢复日志的显示，它用于恢复logdiscard所造成的效果
// Example:
// ```
// logdiscard()
// logrecover()
// ```
func _logRecover() {
	log.SetOutput(os.Stdout)
}

func dummyN(items ...any) {
	if len(items) > 0 {
		fmt.Println(fmt.Sprintf(utils.InterfaceToString(items[0]), items[1:]...))
	}
}

// yakit_output 用于在yakit中输出日志，在非yakit的情况下它会在控制台中输出日志，在mitm插件中调用则会在被动日志中输出日志
// Example:
// ```
// yakit_output("hello %s", "yak")
// ```
func _yakit_output(items ...any) {
	if len(items) > 0 {
		fmt.Println(fmt.Sprintf(utils.InterfaceToString(items[0]), items[1:]...))
	}
}

// yakit_save
// ! 已弃用
func _yakit_save(items ...any) {
}

// yakit_status
// ! 已弃用
func _yakit_status(items ...any) {
}

// uuid 用于生成一个uuid字符串
// Example:
// ```
// println(uuid())
// ```
func _uuid() string {
	return uuid.New().String()
}

// timestamp 用于获取当前时间戳，其返回值为int64类型
// Example:
// ```
// println(timestamp())
// ```
func _timestamp() int64 {
	return time.Now().Unix()
}

// nanotimestamp 用于获取当前时间戳，精确到纳秒，其返回值为int64类型
// Example:
// ```
// println(nanotimestamp())
// ```
func _nanotimestamp() int64 {
	return time.Now().UnixNano()
}

// date 用于获取当前日期，其格式为"2006-01-02“
// Example:
// ```
// println(date())
// ```
func _date() string {
	return time.Now().Format("2006-01-02")
}

// datetime 用于获取当前日期与时间，其格式为"2006-01-02 15:04:05"
// Example:
// ```
// println(datetime())
// ```
func _datetime() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// now 用于获取当前时间的时间结构体
// 它实际上是 time.Now 的别名
// Example:
// ```
// dur = time.ParseDuration("1m")~
// ctx, cancel = context.WithDeadline(context.New(), now().Add(dur))
//
// println(now().Format("2006-01-02 15:04:05"))
// ```
func _now() time.Time {
	return time.Now()
}

// timestampToDatetime 用于将时间戳转换为日期与时间，其格式为"2006-01-02 15:04:05"
// Example:
// ```
// println(timestampToDatetime(timestamp()))
// ```
func _timestampToDatetime(tValue int64) string {
	tm := time.Unix(tValue, 0)
	return tm.Format("2006-01-02 15:04:05")
}

// timestampToTime 用于将时间戳转换为时间结构体
// Example:
// ```
// println(timestampToDatetime(timestamp()))
// ```
func _timestampToTime(tValue int64) time.Time {
	return time.Unix(tValue, 0)
}

// datetimeToTimestamp 用于将日期与时间字符串转换为时间戳，其格式为"2006-01-02 15:04:05"
// Example:
// ```
// println(datetimeToTimestamp("2023-11-11 11:11:11")~)
// ```
func _datetimeToTimestamp(str string) (int64, error) {
	t, err := time.Parse("2006-01-02 15:04:05", str)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

// parseTime 以一个布局解析一个格式化的时间字符串并返回它代表的时间结构体。
// Example:
// ```
// t, err = parseTime("2006-01-02 15:04:05", "2023-11-11 11:11:11")
// ```
func _parseTime(layout, value string) (time.Time, error) {
	return time.Parse(layout, value)
}

// dump 以用户友好的方式格式化并打印任意类型的数据
// Example:
// ```
// dump("hello", 1, ["1", 2, "3"])
// ```
func _dump(i ...any) {
	spew.Dump(i...)
}

// sdump 以用户友好的方式格式化任意类型的数据，返回格式化后的字符串
// Example:
// ```
// println(sdump("hello", 1, ["1", 2, "3"]))
// ```
func _sdump(i ...any) string {
	return spew.Sdump(i...)
}

// randn 用于生成一个随机数，其范围为[min, max)
// 如果min大于max，则会抛出异常
// Example:
// ```
// println(randn(1, 100))
// ```
func _randn(min, max int) int {
	if min > max {
		panic(_sfmt("randn failed; min: %v max: %v", min, max))
	}
	return min + rand.Intn(max-min)
}

// randstr 返回在大小写字母表中随机挑选 n 个字符组成的字符串
// Example:
// ```
// println(randstr(10))
// ```
func _randstr(length int) string {
	return utils.RandStringBytes(length)
}

// wait 用于等待一个上下文完成，或者让当前协程休眠一段时间，其单位为秒
// Example:
// ```
// ctx, cancel = context.WithTimeout(context.New(), time.ParseDuration("5s")~) // 上下文在调用cancel函数或者5秒后完成
// wait(ctx) // 等待上下文完成
// wait(1.5) // 休眠1.5秒
// ```
func _wait(i interface{}) {
	switch ret := i.(type) {
	case context.Context:
		select {
		case <-ret.Done():
		}
	case string:
		sleep(parseFloat(ret))
	case float64:
		sleep(ret)
	case float32:
		sleep(float64(ret))
	case int:
		sleep(float64(ret))
	default:
		panic(fmt.Sprintf("cannot wait %v", spew.Sdump(ret)))
	}
}

// isEmpty 用于判断传入的值是否为空，如果为空则返回true，否则返回false
// Example:
// ```
// isEmpty(nil) // true
// isEmpty(1) // false
// ```
func _isEmpty(i interface{}) bool {
	if i == nil || i == spec.Undefined {
		return true
	}
	return false
}

// chr 将传入的值根据ascii码表转换为对应的字符
// Example:
// ```
// chr(65) // A
// chr("65") // A
// ```
func chr(i interface{}) string {
	switch v := i.(type) {
	case int:
		return string(rune(v))
	case int8:
		return string(rune(v))
	case int16:
		return string(rune(v))
	case int32:
		return string(rune(v))
	case int64:
		return string(rune(v))
	case uint:
		return string(rune(v))
	case uint8:
		return string(rune(v))
	case uint16:
		return string(rune(v))
	case uint32:
		return string(rune(v))
	case uint64:
		return string(rune(v))
	default:
		return string(rune(parseInt(utils.InterfaceToString(i))))
	}
}

// ord  将传入的值转换为对应的ascii码整数
// Example:
// ```
// ord("A") // 65
// ord('A') // 65
// ```
func ord(i interface{}) int {
	switch ret := i.(type) {
	case rune:
		return int(ret)
	case byte:
		return int(ret)
	default:
		strRaw := utils.InterfaceToString(i)
		if strRaw == "" {
			return -1
		}

		if r := []rune(strRaw); r != nil {
			return int(r[0])
		}

		return int(strRaw[0])
	}
}

// typeof 用于获取传入值的类型结构体
// Example:
// ```
// typeof(1) == int // true
// typeof("hello") == string // true
// ```
func typeof(i interface{}) reflect.Type {
	return reflect.TypeOf(i)
}

// desc 以用户友好的方式打印传入的复杂值的详细信息，其往往是一个结构体或者一个结构体引用，详细信息包括可用字段，可用的成员方法
// Example:
// ```
// ins = fuzz.HTTPRequest(poc.BasicRequest())~
// desc(ins)
// ```
func _desc(i interface{}) {
	info, err := yakdocument.Dir(i)
	if err != nil {
		log.Error(err)
		return
	}
	if info == nil {
		return
	}
	info.Show()
}

// descStr 以用户友好的方式打印传入的复杂值的详细信息，其往往是一个结构体或者一个结构体引用，详细信息包括可用字段，可用的成员方法，返回详细信息的字符串
// Example:
// ```
// ins = fuzz.HTTPRequest(poc.BasicRequest())~
// println(descStr(ins))
// ```
func _descToString(i interface{}) string {
	info, err := yakdocument.Dir(i)
	if err != nil {
		log.Error(err)
		return ""
	}
	if info == nil {
		return ""
	}
	return info.String()
}

// tick1s 用于每隔1秒执行一次传入的函数，直到函数返回false为止
// Example:
// ```
// count = 0
// tick1s(func() bool {
// println("hello")
// count++
// return count <= 5
// })
// ```
func tick1s(f func() bool) {
	t := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-t.C:
			if !f() {
				return
			}
		}
	}
}

// sleep 用于让当前协程休眠一段时间，其单位为秒
// Example:
// ```
// sleep(1.5) // 休眠1.5秒
// ```
func sleep(i float64) {
	time.Sleep(utils.FloatSecondDuration(i))
}

var GlobalExport = map[string]interface{}{
	"_createOnLogger":        createLogger,
	"_createOnLoggerConsole": createConsoleLogger,
	"_createOnFailed":        createFailed,
	"_createOnOutput":        createOnOutput,
	"_createOnFinished":      createOnFinished,
	"_createOnAlert":         createOnAlert,

	"loglevel":   setLogLevel,
	"logquiet":   _logDiscard,
	"logdiscard": _logDiscard,
	"logrecover": _logRecover,

	"yakit_output": _yakit_output,
	"yakit_save":   _yakit_save,
	"yakit_status": _yakit_status,

	"fail": _failed,
	"die":  _diewith,
	"uuid": _uuid,

	"timestamp":           _timestamp,
	"nanotimestamp":       _nanotimestamp,
	"datetime":            _datetime,
	"date":                _date,
	"now":                 _now,
	"parseTime":           _parseTime,
	"timestampToDatetime": _timestampToDatetime,
	"timestampToTime":     _timestampToTime,
	"datetimeToTimestamp": _datetimeToTimestamp,
	"tick1s":              tick1s,

	"input": _input,
	"dump":  _dump,
	"sdump": _sdump,

	"randn":   _randn,
	"randstr": _randstr,

	"assert":      _assert,
	"assertTrue":  _assert,
	"isEmpty":     _isEmpty,
	"assertEmpty": _assertEmpty,
	"assertf":     _assertf,

	"parseInt":     parseInt,
	"parseStr":     parseString,
	"parseString":  parseString,
	"parseBool":    parseBool,
	"parseBoolean": parseBool,
	"parseFloat":   parseFloat,
	"atoi":         atoi,

	"sleep": sleep,
	"wait":  _wait,

	"desc":     _desc,
	"descStr":  _descToString,
	"chr":      chr,
	"ord":      ord,
	"type":     typeof,
	"typeof":   typeof,
	"callable": IsYakFunction,
}
