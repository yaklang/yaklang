package yaktest

import "testing"

func TestCrawler(t *testing.T) {
	cases := []YakTestCase{
		{
			Name: "爬虫测试，增加一下额外配置：每个路径下都要检测的接口",
			Src: `
res, err = crawler.Start(
	"159.65.125.15:80", 
	crawler.maxRequest(500),
	crawler.autoLogin("admin", "password"),
	// crawler.urlRegexpExclude("(?i).*?\/?(logout|reset|delete|setup).*"),
)
die(err)
dump(res)
for crawlerReq = range res {
	if crawlerReq == nil {
		continue
	}
	println(crawlerReq.Url())
}
`,
		},
	}

	Run("Yak Crawler 测试", t, cases...)
}
