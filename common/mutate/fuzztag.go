package mutate

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/regen"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yso"
	"io/ioutil"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/fuzztag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// 空内容
var fuzztagfallback = []string{""}

type FuzzTagDescription struct {
	TagName          string
	Handler          func(string) []string
	HandlerEx        func(string) []*fuzztag.FuzzExecResult
	ErrorInfoHandler func(string) ([]string, error)
	IsDyn            bool
	IsDynFun         func(name, params string) bool
	Alias            []string
	Description      string
}

func AddFuzzTagDescriptionToMap(methodMap map[string]*parser.TagMethod, f *FuzzTagDescription) {
	if f == nil {
		return
	}
	name := f.TagName
	alias := f.Alias
	var expand map[string]any
	if f.IsDynFun != nil {
		expand = map[string]any{
			"IsDynFun": f.IsDynFun,
		}
	}
	methodMap[name] = &parser.TagMethod{
		Name:   name,
		IsDyn:  f.IsDyn,
		Expand: expand,
		Fun: func(s string) (result []*parser.FuzzResult, err error) {
			defer func() {
				if r := recover(); r != nil {
					if v, ok := r.(error); ok {
						err = v
					} else {
						err = errors.New(utils.InterfaceToString(r))
					}
				}
			}()
			if f.Handler != nil {
				for _, d := range f.Handler(s) {
					result = append(result, parser.NewFuzzResultWithData(d))
				}
			} else if f.HandlerEx != nil {
				for _, data := range f.HandlerEx(s) {
					var verbose string
					showInfo := data.ShowInfo()
					if len(showInfo) != 0 {
						verbose = utils.InterfaceToString(showInfo)
					}
					result = append(result, parser.NewFuzzResultWithDataVerbose(data.Data(), verbose))
				}
			} else if f.ErrorInfoHandler != nil {
				res, err := f.ErrorInfoHandler(s)
				fuzzRes := []*parser.FuzzResult{}
				for _, r := range res {
					fuzzRes = append(fuzzRes, parser.NewFuzzResultWithData(r))
				}
				return fuzzRes, err
			} else {
				return nil, errors.New("no handler")
			}
			return
		},
	}
	for _, a := range alias {
		methodMap[a] = methodMap[name]
	}
}

var existedFuzztag []*FuzzTagDescription
var tagMethodMap = map[string]*parser.TagMethod{}

