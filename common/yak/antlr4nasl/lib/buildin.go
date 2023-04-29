package lib

import (
	"fmt"
	"math"
	"net"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib/codec"
	"reflect"
	"strings"
	"time"
)

// 这里是与上下文无关的函数
var preferences = map[string]interface{}{
	"optimize_test": false,
}

//	libs := map[string][]*yakvm.Code{
//		""
//	}
type ExitSignal struct {
	Flag interface{}
}

var oid = codec.Md5("code")
var kbs = map[string]interface{}{}
var NaslBuildInNativeMethod = map[string]interface{}{
	"sleep": func(n int) {
		time.Sleep(time.Duration(n) * time.Second)
	},
	"toupper": func(s string) string {
		return strings.ToUpper(s)
	},
	"keys": func(v map[string]interface{}) []string {
		var keys []string
		for k := range v {
			keys = append(keys, k)
		}
		return keys
	},
	"get_host_ip": func() string {
		return ""
	},

	"get_kb_list": func(s string) map[string]interface{} {
		res := map[string]interface{}{}
		for k, v := range kbs {
			if utils.MatchAllOfGlob(k, s) {
				res[k] = v
			}
		}
		return res
	},
	"string": func(i interface{}) string {
		return utils.InterfaceToString(i)
	},
	"display": func(i ...interface{}) {
		s := ""
		for _, i2 := range i {
			s += utils.InterfaceToString(i2)
		}
		println(s)
	},

	"isnull": func(i interface{}) bool {
		return i == nil
	},
	"get_script_oid": func() string {
		return oid
	},
	"__split": func(s string, sep string, keep bool) []string {
		return strings.Split(s, sep)
	},
	"max_index": func(i interface{}) int {
		refV := reflect.ValueOf(i)
		if refV.Type().Kind() == reflect.Array || refV.Type().Kind() == reflect.Slice {
			return refV.Len() - 1
		}
		return -1
	},
	"__set_kb_item": Set_kb_item,
	"__get_kb_item": func(k string) interface{} {
		if v, ok := kbs[k]; ok {
			return v
		}
		return nil
	},

	"reEqual": func(s1, s2 string) bool { // 内置=~运算符号
		return utils.MatchAllOfRegexp(s1, s2)
	},
	"strIn": func(s1, s2 string) bool { // 内置><运算符号
		return strings.Contains(s1, s2)
	},
	"RightShiftLogical": func(s1, s2 int64) uint64 { // 内置>>>运算符号
		return uint64(s1) >> s2
	},
	"BitNot": func(a int64) int64 { // 内置>>>运算符号
		return ^a
	},
	"__NewIterator": NewIterator, // ForEach
	"__pow": func(a, b float64) float64 {
		return math.Pow(a, b)
	},
	"chomp": func(s string) string {
		return strings.TrimRight(s, "\n")
	},

	"replace_kb_item": func(name string, v interface{}) {
		kbs[name] = v
	},
	"close": func(conn net.Conn) {
		if err := conn.Close(); err != nil {
			log.Errorf("close conn error: %v", err)
		}
	},
	"debug_print": func(items ...interface{}) {
		fmt.Print(items...)
	},
	"stridx": func(s1, s2 string) int {
		return strings.Index(s1, s2)
	},
	"tolower": func(s string) string {
		return strings.ToLower(s)
	},
}
