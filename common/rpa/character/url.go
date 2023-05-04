package character

import (
	"regexp"
	"strings"
)

func LastSubAnalysis(str string) string {
	ra2z := regexp.MustCompile("[^a-zA-Z]+")
	result := ra2z.ReplaceAllString(str, "")
	return result
}

func getPartData(url string, part int) string {
	if strings.Contains(url, "/") {
		parts := strings.Split(url, "/")
		length := len(parts)
		if part >= length {
			return ""
		}
		return parts[part]
	}
	return url
}

func cutLastSubUrl(url string) string {
	regNum := regexp.MustCompile("[^0-9]+")
	blocks := strings.Split(url, "/")
	length := len(blocks)
	sub := 1
	if strings.HasSuffix(url, "/") {
		sub++
	}
	for sub <= length {
		temp := blocks[length-sub]
		if regNum.MatchString(temp) {
			return temp
		}
		sub++
	}
	return ""
}

func CutLastSubUrl(url string) string {
	if strings.Contains(url, "?") && strings.Contains(url, "=") {
		parts := strings.Split(url, "?")
		mainPart := parts[0]
		return cutLastSubUrl(mainPart)
	}
	return cutLastSubUrl(url)
}
