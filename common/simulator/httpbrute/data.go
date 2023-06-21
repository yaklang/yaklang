// Package httpbrute
// @Author bcy2007  2023/6/21 11:05
package httpbrute

type loginDetectMode int

const (
	UrlChangeMode     loginDetectMode = 0
	HtmlChangeMode    loginDetectMode = 1
	DefaultChangeMode loginDetectMode = -1
)
