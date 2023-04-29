// Package newcrawlerx
// @Author bcy2007  2023/3/23 10:54
package newcrawlerx

func StartCrawler(url string, opts ...ConfigOpt) chan ReqInfo {
	ch := make(chan ReqInfo)
	opts = append(opts, WithResultChannel(ch))
	crawler := NewCrawler(url, opts...)
	go crawler.Run()
	return ch
}
