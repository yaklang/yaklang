// Package crawlerx
// @Author bcy2007  2023/8/16 11:57
package crawlerx

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestElementClick(t *testing.T) {
	test := assert.New(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(crawlerTestHtml))
	}))
	defer server.Close()

	targetUrl := server.URL
	//targetUrl := "http://testphp.vulnweb.com/"

	// browser start
	launch := launcher.New()
	controlUrl, err := launch.Launch()
	if err != nil {
		t.Errorf("launcher launch error: %v", err.Error())
		return
	}
	browser := rod.New()
	browser.ControlURL(controlUrl)
	err = browser.Connect()
	if err != nil {
		t.Errorf("browser connect error: %v", err.Error())
		return
	}
	defer func() {
		err := browser.Close()
		if err != nil {
			t.Errorf("browser close error: %v", err.Error())
		}
	}()
	page, err := browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		t.Errorf("browser create page error: %v", err.Error())
		return
	}
	time.Sleep(time.Second)
	err = page.Navigate(targetUrl)
	if err != nil {
		t.Errorf("page navigate %v error: %v", targetUrl, err.Error())
		return
	}
	err = page.WaitLoad()
	if err != nil {
		t.Errorf("page wait load error: %v", err.Error())
		return
	}
	elements, err := page.Elements("#search > form > input[type=submit]:nth-child(3)")
	if err != nil {
	}
	element := elements.First()
	if element == nil {
	}
	_, err = element.Eval(`()=>this.click()`)
	if err != nil {
		t.Errorf("click error: %v", err.Error())
	}
	time.Sleep(500 * time.Millisecond)
	newUrl := page.MustWaitLoad().MustInfo().URL
	//t.Logf("new url: %v", newUrl)
	test.Equal(server.URL+"/search.php?test=query", newUrl)
}
