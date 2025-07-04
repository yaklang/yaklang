// Package crawlerx
// @Author bcy2007  2023/7/12 16:31
package crawlerx

import "github.com/go-rod/rod/lib/proto"

var defaultInputMap = map[string]string{
	"admin":    "admin",
	"password": "password",
	"captcha":  "captcha",
	"username": "admin",
	"email":    "admin@admin.com",
	"phone":    "13900000001",
	"num":      "1",
	"code":     "1234",
	"card":     "12345678",
	"id":       "12345678",
	"url":      "testurl.com",
	"website":  "testurl.com",
}

var defaultChromeHeaders = map[string]string{
	"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif," +
		"image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
	"Accept-Encoding":           "gzip, deflate",
	"Accept-Language":           "zh-CN,zh;q=0.9,en;q=0.8,ja;q=0.7,zh-TW;q=0.6",
	"Connection":                "keep-alive",
	"Upgrade-Insecure-Requests": "1",
	"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) " +
		"Chrome/111.0.0.0 Safari/537.36",
}

type repeatLevel int

const (
	unlimited    repeatLevel = 0
	lowLevel     repeatLevel = 1
	midLevel     repeatLevel = 2
	highLevel    repeatLevel = 3
	extremeLevel repeatLevel = 4
)

var RepeatLevelMap = map[int]repeatLevel{
	0: unlimited,
	1: lowLevel,
	2: midLevel,
	3: highLevel,
	4: extremeLevel,
}

type scanRangeLevel int

const (
	mainDomain      scanRangeLevel = 0
	subDomain       scanRangeLevel = 1
	unlimitedDomain scanRangeLevel = 2
	boardDomain     scanRangeLevel = 3
)

var ScanRangeLevelMap = map[int]scanRangeLevel{
	0: mainDomain,
	1: subDomain,
	2: unlimitedDomain,
	3: boardDomain,
}

var generalScanRangeMap = map[scanRangeLevel]func(string) []string{
	mainDomain:      generalMainDomainRange,
	subDomain:       generalSubDomainRange,
	unlimitedDomain: generalUnlimitedDomainRange,
	boardDomain:     generalBoardDomainRange,
}

var elementAttribute = []string{
	"placeholder", "id", "name", "value", "alt",
}

var extraUrlKeywords = []string{
	".ico",
}

var notLoaderResourceType = []proto.NetworkResourceType{
	proto.NetworkResourceTypeImage,
	proto.NetworkResourceTypeMedia,
}

func notLoaderContains(resourceType proto.NetworkResourceType) bool {
	for _, item := range notLoaderResourceType {
		if item == resourceType {
			return true
		}
	}
	return false
}

var inputStringElementTypes = []string{
	"text", "password", "textarea", "search",
}

var defaultInvalidSuffix = []string{
	".js",
	".css",
	".xml",
	".jpg", ".jpeg", ".png",
	".mp3", ".mp4", ".ico", ".bmp",
	".flv", ".aac", ".ogg", ".avi",
	".svg", ".gif", ".woff", ".woff2",
	".doc", ".docx", ".pptx",
	".ppt", ".pdf",
	".swf",
	".json", "./",
}

var jsContentTypes = []string{
	"text/javascript",
	"application/javascript",
	"application/x-javascript",
	"application/ecmascript",
}
