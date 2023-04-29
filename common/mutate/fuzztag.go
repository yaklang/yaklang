package mutate

import (
	"encoding/base64"
	"fmt"
	"github.com/google/uuid"
	"io/ioutil"
	"math/rand"
	"yaklang/common/consts"
	"yaklang/common/fuzztag"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/bizhelper"
	"yaklang/common/utils/regen"
	"yaklang/common/yak/yaklib/codec"
	"yaklang/common/yso"
	"strconv"
	"strings"
	"time"
)

var fuzztagfallback = []string{""}

type FuzzTagDescription struct {
	TagName     string
	Handler     func(string) []string
	HandlerEx   func(string) []*fuzztag.FuzzExecResult
	Alias       []string
	Description string
}

var existedFuzztag []*FuzzTagDescription

func AddFuzzTagToGlobal(f *FuzzTagDescription) {
	if f == nil {
		return
	}
	existedFuzztag = append(existedFuzztag, f)
	if f.Handler != nil {
		defaultFuzzTag[f.TagName] = f.Handler
	}
	if f.HandlerEx != nil {
		defaultFuzzTagEx[f.TagName] = f.HandlerEx
	}
	for _, i := range f.Alias {
		fuzztag.SetMethodAlias(f.TagName, i)
	}
}

func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

// 读取一个分隔符最后出现位置的部分
func sepToEnd(s string, sep string) (string, string) {
	if strings.LastIndex(s, sep) < 0 {
		return s, ""
	}
	return s[:strings.LastIndex(s, sep)], s[strings.LastIndex(s, sep)+1:]
}

var (
	passSuffix1 = []string{
		"", "!", "!@", "!@#", "!@$", "@", "#",
		"_", "$", ".",
		"*",
	}
	passSuffix2 = []string{
		"1", "123", "12345", "123456", "qwerty",
		"qwe", "q1w2e3", "666", "888",
		"666666", "88888888", "111", "111111",
	}
	passPrefix = []string{
		"", "web", "@", "$", "*",
	}
)

func fuzzLowerNUpper(i string) []string {
	if len(i) > 18 {
		return []string{i}
	}

	var res []string
	var bytes = []byte(strings.ToLower(i))
	res = append(res, strings.ToLower(i))
	res = append(res, strings.ToUpper(i))
	// one upper
	for index := 0; index < len(i); index++ {
		copiedBytes := make([]byte, len(bytes))
		copy(copiedBytes, bytes)
		copiedBytes[index] = strings.ToUpper(string([]byte{copiedBytes[index]}))[0]
		res = append(res, string(copiedBytes))
	}

	// two
	for firstIndex := 0; firstIndex < len(i); firstIndex++ {
		for secondIndex := firstIndex + 2; secondIndex < len(i); secondIndex++ {
			if firstIndex == secondIndex {
				continue
			}
			copiedBytes := make([]byte, len(bytes))
			copy(copiedBytes, bytes)
			copiedBytes[firstIndex] = strings.ToUpper(string([]byte{copiedBytes[firstIndex]}))[0]
			copiedBytes[secondIndex] = strings.ToUpper(string([]byte{copiedBytes[secondIndex]}))[0]
			res = append(res, string([]byte{copiedBytes[firstIndex], copiedBytes[secondIndex]}))
			res = append(res, string(copiedBytes))
		}
	}

	// three
	for firstIndex := 0; firstIndex < len(i); firstIndex++ {
		for secondIndex := firstIndex + 2; secondIndex < len(i); secondIndex++ {
			for thirdIndex := secondIndex + 2; thirdIndex < len(i); thirdIndex++ {
				if firstIndex == secondIndex || firstIndex == thirdIndex || secondIndex == thirdIndex {
					continue
				}
				copiedBytes := make([]byte, len(bytes))
				copy(copiedBytes, bytes)
				copiedBytes[firstIndex] = strings.ToUpper(string([]byte{copiedBytes[firstIndex]}))[0]
				copiedBytes[secondIndex] = strings.ToUpper(string([]byte{copiedBytes[secondIndex]}))[0]
				copiedBytes[thirdIndex] = strings.ToUpper(string([]byte{copiedBytes[thirdIndex]}))[0]

				res = append(res, string([]byte{copiedBytes[firstIndex], copiedBytes[secondIndex], copiedBytes[thirdIndex]}))
				res = append(res, string(copiedBytes))
			}
		}
	}
	return res
}

