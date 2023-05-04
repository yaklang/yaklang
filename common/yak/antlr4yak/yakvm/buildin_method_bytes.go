package yakvm

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"unicode"
)

func NewBytesMethodFactory(f func([]byte) interface{}) MethodFactory {
	return func(vm *Frame, i interface{}) interface{} {
		raw, ok := i.([]byte)
		if !ok {
			raw = []byte(fmt.Sprint(i))
		}
		return f(raw)
	}
}

var bytesBuildinMethod = map[string]*buildinMethod{
	"First": {
		Name:       "First",
		ParamTable: nil,
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func() rune {
				return rune(s[0])
			}
		}),
		Description: "获取字节数组的第一个字符",
	},
	"Reverse": {
		Name:       "Reverse",
		ParamTable: nil,
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func() []byte {
				// runes 是为了处理中文字符问题，这个是合理的
				runes := []rune(string(s))
				for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
					runes[i], runes[j] = runes[j], runes[i]
				}
				return []byte(string(runes))
			}
		}),
		Description: "倒序字节数组",
	},
	"Shuffle": {
		Name:       "Shuffle",
		ParamTable: nil,
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func() []byte {
				var runes = []rune(string(s))
				rand.Shuffle(len(runes), func(i, j int) {
					runes[i] = runes[j]
				})
				return []byte(string(runes))
			}
		}),
		Description: "随机打乱字节数组",
	},
	"Fuzz": {
		Name:       "Fuzz",
		ParamTable: nil,
		Snippet:    `Fuzz(${1:{"params": "value"\}})$0`,
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(i ...interface{}) []string {
				var opts []mutate.FuzzConfigOpt
				if len(i) > 0 {
					opts = append(opts, mutate.Fuzz_WithParams(i[0]))
				}

				if len(i) > 1 {
					log.Warn("string.Fuzz only need one param as {{params(...)}} source")
				}

				res, err := mutate.FuzzTagExec(string(s), opts...)
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
		ParamTable: []string{"subslice"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(subslice []byte) bool {
				if len(subslice) == 0 {
					return true
				}
				return bytes.Contains(s, subslice)
			}
		}),
		Description: "判断字节数组是否包含子字节数组",
	},
	"IContains": {
		Name:       "IContains",
		ParamTable: []string{"subslice"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(subslice []byte) bool {
				if len(subslice) == 0 {
					return true
				}
				return bytes.Contains(bytes.ToLower(s), bytes.ToLower(subslice))
			}
		}),
		Description: "判断字节数组是否包含子字节数组,忽略大小写",
	},
	"ReplaceN": {
		Name:       "ReplaceN",
		ParamTable: []string{"old", "new", "n"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(old, new []byte, n int) []byte {
				return bytes.Replace(s, old, new, n)
			}
		},
		),
		Description: "替换字节数组中的子字节数组",
	},
	"ReplaceAll": {
		Name:       "ReplaceAll",
		ParamTable: []string{"old", "new"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(old, new []byte) []byte {
				return bytes.ReplaceAll(s, old, new)
			}
		},
		),
		Description: "替换字节数组中所有的子字节数组",
	},
	"Split": {
		Name:       "Split",
		ParamTable: []string{"separator"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(sep []byte) [][]byte {
				return bytes.Split(s, sep)
			}
		},
		),
		Description: "分割字节数组",
	},
	"SplitN": {
		Name:       "SplitN",
		ParamTable: []string{"separator", "n"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(sep []byte, n int) [][]byte {
				return bytes.SplitN(s, sep, n)
			}
		},
		),
		Description: "分割字节数组，最多分割为N份",
	},
	"Join": {
		Name:       "Join",
		ParamTable: []string{"separator"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(i interface{}) []byte {
				return bytes.Join(utils.InterfaceToBytesSlice(i), s)
			}
		},
		),
		Description: "连接字节数组",
	},
	"Trim": {
		Name:            "Trim",
		ParamTable:      []string{"cutset"},
		IsVariadicParam: true,
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(cutslice ...[]byte) []byte {
				if cutslice != nil {
					return bytes.Trim(s, string(bytes.Join(cutslice, []byte{})))
				}

				return bytes.TrimSpace(s)
			}
		},
		),
		Description: "去除字节数组两端的cutset",
	},
	"TrimLeft": {
		Name:            "TrimLeft",
		ParamTable:      []string{"cutstr"},
		IsVariadicParam: true,
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(cutslice ...[]byte) []byte {
				if cutslice != nil {
					return bytes.TrimLeft(s, string(bytes.Join(cutslice, []byte{})))
				}

				return bytes.TrimLeftFunc(s, unicode.IsSpace)
			}
		}),
		Description: "去除字节数组左端的cutset",
	},
	"TrimRight": {
		Name:       "TrimRight",
		ParamTable: []string{"cutstr"}, IsVariadicParam: true,
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(cutslice ...[]byte) []byte {
				if cutslice != nil {
					return bytes.TrimRight(s, string(bytes.Join(cutslice, []byte{})))
				}

				return bytes.TrimRightFunc(s, unicode.IsSpace)
			}
		}),
		Description: "去除字节数组右端的cutset",
	},
	"HasPrefix": {
		Name:       "HasPrefix",
		ParamTable: []string{"prefix"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(prefix []byte) bool {
				return bytes.HasPrefix(s, prefix)
			}
		},
		),
		Description: "判断字节数组是否以prefix开头",
	},
	"RemovePrefix": {
		Name:       "RemovePrefix",
		ParamTable: []string{"prefix"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(prefix []byte) []byte {
				if bytes.HasPrefix(s, prefix) {
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
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(suffix []byte) bool {
				return bytes.HasSuffix(s, suffix)
			}
		},
		),
		Description: "判断字节数组是否以suffix结尾",
	},
	"RemoveSuffix": {
		Name:       "RemoveSuffix",
		ParamTable: []string{"suffix"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(suffix []byte) []byte {
				if bytes.HasSuffix(s, suffix) {
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
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			zeroBytes := []byte{'0'}
			return func(width int) []byte {
				lenOfS := len(s)
				if width <= lenOfS {
					return s
				} else {
					return append(bytes.Repeat(zeroBytes, width-lenOfS), s...)
				}
			}
		},
		),
		Description: "字节数组左侧填充0",
	},
	"Rzfill": {
		Name:       "Rzfill",
		ParamTable: []string{"width"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			zeroBytes := []byte{'0'}
			return func(width int) []byte {
				lenOfS := len(s)
				if width <= lenOfS {
					return s
				} else {
					return append(s, bytes.Repeat(zeroBytes, width-lenOfS)...)
				}
			}
		},
		),
		Description: "字节数组右侧填充0",
	},
	"Ljust": {
		Name:       "Ljust",
		ParamTable: []string{"width"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			zeroBytes := []byte{' '}
			return func(width int, fill ...[]byte) []byte {
				lenOfS := len(s)
				if width <= lenOfS {
					return s
				} else {
					fillBytes := zeroBytes
					if len(fill) > 0 {
						fillBytes = fill[0]
					}
					return append(s, bytes.Repeat(fillBytes, width-lenOfS)...)
				}
			}
		},
		),
		Description: "字节数组左侧填充空格",
	},
	"Rjust": {
		Name:       "Rjust",
		ParamTable: []string{"width"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			zeroBytes := []byte{' '}
			return func(width int, fill ...[]byte) []byte {
				lenOfS := len(s)
				if width <= lenOfS {
					return s
				} else {
					fillBytes := zeroBytes
					if len(fill) > 0 {
						fillBytes = fill[0]
					}
					return append(bytes.Repeat(fillBytes, width-lenOfS), s...)
				}
			}
		},
		),
		Description: "字节数组右侧填充空格",
	},
	"Count": {
		Name:       "Count",
		ParamTable: []string{"subslice"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(subslice []byte) int {
				return bytes.Count(s, subslice)
			}
		},
		),
		Description: "统计字节数组中subslice出现的次数",
	},
	"Find": {
		Name:       "Find",
		ParamTable: []string{"subslice"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(subslice []byte) int {
				return bytes.Index(s, subslice)
			}
		},
		),
		Description: "查找字节数组中subslice第一次出现的位置, 如果没找到则返回-1",
	},
	"Rfind": {
		Name:       "Rfind",
		ParamTable: []string{"subslice"},
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func(subslice []byte) int {
				return bytes.LastIndex(s, subslice)
			}
		},
		),
		Description: "查找字节数组中subslice最后一次出现的位置, 如果没找到则返回-1",
	},
	"Lower": {
		Name: "Lower",
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func() []byte {
				return bytes.ToLower(s)
			}
		},
		),
		Description: "将字节数组转换为小写",
	},
	"Upper": {
		Name: "Upper",
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func() []byte {
				return bytes.ToUpper(s)
			}
		},
		),
		Description: "将字节数组转换为大写",
	},
	"Title": {
		Name: "Title",
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func() []byte {
				return bytes.Title(s)
			}
		},
		),
		Description: "将字节数组转换为Title格式(即所有单词第一个字母大写, 其余小写)",
	},
	"IsLower": {
		Name: "IsLower",
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func() bool {
				return bytes.Equal(bytes.ToLower(s), s)
			}
		},
		),
		Description: "判断字节数组是否为小写",
	},
	"IsUpper": {
		Name: "IsUpper",
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func() bool {
				return bytes.Equal(bytes.ToUpper(s), s)
			}
		},
		),
		Description: "判断字节数组是否为大写",
	},
	"IsTitle": {
		Name: "IsTitle",
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func() bool {
				return bytes.Equal(bytes.Title(s), s)
			}
		},
		),
		Description: "判断字节数组是否为Title格式",
	},
	"IsAlpha": {
		Name: "IsAlpha",
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func() bool {
				return utils.MatchAllOfRegexp(s, `^[a-zA-Z]+$`)
			}
		},
		),
		Description: "判断字节数组是否为字母",
	},
	"IsDigit": {
		Name: "IsDigit",
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func() bool {
				return utils.MatchAllOfRegexp(s, `^[0-9]+$`)
			}
		},
		),
		Description: "判断字节数组是否为数字",
	},
	"IsAlnum": {
		Name: "IsAlnum",
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func() bool {
				return utils.MatchAllOfRegexp(s, `^[a-zA-Z0-9]+$`)
			}
		},
		),
		Description: "判断字节数组是否为字母或数字",
	},
	"IsPrintable": {
		Name: "IsPrintable",
		HandlerFactory: NewBytesMethodFactory(func(s []byte) interface{} {
			return func() bool {
				return utils.MatchAllOfRegexp(s, `^[\x20-\x7E]+$`)
			}
		},
		),
		Description: "判断字节数组是否为可打印字符",
	},
}

func init() {
	aliasBytesBuildinMethod("ReplaceAll", "Replace")
	aliasBytesBuildinMethod("Find", "IndexOf")
	aliasBytesBuildinMethod("Rfind", "LastIndexOf")
	aliasBytesBuildinMethod("StartsWith", "HasPrefix")
	aliasBytesBuildinMethod("EndsWith", "HasSuffix")
}

func aliasBytesBuildinMethod(origin string, target string) {
	if i, ok := bytesBuildinMethod[origin]; ok {
		bytesBuildinMethod[target] = i
		i.Name = target
	}
}
