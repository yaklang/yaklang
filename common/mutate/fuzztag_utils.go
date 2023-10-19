package mutate

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"
	"strings"
	"time"
)

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

// 解析失败会panic，只能在fuzztagx中使用
func atoi(s string) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return v
}

// 读取一个分隔符最后出现位置的部分
func sepToEnd(s string, sep string) (string, string) {
	if strings.LastIndex(s, sep) < 0 {
		return s, ""
	}
	return s[:strings.LastIndex(s, sep)], s[strings.LastIndex(s, sep)+1:]
}