func fuzzuser(i string, level int) []string {
	if i == "" {
		i = "admin,root"
	}

	var res []string
	splited := utils.PrettifyListFromStringSplitEx(i, ",", "|")
	if len(splited) <= 2 {
		res = append(res, i)
	}
	res = append(res, splited...)
	passSuffix2 := passSuffix2
	switch level {
	case 3:
		for i := 1970; i <= time.Now().Year(); i++ {
			passSuffix2 = append(passSuffix2, fmt.Sprint(i))
		}
	case 2:
		for i := 1990; i <= time.Now().Year(); i++ {
			passSuffix2 = append(passSuffix2, fmt.Sprint(i))
		}
	default:
		for i := 2000; i <= time.Now().Year(); i++ {
			passSuffix2 = append(passSuffix2, fmt.Sprint(i))
		}
	}

	handleItem := func(item string) {
		for _, prefix := range passPrefix {
			item2 := prefix + item
			for _, suffix2 := range passSuffix2 {
				res = append(res, item2+suffix2)
			}
		}
	}

	for _, r := range res {
		for _, item := range fuzzLowerNUpper(r) {
			handleItem(item)
		}
	}
	return res
}

func fuzzpass(i string, level int) []string {
	if i == "" {
		i = "admin,root"
	}

	var res []string
	splited := utils.PrettifyListFromStringSplitEx(i, ",", "|")
	if len(splited) <= 2 {
		res = append(res, i)
	}
	res = append(res, splited...)

	passSuffix2 := passSuffix2
	switch level {
	case 3:
		for i := 1970; i <= time.Now().Year(); i++ {
			passSuffix2 = append(passSuffix2, fmt.Sprint(i))
		}
	case 2:
		for i := 1990; i <= time.Now().Year(); i++ {
			passSuffix2 = append(passSuffix2, fmt.Sprint(i))
		}
	default:
		for i := 2000; i <= time.Now().Year(); i++ {
			passSuffix2 = append(passSuffix2, fmt.Sprint(i))
		}
	}

	handleItem := func(item string) {
		for _, prefix := range passPrefix {
			item2 := prefix + item
			for _, suffix := range passSuffix1 {
				res = append(res, item2+suffix)
			}
			for _, suffix2 := range passSuffix2 {
				res = append(res, item2+suffix2)
			}
			for _, suffix1 := range passSuffix1 {
				for _, suffix2 := range passSuffix2 {
					res = append(res, item2+suffix1+suffix2)
				}
			}
			for _, suffix2 := range passSuffix2 {
				for _, suffix1 := range passSuffix1 {
					res = append(res, item2+suffix1+suffix2)
				}
			}
		}
	}

	for _, r := range res {
		for _, item := range fuzzLowerNUpper(r) {
			handleItem(item)
		}
	}
	return res
}

func init() {
	fuzztag.SetMethodAlias("params", "param", "p")

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
			i, _ := strconv.Atoi(s)
			if i > 0 {
				return make([]string, i)
			}
			return []string{""}
		},
		Description: "重复一个字符串，例如：`{{repeat(abc|3)}}`，结果为：abcabcabc",
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

			splitted := strings.SplitN(s, "|", 2)
			if len(splitted) > 1 {
				s = splitted[0]
				paddingSuffix := strings.TrimSpace(splitted[1])
				enablePadding = true
				paddingRight = strings.HasPrefix(paddingSuffix, "-")
				rawLen := strings.TrimLeft(paddingSuffix, "-")
				paddingLength, _ = strconv.Atoi(rawLen)
			}

			ints := utils.ParseStringToPorts(s)
			if len(ints) <= 0 {
				return []string{""}
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
		Alias:       []string{"port", "ports", "integer", "i", "p"},
		Description: "生成一个整数以及范围，例如 {{int(1,2,3,4,5)}} 生成 1,2,3,4,5 中的一个整数，也可以使用 {{int(1-5)}} 生成 1-5 的整数，也可以使用 `{{int(1-5|4)}}` 生成 1-5 的整数，但是每个整数都是 4 位数，例如 0001, 0002, 0003, 0004, 0005",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "randint",
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
		Handler: func(s string) []string {
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
				max, err = parseUint(raw[0])
				if err != nil {
					return fuzztagfallback
				}
				min = max
				if max <= 0 {
					max = 8
				}
				break
			default:
				return fuzztagfallback
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
			return r
		},
		Alias:       []string{"rand:str", "rs", "rands"},
		Description: "随机生成个字符串，定义为 {{randstr(10)}} 生成长度为 10 的随机字符串，{{randstr(1,30)}} 生成长度为 1-30 为随机字符串，{{randstr(1,30,10)}} 生成 10 个随机字符串，长度为 1-30",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
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
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
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
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
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

var defaultFuzzTag = map[string]func(string) []string{}
var defaultFuzzTagEx = map[string]func(string) []*fuzztag.FuzzExecResult{}

func MutateQuick(i interface{}) (finalResult []string) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("fuzztag execute failed: %s", err)
			finalResult = []string{utils.InterfaceToString(i)}
		}
	}()
	results, err := FuzzTagExec(i)
	if err != nil {
		return []string{utils.InterfaceToString(i)}
	}
	return results
}
