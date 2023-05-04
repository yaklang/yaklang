package utils

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"regexp"
	"strconv"
	"strings"
)

var (
	wordsMap = map[string]int{
		"alpha": 1,
		"beta":  2,
		"gamma": 3,
		"rc":    4,
		//" ":      5,
		"ga":     6,
		"patch":  6,
		"sp":     6,
		"update": 6,
	}
)

type versionPart struct {
	Type  string
	value string
	level int
}

// VersionGreater v1 大于 v2 返回 true
func VersionGreater(v1, v2 string) bool {
	res, err := VersionCompare(v1, v2)
	if err != nil {
		log.Errorf("version compare error : %v", err)
		panic(err)
	}

	if res == 1 {
		return true
	} else {
		return false
	}

}

// VersionGreaterEqual v1 大于等于 v2 返回 true
func VersionGreaterEqual(v1, v2 string) bool {
	res, err := VersionCompare(v1, v2)
	if err != nil {
		log.Errorf("version compare error : %v", err)
		panic(err)
	}
	if res == 0 || res == 1 {
		return true
	} else {
		return false
	}
}

// VersionEqual v1 等于 v2 返回 true
func VersionEqual(v1, v2 string) bool {
	res, err := VersionCompare(v1, v2)
	if err != nil {
		log.Errorf("version compare error : %v", err)
		panic(err)
	}
	if res == 0 {
		return true
	} else {
		return false
	}
}

// VersionLessEqual v1 小于等于 v2 返回true
func VersionLessEqual(v1, v2 string) bool {
	res, err := VersionCompare(v1, v2)
	if err != nil {
		log.Errorf("version compare error : %v", err)
		panic(err)
	}

	if res == 0 || res == -1 {
		return true
	} else {
		return false
	}
}

// VersionLess v1 小于 v2 返回true
func VersionLess(v1, v2 string) bool {
	res, err := VersionCompare(v1, v2)
	if err != nil {
		log.Errorf("version compare error : %v", err)
		panic(err)
	}

	if res == -1 {
		return true
	} else {
		return false
	}
}

// VersionCompare 泛用形的版本比较,传入(p1,p2 string), p1>p2返回1,nil, p1<p2返回-1,nil, p1==p2返回0,nil, 比较失败返回 -2,err
func VersionCompare(v1, v2 string) (int, error) {
	// 验证是否是复合标准的版本号,主要是要没有空格
	flag1, err := versionCheck(v1)
	if err != nil {
		return -2, err
	}
	flag2, err := versionCheck(v2)
	if err != nil {
		return -2, err
	}

	if flag2 && flag1 {
		return -2, errors.Errorf("version compare error : %v", "not format veriosn string")
	}

	//切割版本
	v1Tokens, err := versionSplit([]byte(v1))
	if err != nil {
		return -2, err
	}
	v2Tokens, err := versionSplit([]byte(v2))
	if err != nil {
		return -2, err
	}

	var length int
	var flag = true
	if len(v1Tokens) > len(v2Tokens) {
		length = len(v2Tokens)
		v2Tokens = append(v2Tokens, versionPart{Type: "words", value: " ", level: 5})
	} else if len(v1Tokens) < len(v2Tokens) {
		length = len(v1Tokens)
		v1Tokens = append(v1Tokens, versionPart{Type: "words", value: " ", level: 5})
	} else {
		length = len(v1Tokens)
		flag = false
	}

	for i := 0; i < length; i++ {
		if v1Tokens[i].Type != v2Tokens[i].Type {
			return -2, errors.Errorf("compare version fail: %v", "Incomparable version")
		}

		switch v1Tokens[i].Type {
		case "number":
			res, err := compareNumber(v1Tokens[i], v2Tokens[i])
			if err != nil {
				return -2, err
			}
			if res == 0 {
				continue
			} else {
				return res, nil
			}
		case "separator":
			res, err := compareSeparator(v1Tokens[i], v2Tokens[i])
			if err != nil {
				return -2, err
			}
			if res == 0 {
				continue
			} else {
				return res, nil
			}
		case "orderLetter":
			res, err := compareOrderLetter(v1Tokens[i], v2Tokens[i])
			if err != nil {
				return -2, err
			}
			if res == 0 {
				continue
			} else {
				return res, nil
			}
		case "words":
			res, err := compareWords(v1Tokens[i], v2Tokens[i])
			if err != nil {
				return -2, err
			}
			if res == 0 {
				continue
			} else {
				return res, nil
			}
		}
	}

	if flag {
		return compareSpecial(v1Tokens[length], v2Tokens[length])
	}

	return 0, nil
}

func versionCheck(v string) (bool, error) {

	flag, err := regexp.MatchString("\\s", v)
	if err != nil {
		log.Errorf("version string check error:%v", err)
		return false, errors.Wrap(err, "regexp Match err in versionCheck")
	}
	return flag, nil
}

