package yakvm

import (
	"fmt"
	"math/rand"
	"strings"
	"unicode"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
)

func NewStringMethodFactory(f func(string) interface{}) MethodFactory {
	return func(vm *Frame, i interface{}) interface{} {
		raw, ok := i.(string)
		if !ok {
			raw = fmt.Sprint(i)
		}
		return f(raw)
	}
}

var stringBuildinMethod = map[string]*buildinMethod{
	"First": {
		Name:       "First",
		ParamTable: nil,
		HandlerFactory: NewStringMethodFactory(func(c string) interface{} {
			return func() rune {
				return rune(c[0])
			}
		}),
		Description: "获取字符串第一个字符",
	},
	"Reverse": {
		Name:       "Reverse",
		ParamTable: nil,
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func() string {
				// runes 是为了处理中文字符问题，这个是合理的
				runes := []rune(s)
				for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
					runes[i], runes[j] = runes[j], runes[i]
				}
				return string(runes)
			}
		}),
		Description: "倒序字符串",
	},
	"Shuffle": {
		Name:       "Shuffle",
		ParamTable: nil,
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func() string {
				var raw = []rune(s)
				rand.Shuffle(len(raw), func(i, j int) {
					raw[i] = raw[j]
				})
				return string(raw)
			}
		}),
		Description: "随机打乱字符串",
	},
	"Fuzz": {
		Name:       "Fuzz",
		ParamTable: nil,
		Snippet:    `Fuzz(${1:{"params": "value"\}})$0`,
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(i ...interface{}) []string {
				var opts []mutate.FuzzConfigOpt
				if len(i) > 0 {
					opts = append(opts, mutate.Fuzz_WithParams(i[0]))
				}

				if len(i) > 1 {
					log.Warn("string.Fuzz only need one param as {{params(...)}} source")
				}

				res, err := mutate.FuzzTagExec(s, opts...)
				if err != nil {
					log.Errorf("fuzz tag error: %s", err)
					return nil
				}
				return res
			}
		}),
	},
	"Contains": {
		Name:       "Contains",
		ParamTable: []string{"substr"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(substr string) bool {
				if len(substr) == 0 {
					return true
				}
				return strings.Contains(s, substr)
			}
		}),
		Description: "判断字符串是否包含子串",
	},
	"IContains": {
		Name:       "IContains",
		ParamTable: []string{"substr"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(substr string) bool {
				if len(substr) == 0 {
					return true
				}
				return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
			}
		}),
		Description: "判断字符串是否包含子串",
	},
	"ReplaceN": {
		Name:       "ReplaceN",
		ParamTable: []string{"old", "new", "n"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(old, new string, n int) string {
				return strings.Replace(s, old, new, n)
			}
		},
		),
		Description: "替换字符串中的子串",
	},
	"ReplaceAll": {
		Name:       "ReplaceAll",
		ParamTable: []string{"old", "new"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(old, new string) string {
				return strings.ReplaceAll(s, old, new)
			}
		},
		),
		Description: "替换字符串中所有的子串",
	},
	"Split": {
		Name:       "Split",
		ParamTable: []string{"separator"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(sep string) []string {
				return strings.Split(s, sep)
			}
		},
		),
		Description: "分割字符串",
	},
	"SplitN": {
		Name:       "SplitN",
		ParamTable: []string{"separator", "n"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(sep string, n int) []string {
				return strings.SplitN(s, sep, n)
			}
		},
		),
		Description: "分割字符串，最多分割为N份",
	},
	"Join": {
		Name:       "Join",
		ParamTable: []string{"slice"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(i interface{}) string {
				return strings.Join(utils.InterfaceToStringSlice(i), s)
			}
		},
		),
		Description: "连接字符串",
	},
	"Trim": {
		Name:            "Trim",
		ParamTable:      []string{"cutstr"},
		IsVariadicParam: true,
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(cutset ...string) string {
				if cutset != nil {
					return strings.Trim(s, strings.Join(cutset, ""))
				}

				return strings.TrimSpace(s)
			}
		},
		),
		Description: "去除字符串两端的cutset",
	},
	"TrimLeft": {
		Name:            "TrimLeft",
		ParamTable:      []string{"cutstr"},
		IsVariadicParam: true,
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(cutset ...string) string {
				if cutset != nil {
					return strings.TrimLeft(s, strings.Join(cutset, ""))
				}

				return strings.TrimLeftFunc(s, unicode.IsSpace)
			}
		}),
		Description: "去除字符串左端的cutset",
	},
	"TrimRight": {
		Name:       "TrimRight",
		ParamTable: []string{"cutstr"}, IsVariadicParam: true,
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(cutset ...string) string {
				if cutset != nil {
					return strings.TrimRight(s, strings.Join(cutset, ""))
				}

				return strings.TrimRightFunc(s, unicode.IsSpace)
			}
		}),
		Description: "去除字符串右端的cutset",
	},
	"HasPrefix": {
		Name:       "HasPrefix",
		ParamTable: []string{"prefix"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(prefix string) bool {
				return strings.HasPrefix(s, prefix)
			}
		},
		),
		Description: "判断字符串是否以prefix开头",
	},
	"RemovePrefix": {
		Name:       "RemovePrefix",
		ParamTable: []string{"prefix"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(prefix string) string {
				if strings.HasPrefix(s, prefix) {
					return s[len(prefix):]
				}
				return s
			}
		},
		),
		Description: "移除前缀",
	},
	"HasSuffix": {
		Name:       "HasSuffix",
		ParamTable: []string{"suffix"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(suffix string) bool {
				return strings.HasSuffix(s, suffix)
			}
		},
		),
		Description: "判断字符串是否以suffix结尾",
	},
	"RemoveSuffix": {
		Name:       "RemoveSuffix",
		ParamTable: []string{"suffix"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(suffix string) string {
				if strings.HasSuffix(s, suffix) {
					return s[:len(suffix)]
				}
				return s
			}
		},
		),
		Description: "移除后缀",
	},
	"Zfill": {
		Name:       "Zfill",
		ParamTable: []string{"width"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(width int) string {
				lenOfS := len(s)
				if width <= lenOfS {
					return s
				} else {
					return strings.Repeat("0", width-lenOfS) + s
				}
			}
		},
		),
		Description: "字符串左侧填充0",
	},
	"Rzfill": {
		Name:       "Rzfill",
		ParamTable: []string{"width"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(width int) string {
				lenOfS := len(s)
				if width <= lenOfS {
					return s
				} else {

					return s + strings.Repeat("0", width-lenOfS)
				}
			}
		},
		),
		Description: "字符串右侧填充0",
	},
	"Ljust": {
		Name:       "Ljust",
		ParamTable: []string{"width"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(width int, fill ...string) string {
				lenOfS := len(s)
				if width <= lenOfS {
					return s
				} else {
					fillStr := " "
					if len(fill) > 0 {
						fillStr = fill[0]
					}
					return s + strings.Repeat(fillStr, width-lenOfS)
				}
			}
		},
		),
		Description: "字符串左侧填充空格",
	},
	"Rjust": {
		Name:       "Rjust",
		ParamTable: []string{"width"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(width int, fill ...string) string {
				lenOfS := len(s)
				if width <= lenOfS {
					return s
				} else {
					fillStr := " "
					if len(fill) > 0 {
						fillStr = fill[0]
					}
					return strings.Repeat(fillStr, width-lenOfS) + s
				}
			}
		},
		),
		Description: "字符串右侧填充空格",
	},
	"Count": {
		Name:       "Count",
		ParamTable: []string{"substr"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(substr string) int {
				return strings.Count(s, substr)
			}
		},
		),
		Description: "统计字符串中substr出现的次数",
	},
	"Find": {
		Name:       "Find",
		ParamTable: []string{"substr"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(substr string) int {
				return strings.Index(s, substr)
			}
		},
		),
		Description: "查找字符串中substr第一次出现的位置, 如果没找到则返回-1",
	},
	"Rfind": {
		Name:       "Rfind",
		ParamTable: []string{"substr"},
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func(substr string) int {
				return strings.LastIndex(s, substr)
			}
		},
		),
		Description: "查找字符串中substr最后一次出现的位置, 如果没找到则返回-1",
	},
	"Lower": {
		Name: "Lower",
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func() string {
				return strings.ToLower(s)
			}
		},
		),
		Description: "将字符串转换为小写",
	},
	"Upper": {
		Name: "Upper",
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func() string {
				return strings.ToUpper(s)
			}
		},
		),
		Description: "将字符串转换为大写",
	},
	"Title": {
		Name: "Title",
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func() string {
				return strings.Title(s)
			}
		},
		),
		Description: "将字符串转换为Title格式(即所有单词第一个字母大写, 其余小写)",
	},
	"IsLower": {
		Name: "IsLower",
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func() bool {
				return strings.ToLower(s) == s
			}
		},
		),
		Description: "判断字符串是否为小写",
	},
	"IsUpper": {
		Name: "IsUpper",
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func() bool {
				return strings.ToUpper(s) == s
			}
		},
		),
		Description: "判断字符串是否为大写",
	},
	"IsTitle": {
		Name: "IsTitle",
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func() bool {
				return strings.Title(s) == s
			}
		},
		),
		Description: "判断字符串是否为Title格式",
	},
	"IsAlpha": {
		Name: "IsAlpha",
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func() bool {
				return utils.MatchAllOfRegexp(s, `^[a-zA-Z]+$`)
			}
		},
		),
		Description: "判断字符串是否为字母",
	},
	"IsDigit": {
		Name: "IsDigit",
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func() bool {
				return utils.MatchAllOfRegexp(s, `^[0-9]+$`)
			}
		},
		),
		Description: "判断字符串是否为数字",
	},
	"IsAlnum": {
		Name: "IsAlnum",
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func() bool {
				return utils.MatchAllOfRegexp(s, `^[a-zA-Z0-9]+$`)
			}
		},
		),
		Description: "判断字符串是否为字母或数字",
	},
	"IsPrintable": {
		Name: "IsPrintable",
		HandlerFactory: NewStringMethodFactory(func(s string) interface{} {
			return func() bool {
				return utils.MatchAllOfRegexp(s, `^[\x20-\x7E]+$`)
			}
		},
		),
		Description: "判断字符串是否为可打印字符",
	},
}

func init() {
	aliasStringBuildinMethod("ReplaceAll", "Replace")
	aliasStringBuildinMethod("Find", "IndexOf")
	aliasStringBuildinMethod("Rfind", "LastIndexOf")
	aliasStringBuildinMethod("StartsWith", "HasPrefix")
	aliasStringBuildinMethod("EndsWith", "HasSuffix")
}

func aliasStringBuildinMethod(origin string, target string) {
	if i, ok := stringBuildinMethod[origin]; ok {
		stringBuildinMethod[target] = i
		i.Name = target
	}
}
