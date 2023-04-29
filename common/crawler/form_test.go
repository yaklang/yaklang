package crawler

import (
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 接触最基础的表单情况
func TestCrawler_ParseForm(t *testing.T) {
	//log.SetLevel(logTraceLevel)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">

	<form action="login.php" method="post">

	<fieldset>

			<label for="user">Username</label> <input type="text" class="loginInput" size="20" name="username"><br />


			<label for="pass">Password</label> <input type="password" class="loginInput" AUTOCOMPLETE="off" size="20" name="password"><br />

			<br />

			<p class="submit"><input type="submit" value="Login" name="Login"></p>

	</fieldset>

	<input type='hidden' name='user_token' value='5a08ed1e1bb2aa6f7b1bd2ba2d7f1529' />

	</form>
`))
	}))
	defer ts.Close()

	flag := false
	test := assert.New(t)
	c, err := NewCrawler(ts.URL, WithOnRequest(func(req *Req) {
		if req.request.Method == "POST" {
			flag = true
			println(string(req.requestRaw))
			if !req.IsLoginForm() {
				panic(1)
			}
		}
	}))
	if err != nil {
		test.Fail(err.Error())
		return
	}

	err = c.Run()
	if err != nil {
		return
	}

	test.True(flag)

	//if true {
	//	var wg sync.WaitGroup
	//	for range make([]int, 1000) {
	//		wg.Add(1)
	//		go func() {
	//			wg.Done()
	//			flag := false
	//			c, err := NewCrawler(ts.URL, WithOnRequest(func(Req *Req) {
	//				if Req.request.Method == "POST" {
	//					flag = true
	//					utils.HttpShow(Req.request)
	//				}
	//			}))
	//			if err != nil {
	//				return
	//			}
	//
	//			err = c.Run()
	//			if err != nil {
	//				return
	//			}
	//
	//			if !flag {
	//				log.Error("FAILED..")
	//			}
	//
	//		}()
	//	}
	//}
	//config := &CrawlerConfig{}
	//u, _ := url.Parse(ts.URL)
	//err := config.AddAllowedDomainFilter(u.Host)
	//if err != nil {
	//	t.Logf("config allow domain filter failed: %s", err)
	//	t.FailNow()
	//}
	//crawler, err := NewCrawler(context.Background(), config)
	//if err != nil {
	//	t.Logf("create crawler failed: %s", err)
	//	t.FailNow()
	//}
	//crawler.GetCollyCollector().OnError(func(response *colly.Response, e error) {
	//	log.Errorf("response: %s err: %s", response.Request.URL, e)
	//	t.FailNow()
	//})
	//
	//flag := false
	//crawler.OnCreatedCollyRequest(func(request *colly.Request) {
	//	t.Logf("output url[%s]", request.URL.String())
	//	if request.URL.Path == "/login.php" {
	//		flag = true
	//	}
	//})
	//
	//crawler.Visit(ts.URL)
	//crawler.Wait()
	//
	//if !flag {
	//	t.Log("cannot found login form in login.php")
	//	t.FailNow()
	//}
}

// 解析上传文件表单
func TestCrawler_ParseForm2(t *testing.T) {
	//log.SetLevel(logTraceLevel)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
	<form action="upload.php" method="post" enctype="multipart/form-data">
    Select image to upload:
    <input type="file" name="fileToUpload" id="fileToUpload">
    <input type="submit" value="Upload Image" name="submit">
</form>
`))
	}))
	defer ts.Close()

	//config := &CrawlerConfig{}
	//u, _ := url.Parse(ts.URL)
	//err := config.AddAllowedDomainFilter(u.Host)
	//if err != nil {
	//	t.Logf("config allow domain filter failed: %s", err)
	//	t.FailNow()
	//}
	//crawler, err := NewCrawler(context.Background(), config)
	//if err != nil {
	//	t.Logf("create crawler failed: %s", err)
	//	t.FailNow()
	//}
	//crawler.GetCollyCollector().OnError(func(response *colly.Response, e error) {
	//	log.Errorf("response: %s err: %s", response.Request.URL, e)
	//	t.FailNow()
	//})
	//
	//flag := false
	//multiPartBodyTest := abool.New()
	//crawler.OnCreatedCollyRequest(func(request *colly.Request) {
	//	log.Infof("output url[%s]", request.URL.String())
	//	if request.URL.Path == "/upload.php" {
	//		flag = true
	//		data, err := ioutil.ReadAll(request.Body)
	//		if err != nil {
	//			t.Logf("read data failed: %s", err)
	//		}
	//
	//		if strings.Contains(string(data), `name="fileToUpload";`) {
	//			multiPartBodyTest.Set()
	//		}
	//		log.Infof("data: %s", string(data))
	//	}
	//})
	//
	//crawler.Visit(ts.URL)
	//crawler.Wait()
	//
	//if !flag {
	//	t.Log("cannot found login form in login.php")
	//	t.FailNow()
	//}
	//
	//assert.True(t, multiPartBodyTest.IsSet(), "multipart/form-data verify failed")
}

/*
   <div class="vulnerable_code_area">
           <form name="XSS" action="#" method="GET">
                   <p>
                           What's your name?
                           <input type="text" name="name">
                           <input type="submit" value="Submit">
                   </p>

           </form>
           <pre>Hello crawler-aGVMD</pre>
   </div>
*/

func TestCrawler_ParseForm_1(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(`
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
	<form action="#" id="usrform">
 Name: <input type="text" name="usrname">
        <textarea name="comment">aaa</textarea> 
 <input type="submit">
</form>
`))
	}))
	_ = ts
}

// 解析带有 textarea 表单情形
func TestCrawler_ParseForm_Textarea(t *testing.T) {
	//log.SetLevel(logTraceLevel)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
	<form action="#" id="usrform">
 Name: <input type="text" name="usrname">
        <textarea name="comment">aaa</textarea> 
 <input type="submit">
</form>

`))
	}))
	defer ts.Close()

	//config := &CrawlerConfig{}
	//u, _ := url.Parse(ts.URL)
	//err := config.AddAllowedDomainFilter(u.Host)
	//if err != nil {
	//	t.Logf("config allow domain filter failed: %s", err)
	//	t.FailNow()
	//}
	//crawler, err := NewCrawler(context.Background(), config)
	//if err != nil {
	//	t.Logf("create crawler failed: %s", err)
	//	t.FailNow()
	//}
	//crawler.GetCollyCollector().OnError(func(response *colly.Response, e error) {
	//	log.Errorf("response: %s err: %s", response.Request.URL, e)
	//	t.FailNow()
	//})
	//
	//var flag bool
	//crawler.OnCreatedCollyRequest(func(request *colly.Request) {
	//	log.Infof("output url[%s]", request.URL.String())
	//
	//	if strings.Contains(request.URL.String(), "comment=") {
	//		flag = true
	//	}
	//})
	//
	//crawler.Visit(ts.URL)
	//crawler.Wait()
	//
	//assert.True(t, flag, "cannot parse textarea element")
}
