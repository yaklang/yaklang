package core

import (
	"encoding/base64"
	"fmt"
	"github.com/go-rod/rod/lib/proto"
	"io/ioutil"
	"net"
	"net/http"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils"
)

func (crawler *CrawlerX) Start() {
	//crawler.pageSizedWaitGroup.AddWithContext(crawler.rootContext)
	crawler.PageSizedGroup().AddWithContext(crawler.RootContext())
	//go crawler.browser.EachEvent(
	//	func(e *proto.TargetTargetCreated) {
	//		targetID := e.TargetInfo.TargetID
	//		page, err := crawler.browser.PageFromTarget(targetID)
	//		defer page.Close()
	//		if err != nil {
	//			return
	//		}
	//		page.WaitLoad()
	//		//go crawler.VisitPage(&generalPage)
	//		go func() {
	//			generalPage := GeneralPage{page, 0}
	//			defer crawler.pageSizedWaitGroup.Done()
	//			crawler.VisitPage(&generalPage)
	//		}()
	//	},
	//)()
	go func() {
		crawler.VisitUrl(crawler.targetUrl, 0)
	}()
	if crawler.sendInfoChannel != nil {
		defer close(crawler.sendInfoChannel)
	}
	//crawler.pageSizedWaitGroup.Wait()
	crawler.PageSizedGroup().Wait()
	log.Info("end")
}

func (crawler *CrawlerX) StartRemote() {
	//go crawler.browser.EachEvent(
	//	func(e *proto.TargetTargetCreated) {
	//		targetID := e.TargetInfo.TargetID
	//		page, err := crawler.browser.PageFromTarget(targetID)
	//		defer page.Close()
	//		if err != nil {
	//			return
	//		}
	//		generalPage := GeneralPage{page, 0}
	//		go crawler.VisitPage(&generalPage)
	//	},
	//)()
	go func() {
		crawler.VisitUrl(crawler.targetUrl, 0)
	}()
}

func (crawler *CrawlerX) Monitor() {
	crawler.pageSizedWaitGroup.AddWithContext(crawler.rootContext)
	go func() {
		crawler.monitor()
	}()
	defer close(crawler.sendInfoChannel)
	crawler.pageSizedWaitGroup.Wait()
	//time.Sleep(60 * time.Second)
	log.Info("end")
}

func (crawler *CrawlerX) monitor() {
	//UploadServer()
	defer crawler.pageSizedWaitGroup.Done()
	page := crawler.GetPage(
		proto.TargetCreateTarget{URL: crawler.targetUrl},
		0,
	)
	//page.MustWaitLoad()
	//time.Sleep(time.Second)
	//page.MustWaitLoad()
	//page.MustElement(`input[name="upload"]`).MustSetFiles("/Users/chenyangbao/1.txt")
	//time.Sleep(time.Second)
	//page.MustWaitLoad()
	//time.Sleep(time.Second)
	//page.MustWaitLoad()
	//page.MustElement(`input[name="submit"]`).Click(proto.InputMouseButtonLeft)
	//page.MustWaitLoad()
	//log.Info("qqqq", page.MustHTML())

	page.Navigate("http://192.168.0.3/login.php")
	page.MustWaitLoad()
	page.MustElement("#content > form > fieldset > input:nth-child(2)").MustInput("admin")
	page.MustElement("#content > form > fieldset > input:nth-child(5)").MustInput("password")
	page.MustElement("#content > form > fieldset > p > input[type=submit]").MustClick()
	page.MustWaitLoad()
	page.Navigate("http://192.168.0.3/vulnerabilities/upload/")
	page.MustWaitLoad()
	page.MustEval(setFileUploadInfo)
	page.MustElement("#main_body > div > div > form > input[type=file]:nth-child(4)").MustSetFiles("/Users/chenyangbao/1.txt")
	log.Info(page.MustEval(getFileUploadInfo).String())
	page.MustWaitLoad()
	time.Sleep(time.Second)
	page.MustElement("#main_body > div > div > form > input[type=submit]:nth-child(7)").MustClick()
	time.Sleep(time.Second)
	page.MustWaitLoad()
}

func UploadServer() string {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		_, _ = fmt.Fprint(res, uploadHTML)
	})
	mux.HandleFunc("/upload", func(res http.ResponseWriter, req *http.Request) {
		f, _, err := req.FormFile("upload")
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}
		defer func() { _ = f.Close() }()

		buf, err := ioutil.ReadAll(f)
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}

		_, _ = fmt.Fprintf(res, resultHTML, len(buf))
	})
	l, _ := net.Listen("tcp4", "0.0.0.0:0")
	go func() { _ = http.Serve(l, mux) }()
	return "http://" + l.Addr().String()
}

const (
	uploadHTML = `<!doctype html>
<html>
<body>
  <form method="POST" action="/upload" enctype="multipart/form-data">
    <input name="upload" type="file"/>
    <input name="submit" type="submit"/>
  </form>
</body>
</html>`

	resultHTML = `<!doctype html>
<html>
<body>
  <div id="result">%d</div>
</body>
</html>`
)

func (crawler *CrawlerX) PageScreenShot(urlStr string) (string, error) {
	page := crawler.GetPage(
		proto.TargetCreateTarget{URL: "about:blank"},
		0,
	)
	err := page.Navigate(urlStr)
	if err != nil {
		return "", utils.Errorf("navigate page %s error: %s", urlStr, err)
	}
	err = page.WaitLoad()
	if err != nil {
		return "", err
	}
	wait := page.MustWaitRequestIdle()
	wait()
	pngBytes, err := page.Screenshot(false, nil)
	if err != nil {
		return "", utils.Errorf("page %s screen shot error: %s", urlStr, err)
	}
	pngBase64 := base64.StdEncoding.EncodeToString(pngBytes)
	return "data:image/png;base64," + pngBase64, nil
}