func versionSplit(v []byte) ([]versionPart, error) {
	var versionTokens []versionPart
	current := 0

	for current < len(v) {
		if charIsNumber(v[current]) {
			value := getNumber(v, current)
			versionTokens, current = addNumber(versionTokens, current, value)
		} else if charIsLetter(v[current]) {
			if !charIsLetter(peek(v, current)) {
				versionTokens, current = addOrderLetter(versionTokens, current, string(v[current]))
			} else {
				value := getString(v, current)
				level, ok := wordsMap[value]
				if ok {
					versionTokens, current = addWords(versionTokens, current, value, level)
				} else {
					versionTokens, current = addSeparator(versionTokens, current, value)
				}
			}
		} else {
			value := getString(v, current)
			versionTokens, current = addSeparator(versionTokens, current, value)

		}
	}
	return versionTokens, nil
}

// addNumber 添加纯数字的token
func addNumber(versionTokens []versionPart, current int, value string) ([]versionPart, int) {
	token := versionPart{Type: "number", value: value}
	versionTokens = append(versionTokens, token)
	return versionTokens, current + len(value)
}

// addSeparator 添加分割符的token
func addSeparator(versionTokens []versionPart, current int, value string) ([]versionPart, int) {
	token := versionPart{Type: "separator", value: value}
	versionTokens = append(versionTokens, token)
	return versionTokens, current + len(value)
}

// addOrderLetter 添加有序字符的token
func addOrderLetter(versionTokens []versionPart, current int, value string) ([]versionPart, int) {
	token := versionPart{Type: "orderLetter", value: value}
	versionTokens = append(versionTokens, token)
	return versionTokens, current + 1
}

// addWords 添加有意义的字符串的token
func addWords(versionTokens []versionPart, current int, value string, level int) ([]versionPart, int) {
	token := versionPart{
		Type:  "words",
		value: value,
		level: level,
	}
	versionTokens = append(versionTokens, token)
	return versionTokens, current + len(value)
}

func getString(v []byte, current int) string {
	end := current + 1
	if charIsMark(v[current]) {
		for end < len(v) && charIsMark(v[end]) {
			end = end + 1
		}
	}

	if charIsLetter(v[current]) {
		for end < len(v) && charIsLetter(v[end]) {
			end = end + 1
		}
	}

	value := string(v[current:end])

	return value
}

func getNumber(v []byte, current int) string {
	end := current + 1

	for end < len(v) && charIsNumber(v[end]) {
		end = end + 1
	}

	return string(v[current:end])
}

func charIsNumber(one byte) bool {
	return one >= '0' && one <= '9'
}

func charIsLetter(one byte) bool {
	return (one >= 'a' && one <= 'z') || (one >= 'A' && one <= 'Z')
}

func charIsMark(one byte) bool {
	return !charIsNumber(one) && !charIsLetter(one)
}

func peek(v []byte, current int) byte {
	if current+1 < len(v) {
		return v[current+1]
	} else {
		return ' '
	}
}

func compareNumber(p1 versionPart, p2 versionPart) (int, error) {
	p1Number, err := strconv.Atoi(p1.value)
	if err != nil {
		return -2, errors.Wrap(err, "versionPart atoi error")
	}

	p2Number, err := strconv.Atoi(p2.value)
	if err != nil {
		return -2, errors.Wrap(err, "versionPart atoi error")
	}

	if p1Number > p2Number {
		return 1, nil
	} else if p1Number < p2Number {
		return -1, nil
	} else {
		return 0, nil
	}

}

func compareSeparator(p1 versionPart, p2 versionPart) (int, error) {
	if p1.value == p2.value {
		return 0, nil
	} else {
		return -2, errors.Errorf("compare separator mark error: %v", "Unable to compare")
	}
}

func compareOrderLetter(p1 versionPart, p2 versionPart) (int, error) {
	p1Letter := strings.ToLower(p1.value)
	p2Letter := strings.ToLower(p2.value)

	return strings.Compare(p1Letter, p2Letter), nil
}

func compareWords(p1 versionPart, p2 versionPart) (int, error) {
	if p1.level > p2.level {
		return 1, nil
	} else if p1.level < p2.level {
		return -1, nil
	}

	if p1.value == p2.value {
		return 0, nil
	}

	return -2, errors.Errorf("compare version part error:%v", "Unable to compare")
}

func compareSpecial(p1 versionPart, p2 versionPart) (int, error) {
	if p1.Type == "words" && p1.value == " " {
		if p2.Type == "words" {
			return compareWords(p1, p2)
		} else {
			return -1, nil
		}
	}

	if p2.Type == "words" && p2.value == " " {
		if p1.Type == "words" {
			return compareWords(p1, p2)
		} else {
			return 1, nil
		}
	}

	return -2, errors.Errorf("compare error : %v", "Unable to compare")
}
