package teststh

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/crawlerx/core"
	"github.com/yaklang/yaklang/common/log"
	"golang.org/x/net/html"
	"net/http"
)

func visit(n *html.Node) bool {
	if n.Type == html.ElementNode && n.Data == "input" {
		log.Infof("%s", n)
		for _, a := range n.Attr {
			if a.Key == "type" && a.Val == "submit" {
				return true
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		//log.Info(c)
		if visit(c) {
			return true
		}
	}
	return false
}

func Test2() {
	crawler := &core.SimpleTestCrawler{}
	crawler.Init()
	crawler.NewPageDetectTest("http://testphp.vulnweb.com/")
}

func Test() {
	//url := "http://testphp.vulnweb.com/"
	//page := rod.New().MustConnect().MustPage(url)
	//selector := "#search > form > input[type=submit]:nth-child(3)"
	//page.MustWaitLoad()
	//selector := "input"
	//page.MustElement("#search > form > input[type=submit]:nth-child(3)").MustClick()
	//page.MustWaitLoad()
	//length, _ := page.Eval(`()=>history.length`)
	//fmt.Println(length.Value.Int())
	//page.Eval(`()=>document.querySelector("%s")`)
	//query := fmt.Sprintf(`()=>document.querySelector("%s")`, selector)
	//result, err := page.ElementsByJS(rod.Eval(query))
	//result, _ := page.Elements(selector)
	//result, _ := page.Race().Element(selector).Do()
	//result, _ := page.Search(selector)
	//result, element, _ := page.Has(selector)
	//fmt.Println(result, element)
	//value := page.MustEval(query)
	//fmt.Println(value)
	//fmt.Println(page.MustElements(selector))
	//defer func() { fmt.Println(recover()) }()
	//var c tag.Conf
	//conf := c.GetConf()
	//fmt.Println(conf.Host)
	//fmt.Println(conf.Devs[0])
	//fmt.Println(conf.FilterTypes["813-http"])
	//L := lua.NewState()
	//defer L.Close()
	//if err := L.DoString(`print("hello")`); err != nil {
	//	panic(err)
	//}
	// Create a browser launcher
	//urlStr := "http://bcy:password@127.0.0.1:8083"
	//u, _ := url.Parse(urlStr)
	//log.Info(u)
	//l := launcher.MustNewManaged()

	host := core.UploadServer()
	log.Info(host)
	//
	l := launcher.New()
	// Pass '--proxy-server=127.0.0.1:8081' argument to the browser on launch
	l.Set(flags.ProxyServer, "http://127.0.0.1:8080")
	//l.Proxy("127.0.0.1:8080")
	// Launch the browser and get debug URL
	controlURL, _ := l.Launch()

	// Connect to the newly launched browser
	browser := rod.New().ControlURL(controlURL).MustConnect()
	browser.MustIgnoreCertErrors(true)

	hijackRouter := browser.HijackRequests()
	hijackRouter.MustAdd("*", func(hijack *rod.Hijack) {

		client := http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		err := hijack.LoadResponse(&client, true)
		if err != nil {
			hijack.ContinueRequest(&proto.FetchContinueRequest{})
		}
		//log.Info(err)
	})
	go func() {
		hijackRouter.Run()
	}()

	// Handle proxy authentication pop-up
	//go browser.MustHandleAuth("user", "password")() // <-- Notice how HandleAuth returns
	//     a function that must be
	//     started as a goroutine!

	// Ignore certificate errors since we are using local insecure proxy
	//page := browser.MustPage("https://183.129.247.155/js/app.js")
	//log.Info(page.MustHTML())
	page := browser.MustPage("http://43.206.141.198/vul/infoleak/infoleak.php")
	page.MustWaitLoad()
	result := page.MustEval(core.CommentMatch)
	log.Info(result)
	for _, r := range result.Arr() {
		log.Info(r.Str())
	}

	//page := browser.MustPage(host)
	//page.MustElement(`input[name="upload"]`).MustSetFiles("/Users/chenyangbao/1.txt")
	//page.MustElement(`input[name="submit"]`).MustClick()
	//
	//log.Printf(
	//	"qqqqq",
	//	page.MustElement("#result").MustText(),
	//)
}
