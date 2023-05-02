package core

import (
	"github.com/go-rod/rod"
	"strings"
	"yaklang/common/simulator/config"
	"yaklang/common/utils"
)

func calculateGroupRelevance(target string, originGroup []string) float32 {
	//return 0.0
	var likeValue float32
	for _, origin := range originGroup {
		tempValue := calculateRelevance(target, origin)
		if tempValue > likeValue {
			likeValue = tempValue
		}
	}
	return likeValue
}

func calculateRelevance(target, origin string) float32 {
	repeatStr := getRepeatStr(target, origin)
	return float32(len(repeatStr)) / float32(utils.Max(len(target), len(origin)))
}

func getRepeatStr(origin, source string) string {
	originBytes := []byte(origin)
	sourceBytes := []byte(source)
	var maxTemp []byte
	for num, ob := range originBytes {
		i := 1
		if ob != sourceBytes[0] {
			continue
		}
		temp := []byte{ob}
		if num+i < len(originBytes) {
			for originBytes[num+i] == sourceBytes[i] {
				temp = append(temp, originBytes[num+i])
				i++
				if i >= len(sourceBytes) {
					break
				}
				if num+i >= len(originBytes) {
					break
				}
			}
		}
		if len(temp) >= len(maxTemp) {
			maxTemp = temp
		}
	}
	return string(maxTemp)
}

func GetAttribute(element *rod.Element, attributeStr string) string {
	attribute, err := element.Attribute(attributeStr)
	if err != nil {
		return ""
	}
	if attribute == nil {
		return ""
	}
	result := strings.ToLower(*attribute)
	return result
}

func GetWholeAttributesStr(element *rod.Element) string {
	attributes := config.ElementAttribute
	var attributesStr string
	for _, attribute := range attributes {
		tempStr := GetAttribute(element, attribute)
		if tempStr != "" {
			attributesStr += tempStr + ";"
		}
	}
	return attributesStr
}

func ContainsGroup(target string, originGroup []string) bool {
	for _, origin := range originGroup {
		if strings.Contains(target, origin) {
			return true
		}
	}
	return false
}

func SliceDelete(origin []interface{}, target interface{}) []interface{} {
	for i := 0; i < len(origin); i++ {
		if origin[i] == target {
			origin = append(origin[:i], origin[i+1:]...)
			return origin
		}
	}
	return origin
}