func AddFuzzTagToGlobal(f *FuzzTagDescription) {
	AddFuzzTagDescriptionToMap(tagMethodMap, f)
}
func init() {
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "trim",
		Handler: func(s string) []string {
			return []string{strings.TrimSpace(s)}
		},
		Description: "移除前后多余的空格，例如：`{{trim( abc )}}`，结果为：`abc`",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "substr",
		Handler: func(s string) []string {
			index := strings.LastIndexByte(s, '|')
			if index == -1 {
				return []string{s}
			}
			before, after := s[:index], s[index+1:]
			if strings.Contains(after, ",") {
				start, length := sepToEnd(after, ",")
				startInt := codec.Atoi(start)
				lengthInt := codec.Atoi(length)
				if lengthInt <= 0 {
					lengthInt = len(before)
				}
				if startInt >= len(before) {
					return []string{""}
				}
				if startInt+lengthInt >= len(before) {
					return []string{before[startInt:]}
				}
				return []string{before[startInt : startInt+lengthInt]}
			} else {
				start := codec.Atoi(after)
				if len(before) > start {
					return []string{before[start:]}
				}
				return []string{""}
			}
		},
		Description: "输出一个字符串的子符串，定义为 {{substr(abc|start,length)}}，例如：{{substr(abc|1)}}，结果为：bc，{{substr(abcddd|1,2)}}，结果为：bc",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "fuzz:password",
		Handler: func(s string) []string {
			origin, level := sepToEnd(s, "|")
			var levelInt = atoi(level)
			return fuzzpass(origin, levelInt)
		},
		Alias:       []string{"fuzz:pass"},
		Description: "根据所输入的操作随机生成可能的密码（默认为 root/admin 生成）",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "zlib:encode",
		Handler: func(s string) []string {
			res, _ := utils.ZlibCompress(s)
			return []string{
				string(res),
			}
		},
		Alias:       []string{"zlib:enc", "zlibc", "zlib"},
		Description: "Zlib 编码，把标签内容进行 zlib 压缩",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "zlib:decode",
		Handler: func(s string) []string {
			res, _ := utils.ZlibDeCompress([]byte(s))
			return []string{
				string(res),
			}
		},
		Alias:       []string{"zlib:dec", "zlibdec", "zlibd"},
		Description: "Zlib 解码，把标签内的内容进行 zlib 解码",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "gzip:encode",
		Handler: func(s string) []string {
			res, _ := utils.GzipCompress(s)
			return []string{
				string(res),
			}
		},
		Alias:       []string{"gzip:enc", "gzipc", "gzip"},
		Description: "Gzip 编码，把标签内容进行 gzip 压缩",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "gzip:decode",
		Handler: func(s string) []string {
			res, _ := utils.GzipDeCompress([]byte(s))
			return []string{
				string(res),
			}
		},
		Alias:       []string{"gzip:dec", "gzipdec", "gzipd"},
		Description: "Gzip 解码，把标签内的内容进行 gzip 解码",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "fuzz:username",
		Handler: func(s string) []string {
			origin, level := sepToEnd(s, "|")
			var levelInt = atoi(level)
			return fuzzuser(origin, levelInt)
		},
		Alias:       []string{"fuzz:user"},
		Description: "根据所输入的操作随机生成可能的用户名（默认为 root/admin 生成）",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "date",
		Handler: func(s string) []string {
			if s == "" {
				return []string{utils.JavaTimeFormatter(time.Now(), "YYYY-MM-dd")}
			}
			return []string{utils.JavaTimeFormatter(time.Now(), s)}
		},
		Description: "生成一个时间，格式为YYYY-MM-dd，如果指定了格式，将按照指定的格式生成时间",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "datetime",
		Handler: func(s string) []string {
			if s == "" {
				return []string{utils.JavaTimeFormatter(time.Now(), "YYYY-MM-dd")}
			}
			return []string{utils.JavaTimeFormatter(time.Now(), s)}
		},
		Alias:       []string{"time"},
		Description: "生成一个时间，格式为YYYY-MM-dd HH:mm:ss，如果指定了格式，将按照指定的格式生成时间",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "timestamp",
		IsDyn:   true,
		Handler: func(s string) []string {
			switch strings.ToLower(s) {
			case "s", "sec", "seconds", "second":
				return []string{fmt.Sprint(time.Now().Unix())}
			case "ms", "milli", "millis", "millisecond", "milliseconds":
				return []string{fmt.Sprint(time.Now().UnixMilli())}
			case "ns", "nano", "nanos", "nanosecond", "nanoseconds":
				return []string{fmt.Sprint(time.Now().UnixNano())}
			}
			return []string{fmt.Sprint(time.Now().Unix())}
		},
		Description: "生成一个时间戳，默认单位为秒，可指定单位：s, ms, ns: {{timestamp(s)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "uuid",
		Handler: func(s string) []string {
			var result = []string{}
			for i := 0; i < atoi(s); i++ {
				result = append(result, uuid.New().String())
			}
			if len(result) == 0 {
				return []string{uuid.New().String()}
			}
			return result
		},
		Description: "生成一个随机的uuid，如果指定了数量，将生成指定数量的uuid",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "trim",
		Handler: func(s string) []string {
			return []string{
				strings.TrimSpace(s),
			}
		},
		Description: "去除字符串两边的空格，一般配合其他 tag 使用，如：{{trim({{x(dict)}})}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "null",
		Handler: func(s string) []string {
			if ret := atoi(s); ret > 0 {
				return []string{
					strings.Repeat("\x00", ret),
				}
			}
			return []string{
				"\x00",
			}
		},
		Alias:       []string{"nullbyte"},
		Description: "生成一个空字节，如果指定了数量，将生成指定数量的空字节 {{null(5)}} 表示生成 5 个空字节",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "gb18030",
		Handler: func(s string) []string {
			g, err := codec.Utf8ToGB18030([]byte(s))
			if err != nil {
				return []string{s}
			}
			return []string{string(g)}
		},
		Description: `将字符串转换为 GB18030 编码，例如：{{gb18030(你好)}}`,
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "gb18030toUTF8",
		Handler: func(s string) []string {
			g, err := codec.GB18030ToUtf8([]byte(s))
			if err != nil {
				return []string{s}
			}
			return []string{string(g)}
		},
		Description: `将字符串转换为 UTF8 编码，例如：{{gb18030toUTF8({{hexd(c4e3bac3)}})}}`,
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "padding:zero",
		Handler: func(s string) []string {
			origin, paddingTotal := sepToEnd(s, "|")
			right := strings.HasPrefix(paddingTotal, "-")
			paddingTotal = strings.TrimLeft(paddingTotal, "-+")
			if ret := atoi(paddingTotal); ret > 0 {
				if right {
					return []string{
						strings.Repeat("0", ret-len(origin)) + origin,
					}
				}
				return []string{
					strings.Repeat("0", ret-len(origin)) + origin,
				}
			}
			return []string{origin}
		},
		Alias:       []string{"zeropadding", "zp"},
		Description: "使用0来填充补偿字符串长度不足的问题，{{zeropadding(abc|5)}} 表示将 abc 填充到长度为 5 的字符串（00abc），{{zeropadding(abc|-5)}} 表示将 abc 填充到长度为 5 的字符串，并且在右边填充 (abc00)",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "padding:null",
		Handler: func(s string) []string {
			origin, paddingTotal := sepToEnd(s, "|")
			right := strings.HasPrefix(paddingTotal, "-")
			paddingTotal = strings.TrimLeft(paddingTotal, "-+")
			if ret := atoi(paddingTotal); ret > 0 {
				if right {
					return []string{
						strings.Repeat("\x00", ret-len(origin)) + origin,
					}
				}
				return []string{
					strings.Repeat("\x00", ret-len(origin)) + origin,
				}
			}
			return []string{origin}
		},
		Alias:       []string{"nullpadding", "np"},
		Description: "使用 \\x00 来填充补偿字符串长度不足的问题，{{nullpadding(abc|5)}} 表示将 abc 填充到长度为 5 的字符串（\\x00\\x00abc），{{nullpadding(abc|-5)}} 表示将 abc 填充到长度为 5 的字符串，并且在右边填充 (abc\\x00\\x00)",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "char",
		Handler: func(s string) []string {
			if s == "" {
				return []string{""}
			}

			if !strings.Contains(s, "-") {
				log.Errorf("bad char params: %v, eg.: %v", s, "a-z")
				return []string{""}
			}

			ret := strings.Split(s, "-")
			switch len(ret) {
			case 2:
				p1, p2 := ret[0], ret[1]
				if !(len(p1) == 1 && len(p2) == 1) {
					log.Errorf("start char or end char is not char(1 byte): %v eg.: %v", s, "a-z")
					return []string{""}
				}

				p1Byte := []byte(p1)[0]
				p2Byte := []byte(p2)[0]
				var rets []string
				min, max := utils.MinByte(p1Byte, p2Byte), utils.MaxByte(p1Byte, p2Byte)
				for i := min; i <= max; i++ {
					rets = append(rets, string(i))
				}
				return rets
			default:
				log.Errorf("bad params[%s], eg.: %v", s, "a-z")
			}
			return []string{""}
		},
		Alias:       []string{"c", "ch"},
		Description: "生成一个字符，例如：`{{char(a-z)}}`, 结果为 [a b c ... x y z]",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "repeat",
		Handler: func(s string) []string {
			i := atoi(s)
			if i == 0 {
				chr, right := sepToEnd(s, "|")
				if repeatTimes := atoi(right); repeatTimes > 0 {
					return lo.Times(repeatTimes, func(index int) string {
						return chr
					})
				}
			} else {
				return make([]string, i)
			}
			return []string{""}
		},
		Description: "重复一个字符串或者一个次数，例如：`{{repeat(abc|3)}}`，结果为：abcabcabc，或者`{{repeat(3)}}`，结果是重复三次",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "repeat:range",
		Handler: func(s string) []string {
			origin, times := sepToEnd(s, "|")
			if ret := atoi(times); ret > 0 {
				var results = make([]string, ret+1)
				for i := 0; i < ret+1; i++ {
					if i == 0 {
						results[i] = ""
						continue
					}
					results[i] = strings.Repeat(origin, i)
				}
				return results
			}
			return []string{s}
		},
		Description: "重复一个字符串，并把重复步骤全都输出出来，例如：`{{repeat(abc|3)}}`，结果为：['' abc abcabc abcabcabc]",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "payload",
		Handler: func(s string) []string {
			var db = consts.GetGormProfileDatabase()
			if db == nil {
				return []string{s}
			}
			db = bizhelper.ExactQueryStringArrayOr(db, "`group`", utils.PrettifyListFromStringSplited(s, ","))
			//_ = db

			var payloads []string
			if rows, err := db.Table("payloads").Select("content").Rows(); err != nil {
				return []string{s}
			} else {
				for rows.Next() {
					var payloadRaw string
					err := rows.Scan(&payloadRaw)
					if err != nil {
						return payloads
					}
					raw, err := strconv.Unquote(payloadRaw)
					if err != nil {
						payloads = append(payloads, payloadRaw)
						continue
					}
					payloads = append(payloads, raw)
				}
				return payloads
			}
		},
		Alias:       []string{"x"},
		Description: "从数据库加载 Payload, `{{payload(pass_top25)}}`",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "array",
		Alias:       []string{"list"},
		Description: "设置一个数组，使用 `|` 分割，例如：`{{array(1|2|3)}}`，结果为：[1,2,3]，",
		Handler: func(s string) []string {
			return strings.Split(s, "|")
		},
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "ico",
		Handler:     func(s string) []string { return []string{"\x00\x00\x01\x00\x01\x00\x20\x20"} },
		Description: "生成一个 ico 文件头，例如 `{{ico}}`",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "tiff",
		Handler: func(s string) []string {
			return []string{"\x4d\x4d", "\x49\x49"}
		},
		Description: "生成一个 tiff 文件头，例如 `{{tiff}}`",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{TagName: "bmp", Handler: func(s string) []string { return []string{"\x42\x4d"} }, Description: "生成一个 bmp 文件头，例如 {{bmp}}"})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "gif",
		Handler:     func(s string) []string { return []string{"GIF89a"} },
		Description: "生成 gif 文件头",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "png",
		Handler: func(s string) []string {
			return []string{
				"\x89PNG" +
					"\x0d\x0a\x1a\x0a" +
					"\x00\x00\x00\x0D" +
					"IHDR\x00\x00\x00\xce\x00\x00\x00\xce\x08\x02\x00\x00\x00" +
					"\xf9\x7d\xaa\x93",
			}
		},
		Description: "生成 PNG 文件头",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "jpg",
		Handler: func(s string) []string {
			return []string{
				"\xff\xd8\xff\xe0\x00\x10JFIF" + s + "\xff\xd9",
				"\xff\xd8\xff\xe1\x00\x1cExif" + s + "\xff\xd9",
			}
		},
		Alias:       []string{"jpeg"},
		Description: "生成 jpeg / jpg 文件头",
	})

	const puncSet = `<>?,./:";'{}[]|\_+-=)(*&^%$#@!'"` + "`"
	var puncArr []string
	for _, s := range puncSet {
		puncArr = append(puncArr, string(s))
	}
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "punctuation",
		Handler: func(s string) []string {
			return puncArr
		},
		Alias:       []string{"punc"},
		Description: "生成所有标点符号",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "regen",
		Handler: func(s string) []string {
			return regen.MustGenerate(s)
		},
		Alias:       []string{"re"},
		Description: "使用正则生成所有可能的字符",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "regen:one",
		Handler: func(s string) []string {
			return []string{regen.MustGenerateOne(s)}
		},
		Alias:       []string{"re:one"},
		Description: "使用正则生成所有可能的字符中的随机一个",
		IsDyn:       true,
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "rangechar",
		Handler: func(s string) []string {
			var min byte = 0
			var max byte = 0xff

			ret := utils.PrettifyListFromStringSplited(s, ",")
			switch len(ret) {
			case 2:
				p1, p2 := ret[0], ret[1]
				p1Uint, _ := strconv.ParseUint(p1, 16, 8)
				min = uint8(p1Uint)
				p2Uint, _ := strconv.ParseUint(p2, 16, 8)
				max = uint8(p2Uint)
				if max <= 0 {
					max = 0xff
				}
			case 1:
				p2Uint, _ := strconv.ParseUint(ret[0], 16, 8)
				max = uint8(p2Uint)
				if max <= 0 {
					max = 0xff
				}
			}

			var res []string
			if min > max {
				min = 0
			}

			for i := min; true; i++ {
				res = append(res, string(i))
				if i >= max {
					break
				}
			}
			return res
		},
		Alias:       []string{"range:char", "range"},
		Description: "按顺序生成一个 range 字符集，例如 `{{rangechar(20,7e)}}` 生成 0x20 - 0x7e 的字符集",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "network",
		Handler:     utils.ParseStringToHosts,
		Alias:       []string{"host", "hosts", "cidr", "ip", "net"},
		Description: "生成一个网络地址，例如 `{{network(192.168.1.1/24)}}` 对应 cidr 192.168.1.1/24 所有地址，可以逗号分隔，例如 `{{network(8.8.8.8,192.168.1.1/25,example.com)}}`",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "int",
		Handler: func(s string) []string {
			if s == "" {
				return []string{fmt.Sprint(rand.Intn(10))}
			}

			var enablePadding = false
			var paddingLength = 4
			var paddingRight = false
			var step = 1

			// 使用管道符号分割参数
			parts := strings.Split(s, "|")

			if len(parts) > 1 {
				s = parts[0]
				paddingSuffix := strings.TrimSpace(parts[1])
				enablePadding = true
				paddingRight = strings.HasPrefix(paddingSuffix, "-")
				rawLen := strings.TrimLeft(paddingSuffix, "-")
				paddingLength, _ = strconv.Atoi(rawLen)
			}

			if strings.Contains(s, "-") {
				splited := strings.Split(s, "-")
				left := splited[0]
				if len(left) > 1 && strings.HasPrefix(left, "0") {
					enablePadding = true
					paddingLength = len(left)
				}
			}

			if len(parts) > 2 {
				step, _ = strconv.Atoi(parts[2])
			}

			ints := utils.ParseStringToPorts(s)
			if len(ints) <= 0 {
				return []string{""}
			}

			if step > 1 {
				// 按步长生成结果
				var filteredResults []int
				for i := 0; i < len(ints); i += step {
					filteredResults = append(filteredResults, ints[i])
				}
				ints = filteredResults
			}

			var results []string
			for _, i := range ints {
				r := fmt.Sprint(i)
				if enablePadding && paddingLength > len(r) {
					repeatedPaddingCount := paddingLength - len(r)
					if paddingRight {
						r = r + strings.Repeat("0", repeatedPaddingCount)
					} else {
						r = strings.Repeat("0", repeatedPaddingCount) + r
					}
				}
				results = append(results, r)
			}
			return results
		},
		Alias:       []string{"port", "ports", "integer", "i"},
		Description: "生成一个整数以及范围，例如 {{int(1,2,3,4,5)}} 生成 1,2,3,4,5 中的一个整数，也可以使用 {{int(1-5)}} 生成 1-5 的整数，也可以使用 `{{int(1-5|4)}}` 生成 1-5 的整数，但是每个整数都是 4 位数，例如 0001, 0002, 0003, 0004, 0005，还可以使用 `{{int(1-10|2|3)}}` 来生成带有步长的整数列表。",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "randint",
		IsDynFun: func(name, params string) bool {
			if len(utils.PrettifyListFromStringSplited(params, ",")) == 3 {
				return false
			}
			return true
		},
		Handler: func(s string) []string {
			var (
				min, max, count uint
				err             error

				enablePadding = false
				paddingRight  bool
				paddingLength int
			)

			splitted := strings.SplitN(s, "|", 2)
			if len(splitted) > 1 {
				s = splitted[0]
				paddingSuffix := strings.TrimSpace(splitted[1])
				enablePadding = true
				paddingRight = strings.HasPrefix(paddingSuffix, "-")
				rawLen := strings.TrimLeft(paddingSuffix, "-")
				paddingLength, _ = strconv.Atoi(rawLen)
			}

			count = 1
			fuzztagfallback := []string{fmt.Sprint(rand.Intn(10))}
			raw := utils.PrettifyListFromStringSplited(s, ",")
			switch len(raw) {
			case 3:
				count, err = parseUint(raw[2])
				if err != nil {
					return fuzztagfallback
				}

				if count <= 0 {
					count = 1
				}
				fallthrough
			case 2:
				min, err = parseUint(raw[0])
				if err != nil {
					return fuzztagfallback
				}
				max, err = parseUint(raw[1])
				if err != nil {
					return fuzztagfallback
				}

				min = uint(utils.Min(int(min), int(max)))
				max = uint(utils.Max(int(min), int(max)))
				break
			case 1:
				min = 0
				max, err = parseUint(raw[0])
				if err != nil {
					return fuzztagfallback
				}
				if max <= 0 {
					max = 10
				}
				break
			default:
				return fuzztagfallback
			}

			var results []string
			RepeatFunc(count, func() bool {
				res := int(max - min)
				if res <= 0 {
					res = 10
				}
				i := min + uint(rand.Intn(res))
				c := fmt.Sprint(i)
				if enablePadding && paddingLength > len(c) {
					repeatedPaddingCount := paddingLength - len(c)
					if paddingRight {
						c = c + strings.Repeat("0", repeatedPaddingCount)
					} else {
						c = strings.Repeat("0", repeatedPaddingCount) + c
					}
				}

				results = append(results, fmt.Sprint(c))
				return true
			})
			return results
		},
		Alias:       []string{"ri", "rand:int", "randi"},
		Description: "随机生成整数，定义为 {{randint(10)}} 生成0-10中任意一个随机数，{{randint(1,50)}} 生成 1-50 任意一个随机数，{{randint(1,50,10)}} 生成 1-50 任意一个随机数，重复 10 次",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "randstr",
		IsDynFun: func(name, params string) bool {
			if len(utils.PrettifyListFromStringSplited(params, ",")) == 3 {
				return false
			}
			return true
		},
		ErrorInfoHandler: func(s string) ([]string, error) {
			var (
				min, max, count uint
				err             error
			)
			fuzztagfallback := []string{utils.RandStringBytes(8)}
			count = 1
			min = 1
			raw := utils.PrettifyListFromStringSplited(s, ",")
			switch len(raw) {
			case 3:
				count, err = parseUint(raw[2])
				if err != nil {
					return fuzztagfallback, err
				}

				if count <= 0 {
					count = 1
				}
				fallthrough
			case 2:
				min, err = parseUint(raw[0])
				if err != nil {
					return fuzztagfallback, err
				}
				max, err = parseUint(raw[1])
				if err != nil {
					return fuzztagfallback, err
				}
				min = uint(utils.Min(int(min), int(max)))
				max = uint(utils.Max(int(min), int(max)))
				if max >= 1e8 {
					max = 1e8
					err = fmt.Errorf("max length is 100000000")
				}
				break
			case 1:
				max, err = parseUint(raw[0])
				if err != nil {
					return fuzztagfallback, err
				}
				min = max
				if max <= 0 {
					max = 8
				}
				break
			default:
				return fuzztagfallback, err
			}

			var r []string
			RepeatFunc(count, func() bool {
				result := int(max - min)
				if result < 0 {
					result = 8
				}

				var offset uint = 0
				if result > 0 {
					offset = uint(rand.Intn(result))
				}
				c := min + offset
				r = append(r, utils.RandStringBytes(int(c)))
				return true
			})
			return r, err
		},
		Alias:       []string{"rand:str", "rs", "rands"},
		Description: "随机生成个字符串，定义为 {{randstr(10)}} 生成长度为 10 的随机字符串，{{randstr(1,30)}} 生成长度为 1-30 为随机字符串，{{randstr(1,30,10)}} 生成 10 个随机字符串，长度为 1-30",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "codec",
		Handler: func(s string) []string {
			s = strings.Trim(s, " ()")

			if codecCaller == nil {
				return []string{s}
			}

			lastDividerIndex := strings.LastIndexByte(s, '|')
			if lastDividerIndex < 0 {
				script, err := codecCaller(s, "")
				if err != nil {
					log.Errorf("codec caller error: %s", err)
					return []string{s}
				}
				//log.Errorf("fuzz.codec no plugin / param specific")
				return []string{script}
			}
			name, params := s[:lastDividerIndex], s[lastDividerIndex+1:]
			script, err := codecCaller(name, params)
			if err != nil {
				log.Errorf("codec caller error: %s", err)
				return []string{s}
			}
			return []string{script}
		},
		Description: "调用 Yakit Codec 插件",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "codec:line",
		Handler: func(s string) []string {
			if codecCaller == nil {
				return fuzztagfallback
			}

			s = strings.Trim(s, " ()")
			lastDividerIndex := strings.LastIndexByte(s, '|')
			if lastDividerIndex < 0 {
				log.Errorf("fuzz.codec no plugin / param specific")
				return fuzztagfallback
			}
			name, params := s[:lastDividerIndex], s[lastDividerIndex+1:]
			script, err := codecCaller(name, params)
			if err != nil {
				log.Errorf("codec caller error: %s", err)
				return fuzztagfallback
			}
			var results []string
			for line := range utils.ParseLines(script) {
				results = append(results, line)
			}
			return results
		},
		Description: "调用 Yakit Codec 插件，把结果解析成行",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "unquote",
		Handler: func(s string) []string {
			raw, err := strconv.Unquote(s)
			if err != nil {
				raw, err := strconv.Unquote(`"` + s + `"`)
				if err != nil {
					log.Errorf("unquoted failed: %s", err)
					return []string{s}
				}
				return []string{raw}
			}

			return []string{
				raw,
			}
		},
		Description: "把内容进行 strconv.Unquote 转化",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "quote",
		Handler: func(s string) []string {
			return []string{
				strconv.Quote(s),
			}
		},
		Description: "strconv.Quote 转化",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "lower",
		Handler: func(s string) []string {
			return []string{
				strings.ToLower(s),
			}
		},
		Description: "把传入的内容都设置成小写 {{lower(Abc)}} => abc",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "upper",
		Handler: func(s string) []string {
			return []string{
				strings.ToUpper(s),
			}
		},
		Description: "把传入的内容变成大写 {{upper(abc)}} => ABC",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "base64enc",
		Handler: func(s string) []string {
			return []string{
				base64.StdEncoding.EncodeToString([]byte(s)),
			}
		},
		Alias:       []string{"base64encode", "base64e", "base64", "b64"},
		Description: "进行 base64 编码，{{base64enc(abc)}} => YWJj",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "base64dec",
		Handler: func(s string) []string {
			r, err := codec.DecodeBase64(s)
			if err != nil {
				return []string{s}
			}
			return []string{string(r)}
		},
		Alias:       []string{"base64decode", "base64d", "b64d"},
		Description: "进行 base64 解码，{{base64dec(YWJj)}} => abc",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "md5",
		Handler: func(s string) []string {
			return []string{codec.Md5(s)}
		},
		Description: "进行 md5 编码，{{md5(abc)}} => 900150983cd24fb0d6963f7d28e17f72",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "hexenc",
		Handler: func(s string) []string {
			return []string{codec.EncodeToHex(s)}
		},
		Alias:       []string{"hex", "hexencode"},
		Description: "HEX 编码，{{hexenc(abc)}} => 616263",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "hexdec",
		Handler: func(s string) []string {
			raw, err := codec.DecodeHex(s)
			if err != nil {
				return []string{s}
			}
			return []string{string(raw)}
		},
		Alias:       []string{"hexd", "hexdec", "hexdecode"},
		Description: "HEX 解码，{{hexdec(616263)}} => abc",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "sha1",
		Handler:     func(s string) []string { return []string{codec.Sha1(s)} },
		Description: "进行 sha1 编码，{{sha1(abc)}} => a9993e364706816aba3e25717850c26c9cd0d89d",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "sha256",
		Handler:     func(s string) []string { return []string{codec.Sha256(s)} },
		Description: "进行 sha256 编码，{{sha256(abc)}} => ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "sha224",
		Handler:     func(s string) []string { return []string{codec.Sha224(s)} },
		Description: "",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "sha512",
		Handler:     func(s string) []string { return []string{codec.Sha512(s)} },
		Description: "进行 sha512 编码，{{sha512(abc)}} => ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "sha384",
		Handler: func(s string) []string { return []string{codec.Sha384(s)} },
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "sm3",
		Handler:     func(s string) []string { return []string{codec.EncodeToHex(codec.SM3(s))} },
		Description: "计算 sm3 哈希值，{{sm3(abc)}} => 66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a3f0b8ddb27d8a7eb3",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "hextobase64",
		Handler: func(s string) []string {
			raw, err := codec.DecodeHex(s)
			if err != nil {
				return []string{s}
			}
			return []string{base64.StdEncoding.EncodeToString(raw)}
		},
		Alias:       []string{"h2b64", "hex2base64"},
		Description: "把 HEX 字符串转换为 base64 编码，{{hextobase64(616263)}} => YWJj",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "base64tohex",
		Handler: func(s string) []string {
			raw, err := codec.DecodeBase64(s)
			if err != nil {
				return []string{s}
			}
			return []string{codec.EncodeToHex(string(raw))}
		},
		Alias:       []string{"b642h", "base642hex"},
		Description: "把 Base64 字符串转换为 HEX 编码，{{base64tohex(YWJj)}} => 616263",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "urlescape",
		Handler: func(s string) []string {
			return []string{codec.QueryEscape(s)}
		},
		Alias:       []string{"urlesc"},
		Description: "url 编码(只编码特殊字符)，{{urlescape(abc=)}} => abc%3d",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "urlenc",
		Handler: func(s string) []string {
			return []string{codec.EncodeUrlCode(s)}
		},
		Alias:       []string{"urlencode", "url"},
		Description: "URL 强制编码，{{urlenc(abc)}} => %61%62%63",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "urldec",
		Handler: func(s string) []string {
			r, err := codec.QueryUnescape(s)
			if err != nil {
				return []string{s}
			}
			return []string{string(r)}
		},
		Alias:       []string{"urldecode", "urld"},
		Description: "URL 强制解码，{{urldec(%61%62%63)}} => abc",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "doubleurlenc",
		Handler: func(s string) []string {
			return []string{codec.DoubleEncodeUrl(s)}
		},
		Alias:       []string{"doubleurlencode", "durlenc", "durl"},
		Description: "双重URL编码，{{doubleurlenc(abc)}} => %2561%2562%2563",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "doubleurldec",
		Handler: func(s string) []string {
			r, err := codec.DoubleDecodeUrl(s)
			if err != nil {
				return []string{s}
			}
			return []string{r}
		},
		Alias:       []string{"doubleurldecode", "durldec", "durldecode"},
		Description: "双重URL解码，{{doubleurldec(%2561%2562%2563)}} => abc",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "htmlenc",
		Handler: func(s string) []string {
			return []string{codec.EncodeHtmlEntity(s)}
		},
		Alias:       []string{"htmlencode", "html", "htmle", "htmlescape"},
		Description: "HTML 实体编码，{{htmlenc(abc)}} => &#97;&#98;&#99;",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "htmlhexenc",
		Handler: func(s string) []string {
			return []string{codec.EncodeHtmlEntityHex(s)}
		},
		Alias:       []string{"htmlhex", "htmlhexencode", "htmlhexescape"},
		Description: "HTML 十六进制实体编码，{{htmlhexenc(abc)}} => &#x61;&#x62;&#x63;",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "htmldec",
		Handler: func(s string) []string {
			return []string{codec.UnescapeHtmlString(s)}
		},
		Alias:       []string{"htmldecode", "htmlunescape"},
		Description: "HTML 解码，{{htmldec(&#97;&#98;&#99;)}} => abc",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "repeatstr",
		Handler: func(s string) []string {
			if !strings.Contains(s, "|") {
				return []string{s + s}
			}
			index := strings.LastIndex(s, "|")
			if index <= 0 {
				return []string{s}
			}
			n, _ := strconv.Atoi(s[index+1:])
			s = s[:index]
			return []string{strings.Repeat(s, n)}
		},
		Alias:       []string{"repeat:str"},
		Description: "重复字符串，`{{repeatstr(abc|3)}}` => abcabcabc",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "randomupper",
		Handler: func(s string) []string {
			return []string{codec.RandomUpperAndLower(s)}
		},
		Alias:       []string{"random:upper", "random:lower"},
		Description: "随机大小写，{{randomupper(abc)}} => aBc",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "yso:exec",
		HandlerEx: func(s string) []*fuzztag.FuzzExecResult {
			var result []*fuzztag.FuzzExecResult
			pushNewResult := func(d []byte, verbose []string) {
				result = append(result, fuzztag.NewFuzzExecResult(d, verbose))
			}
			for _, gadget := range yso.GetAllRuntimeExecGadget() {
				javaObj, err := gadget(s)
				if javaObj == nil || err != nil {
					continue
				}
				objBytes, err := yso.ToBytes(javaObj)
				if err != nil {
					continue
				}
				pushNewResult(objBytes, []string{javaObj.Verbose().GetNameVerbose(), "runtime exec evil class", s})
			}
			for _, gadget := range yso.GetAllTemplatesGadget() {
				javaObj, err := gadget(yso.SetProcessImplExecEvilClass(s))
				if javaObj == nil || err != nil {
					continue
				}
				objBytes, err := yso.ToBytes(javaObj)
				if err != nil {
					continue
				}
				pushNewResult(objBytes, []string{javaObj.Verbose().GetNameVerbose(), "processImpl exec evil class", s})
			}
			if len(result) > 0 {
				return result
			}

			return []*fuzztag.FuzzExecResult{fuzztag.NewFuzzExecResult([]byte(s), []string{s})}
		},
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "yso:dnslog",
		HandlerEx: func(s string) []*fuzztag.FuzzExecResult {
			var getDomain func() string
			sSplit := strings.Split(s, "|")
			if len(sSplit) == 2 {
				n := 0
				getDomain = func() string {
					n++
					return fmt.Sprintf("%s%d.%s", sSplit[1], n, sSplit[0])
				}
			} else {
				getDomain = func() string {
					return s
				}
			}
			var result []*fuzztag.FuzzExecResult
			pushNewResult := func(d []byte, verbose []string) {
				result = append(result, fuzztag.NewFuzzExecResult(d, verbose))
			}
			for _, gadget := range yso.GetAllTemplatesGadget() {
				domain := getDomain()
				javaObj, err := gadget(yso.SetDnslogEvilClass(domain))
				if javaObj == nil || err != nil {
					continue
				}
				objBytes, err := yso.ToBytes(javaObj)
				if err != nil {
					continue
				}
				pushNewResult(objBytes, []string{javaObj.Verbose().GetNameVerbose(), "dnslog evil class", domain})
			}
			if len(result) > 0 {
				return result
			}

			return []*fuzztag.FuzzExecResult{fuzztag.NewFuzzExecResult([]byte(s), []string{s})}
		},
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "yso:urldns",
		HandlerEx: func(s string) []*fuzztag.FuzzExecResult {
			var result []*fuzztag.FuzzExecResult
			javaObj, err := yso.GetURLDNSJavaObject(s)
			if err == nil {
				objBytes, err := yso.ToBytes(javaObj)
				if err == nil {
					return append(result, fuzztag.NewFuzzExecResult(objBytes, []string{javaObj.Verbose().GetNameVerbose()}))
				}
			}
			return []*fuzztag.FuzzExecResult{fuzztag.NewFuzzExecResult([]byte(s), []string{s})}
		},
	})
	// 标签太长了，前端加了个简单的 fuzztag 提示
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "yso:find_gadget_by_dns",
		HandlerEx: func(s string) []*fuzztag.FuzzExecResult {
			var result []*fuzztag.FuzzExecResult
			javaObj, err := yso.GetFindGadgetByDNSJavaObject(s)
			if err == nil {
				objBytes, err := yso.ToBytes(javaObj)
				if err == nil {
					return append(result, fuzztag.NewFuzzExecResult(objBytes, []string{javaObj.Verbose().GetNameVerbose()}))
				}
			}
			return []*fuzztag.FuzzExecResult{fuzztag.NewFuzzExecResult([]byte(s), []string{s})}
		},
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "yso:find_gadget_by_bomb",
		HandlerEx: func(s string) []*fuzztag.FuzzExecResult {
			var result []*fuzztag.FuzzExecResult
			pushNewResult := func(d []byte, verbose []string) {
				result = append(result, fuzztag.NewFuzzExecResult(d, verbose))
			}

			if s == "all" {
				for gadget, className := range yso.GetGadgetChecklist() {
					javaObj, err := yso.GetFindClassByBombJavaObject(className)
					if javaObj == nil || err != nil {
						continue
					}
					objBytes, err := yso.ToBytes(javaObj)
					if err != nil {
						continue
					}
					pushNewResult(objBytes, []string{javaObj.Verbose().GetNameVerbose(), "Gadget", gadget})
				}
			} else {
				javaObj, err := yso.GetFindClassByBombJavaObject(s)
				if javaObj == nil || err != nil {
					return result
				}
				objBytes, err := yso.ToBytes(javaObj)
				if err != nil {
					return result
				}
				pushNewResult(objBytes, []string{javaObj.Verbose().GetNameVerbose(), "类名", s})
			}

			if len(result) > 0 {
				return result
			}

			return []*fuzztag.FuzzExecResult{fuzztag.NewFuzzExecResult([]byte(s), []string{s})}
		},
	})
	//这几个标签不稳定
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "yso:headerecho",
		Description: "尽力使用 header echo 生成多个链",
		HandlerEx: func(s string) []*fuzztag.FuzzExecResult {
			headers := strings.Split(s, "|")
			if len(headers) != 2 {
				return []*fuzztag.FuzzExecResult{fuzztag.NewFuzzExecResult([]byte(s), []string{s})}
			}
			var result []*fuzztag.FuzzExecResult
			pushNewResult := func(d []byte, verbose []string) {
				result = append(result, fuzztag.NewFuzzExecResult(d, verbose))
			}
			for _, gadget := range yso.GetAllTemplatesGadget() {
				javaObj, err := gadget(yso.SetMultiEchoEvilClass(), yso.SetHeader(headers[0], headers[1]))
				if javaObj == nil || err != nil {
					continue
				}
				objBytes, err := yso.ToBytes(javaObj)
				if err != nil {
					continue
				}
				pushNewResult(objBytes, []string{javaObj.Verbose().GetNameVerbose(), "tomcat header echo evil class", s})
			}
			if len(result) > 0 {
				return result
			}
			return []*fuzztag.FuzzExecResult{fuzztag.NewFuzzExecResult([]byte(s), []string{s})}
		},
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "yso:bodyexec",
		Description: "尽力使用 class body exec 的方式生成多个链",
		HandlerEx: func(s string) []*fuzztag.FuzzExecResult {
			var result []*fuzztag.FuzzExecResult
			pushNewResult := func(d []byte, verbose []string) {
				result = append(result, fuzztag.NewFuzzExecResult(d, verbose))
			}
			for _, gadget := range yso.GetAllTemplatesGadget() {
				javaObj, err := gadget(yso.SetMultiEchoEvilClass(), yso.SetExecAction(), yso.SetParam(s), yso.SetEchoBody())
				if javaObj == nil || err != nil {
					continue
				}
				objBytes, err := yso.ToBytes(javaObj)
				if err != nil {
					continue
				}
				pushNewResult(objBytes, []string{javaObj.Verbose().GetNameVerbose(), "tomcat body exec echo evil class", s})
			}
			if len(result) > 0 {
				return result
			}

			return []*fuzztag.FuzzExecResult{fuzztag.NewFuzzExecResult([]byte(s), []string{s})}
		},
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "headerauth",
		Handler: func(s string) []string {
			return []string{"Accept-Language: zh-CN,zh;q=1.9"}
		},
	})
}

