// Package newcrawlerx
// @Author bcy2007  2023/3/7 11:33
package newcrawlerx

type ReqInfo interface {
	Type() string

	Url() string
	Method() string

	RequestHeaders() map[string]string
	RequestBody() string

	StatusCode() int
	ResponseHeaders() map[string]string
	ResponseBody() string

	Screenshot() string

	From() string
}
