package detect

import (
	"strings"
)

const HighLevel = 3
const MediumLevel = 2
const LowLevel = 1
const UnLimit = 0

func GetURLRepeatCheck(repeatLevel int) func(string, string) string {
	switch repeatLevel {
	case UnLimit:
		return unLimitFunc
	case LowLevel:
		return lowFunc
	case MediumLevel:
		return mediumFunc
	case HighLevel:
		return highFunc
	default:
		return unLimitFunc
	}
}

func unLimitFunc(urlStr, method string) string {
	return strings.ToLower(method) + " " + urlStr
}

func lowFunc(urlStr, method string) string {
	separates := strings.Split(urlStr, "?")
	if len(separates) < 2 {
		return strings.ToLower(method) + " " + urlStr
	}
	var resultParams string
	queries := strings.Split(separates[1], "&")
	for _, query := range queries {
		params := strings.Split(query, "=")
		if len(params) < 1 {
			continue
		}
		resultParams += params[0] + "&"
	}
	if resultParams != "" {
		resultParams = resultParams[:len(resultParams)-1]
	}
	return strings.ToLower(method) + " " + separates[0] + "?" + resultParams
}

func mediumFunc(urlStr, method string) string {
	separates := strings.Split(urlStr, "?")
	if len(separates) < 2 {
		return strings.ToLower(method) + " " + urlStr
	}
	return strings.ToLower(method) + " " + separates[0]
}

func highFunc(urlStr, _ string) string {
	separates := strings.Split(urlStr, "?")
	if len(separates) < 2 {
		return urlStr
	}
	return separates[0]
}