func FuzzFileOptions() []FuzzConfigOpt {
	var opt []FuzzConfigOpt
	for _, t := range Filetag() {
		opt = append(opt, Fuzz_WithExtraFuzzTagHandler(t.TagName, t.Handler))
		for _, a := range t.Alias {
			opt = append(opt, Fuzz_WithExtraFuzzTagHandler(a, t.Handler))
		}
	}
	return opt
}

func Fuzz_WithEnableFiletag() FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		for _, opt := range FuzzFileOptions() {
			opt(config)
		}
	}
}

func Filetag() []*FuzzTagDescription {
	return []*FuzzTagDescription{
		{
			TagName: "file:line",
			Handler: func(s string) []string {
				s = strings.Trim(s, " ()")
				var result []string
				for _, lineFile := range utils.PrettifyListFromStringSplited(s, "|") {
					lineChan, err := utils.FileLineReader(lineFile)
					if err != nil {
						log.Errorf("fuzztag read file failed: %s", err)
						continue
					}
					for line := range lineChan {
						result = append(result, string(line))
					}
				}
				if len(result) <= 0 {
					return fuzztagfallback
				}
				return result
			},
			Alias:       []string{"fileline", "file:lines"},
			Description: "解析文件名（可以用 `|` 分割），把文件中的内容按行反回成数组，定义为 `{{file:line(/tmp/test.txt)}}` 或 `{{file:line(/tmp/test.txt|/tmp/1.txt)}}`",
		},
		{
			TagName: "file:dir",
			Handler: func(s string) []string {
				s = strings.Trim(s, " ()")
				var result []string
				for _, lineFile := range utils.PrettifyListFromStringSplited(s, "|") {
					fileRaw, err := ioutil.ReadDir(lineFile)
					if err != nil {
						log.Errorf("fuzz.filedir read dir failed: %s", err)
						continue
					}
					for _, info := range fileRaw {
						if info.IsDir() {
							continue
						}
						fileContent, err := ioutil.ReadFile(info.Name())
						if err != nil {
							continue
						}
						result = append(result, string(fileContent))
					}
				}
				if len(result) <= 0 {
					return fuzztagfallback
				}
				return result
			},
			Alias:       []string{"filedir"},
			Description: "解析文件夹，把文件夹中文件的内容读取出来，读取成数组返回，定义为 `{{file:dir(/tmp/test)}}` 或 `{{file:dir(/tmp/test|/tmp/1)}}`",
		},
		{
			TagName: "file",
			Handler: func(s string) []string {
				s = strings.Trim(s, " ()")
				var result []string
				for _, lineFile := range utils.PrettifyListFromStringSplited(s, "|") {
					fileRaw, err := ioutil.ReadFile(lineFile)
					if err != nil {
						log.Errorf("fuzz.files read file failed: %s", err)
						continue
					}
					result = append(result, string(fileRaw))
				}
				if len(result) <= 0 {
					return fuzztagfallback
				}
				return result
			},
			Description: "读取文件内容，可以支持多个文件，用竖线分割，`{{file(/tmp/1.txt)}}` 或 `{{file(/tmp/1.txt|/tmp/test.txt)}}`",
		},
	}
}
