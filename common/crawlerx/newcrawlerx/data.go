// Package newcrawlerx
// @Author bcy2007  2023/3/7 14:24
package newcrawlerx

import "github.com/go-rod/rod/lib/proto"

var defaultInputMap = map[string]string{
	"admin":    "admin",
	"password": "password",
	"captcha":  "captcha",
	"username": "admin",
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

var elementAttribute = []string{
	"placeholder", "id", "name", "value", "alt",
}

var extraUrlKeywords = []string{
	".ico",
}

//var extraUrlKeyword = ".ico"

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

type limitLevel int

const (
	unlimited    limitLevel = 0
	lowLevel     limitLevel = 1
	midLevel     limitLevel = 2
	highLevel    limitLevel = 3
	extremeLevel limitLevel = 4
)

type scanRangeLevel int

const (
	mainDomain scanRangeLevel = 0
	subDomain  scanRangeLevel = 1
)

var scanRangeMap = map[scanRangeLevel]func(string) string{
	mainDomain: mainDomainRange,
	subDomain:  subDomainRange,
}

var inputStringElementTypes = []string{
	"text", "password", "textarea", "search",
}
