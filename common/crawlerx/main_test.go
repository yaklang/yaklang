// Package crawlerx
// @Author bcy2007  2023/7/17 14:25
package crawlerx

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/assert"
	_ "github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/embed"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

var CrawlerTestHtml = `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN"
"http://www.w3.org/TR/html4/loose.dtd">
<html><!-- InstanceBegin template="/Templates/main_dynamic_template.dwt.php" codeOutsideHTMLIsLocked="false" -->
<head>
<meta http-equiv="Content-Type" content="text/html; charset=iso-8859-2">

<!-- InstanceBeginEditable name="document_title_rgn" -->
<title>Home of Acunetix Art</title>
<!-- InstanceEndEditable -->
<link rel="stylesheet" href="style.css" type="text/css">
<!-- InstanceBeginEditable name="headers_rgn" -->
<!-- here goes headers headers -->
<!-- InstanceEndEditable -->
<script language="JavaScript" type="text/JavaScript">
<!--
function MM_reloadPage(init) {  //reloads the window if Nav4 resized
  if (init==true) with (navigator) {if ((appName=="Netscape")&&(parseInt(appVersion)==4)) {
    document.MM_pgW=innerWidth; document.MM_pgH=innerHeight; onresize=MM_reloadPage; }}
  else if (innerWidth!=document.MM_pgW || innerHeight!=document.MM_pgH) location.reload();
}
MM_reloadPage(true);
//-->
</script>

</head>
<body> 
<div id="mainLayer" style="position:absolute; width:700px; z-index:1">
<div id="masthead"> 
  <h1 id="siteName"><a href="https://www.acunetix.com/"><img src="images/logo.gif" width="306" height="38" border="0" alt="Acunetix website security"></a></h1>   
  <h6 id="siteInfo">TEST and Demonstration site for <a href="https://www.acunetix.com/vulnerability-scanner/">Acunetix Web Vulnerability Scanner</a></h6>
  <div id="globalNav"> 
      	<table border="0" cellpadding="0" cellspacing="0" width="100%"><tr>
	<td align="left">
		<a href="index.php">home</a> | <a href="categories.php">categories</a> | <a href="artists.php">artists
		</a> | <a href="disclaimer.php">disclaimer</a> | <a href="cart.php">your cart</a> | 
		<a href="guestbook.php">guestbook</a> | 
		<a href="AJAX/index.php">AJAX Demo</a>
	</td>
	<td align="right">
		</td>
	</tr></table>
  </div> 
</div> 
<!-- end masthead --> 

<!-- begin content -->
<!-- InstanceBeginEditable name="content_rgn" -->
<div id="content">
	<h2 id="pageName">welcome to our page</h2>
	  <div class="story">
		<h3>Test site for Acunetix WVS.</h3>
	  </div>
</div>
<!-- InstanceEndEditable -->
<!--end content -->

<div id="navBar"> 
  <div id="search"> 
    <form action="search.php?test=query" method="post"> 
      <label>search art</label> 
      <input name="searchFor" type="text" size="10"> 
      <input name="goButton" type="submit" value="go"> 
    </form> 
  </div> 
  <div id="sectionLinks"> 
    <ul> 
      <li><a href="categories.php">Browse categories</a></li> 
      <li><a href="artists.php">Browse artists</a></li> 
      <li><a href="cart.php">Your cart</a></li> 
      <li><a href="login.php">Signup</a></li>
	  <li><a href="userinfo.php">Your profile</a></li>
	  <li><a href="guestbook.php">Our guestbook</a></li>
		<li><a href="AJAX/index.php">AJAX Demo</a></li>
	  </li> 
    </ul> 
  </div> 
  <div class="relatedLinks"> 
    <h3>Links</h3> 
    <ul> 
      <li><a href="http://www.acunetix.com">Security art</a></li> 
	  <li><a href="https://www.acunetix.com/vulnerability-scanner/php-security-scanner/">PHP scanner</a></li>
	  <li><a href="https://www.acunetix.com/blog/articles/prevent-sql-injection-vulnerabilities-in-php-applications/">PHP vuln help</a></li>
	  <li><a href="http://www.eclectasy.com/Fractal-Explorer/index.html">Fractal Explorer</a></li> 
    </ul> 
  </div> 
  <div id="advert"> 
    <p>
      <object classid="clsid:D27CDB6E-AE6D-11cf-96B8-444553540000" codebase="http://download.macromedia.com/pub/shockwave/cabs/flash/swflash.cab#version=6,0,29,0" width="107" height="66">
        <param name="movie" value="Flash/add.swf">
        <param name=quality value=high>
        <embed src="Flash/add.swf" quality=high pluginspage="http://www.macromedia.com/shockwave/download/index.cgi?P1_Prod_Version=ShockwaveFlash" type="application/x-shockwave-flash" width="107" height="66"></embed>
      </object>
    </p>
  </div> 
</div> 

<!--end navbar --> 
<div id="siteInfo">  <a href="http://www.acunetix.com">About Us</a> | <a href="privacy.php">Privacy Policy</a> | <a href="mailto:wvs@acunetix.com">Contact Us</a> | <a href="/Mod_Rewrite_Shop/">Shop</a> | <a href="/hpp/">HTTP Parameter Pollution</a> | &copy;2019
  Acunetix Ltd 
</div> 

<div>
	<div>
		<textarea class="xsss_in" name="message"></textarea>
	</div>
	<div>
		<select name="id">
			<option value="">---</option>
			<option value="1">1</option>
			<option value="2">2</option>
			<option value="3">3</option>
			<option value="4">4</option>
			<option value="5">5</option>
			<option value="6">6</option>
		</select>
	</div>
</div>
    
<br> 
<div style="background-color:lightgray;width:100%;text-align:center;font-size:12px;padding:1px">
<p style="padding-left:5%;padding-right:5%"><b>Warning</b>: This is not a real shop. This is an example PHP application, which is intentionally vulnerable to web attacks. It is intended to help you test Acunetix. It also helps you understand how developer errors and bad configuration may let someone break into your website. You can use it to test other tools and your manual hacking skills as well. Tip: Look for potential SQL Injections, Cross-site Scripting (XSS), and Cross-site Request Forgery (CSRF), and more.</p>
</div>
</div>
</body>
<!-- InstanceEnd --></html>`

func TestServer(t *testing.T) {
	test := assert.New(t)
	base := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(CrawlerTestHtml))
	}))
	defer base.Close()
	time.Sleep(time.Second)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", base.URL)
		w.WriteHeader(302)
	}))
	server.StartTLS()
	defer server.Close()
	time.Sleep(time.Second)

	urlJumpResult, err := TargetUrlCheck(strings.Replace(server.URL, "http://", "", 1), nil)
	if err != nil {
		t.Error(err)
		return
	}
	test.Equal(base.URL, urlJumpResult)
}

func TestStartCrawler(t *testing.T) {
	//server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	//	_, _ = w.Write([]byte(crawlerTestHtml))
	//}))
	//log.SetLevel(log.DebugLevel)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(CrawlerTestHtml))
	}))
	server.StartTLS()
	defer server.Close()
	opts := make([]ConfigOpt, 0)
	resultSave := make([]string, 0)
	opts = append(opts,
		WithFormFill(map[string]string{"username": "admin", "password": "password"}),
		WithBlackList("logout"),
		WithMaxDepth(2),
		WithLeakless("default"),
		WithExtraWaitLoadTime(500),
		WithLocalStorage(map[string]string{"test": "abc"}),
		WithConcurrent(3),
		WithStealth(true),
		WithFullTimeout(10),
		WithScanRangeLevel(mainDomain),
		//WithPageTimeout(1),
		//WithBrowserInfo(`{"ws_address":"","exe_path":"","proxy_address":"http://127.0.0.1:8099","proxy_username":"","proxy_password":""}`),
		//WithRuntimeID("abc123-123-123"),
		//WithSaveToDB(true),
		WithEvalJs(`index.php`, `()=>document.URL`),
		WithJsResultSave(func(s string) {
			resultSave = append(resultSave, s)
		}),
		//WithAIInputUrl("http://192.168.0.150:6007/CrawlerEnhancer"),
		//WithAIInputInf("测试账户填admin，密码填password"),
	)
	//ch, err := StartCrawler("http://testphp.vulnweb.com/", opts...)
	ch, err := StartCrawler(server.URL, opts...)
	if err != nil {
		t.Error(err)
		return
	}
	var saveList []interface{}
	for item := range ch {
		saveList = append(saveList, item)
		t.Logf(`%s %s from %s`, item.Method(), item.Url(), item.From())
	}
	t.Log(`done!`)
	t.Log(resultSave)
	err = OutputData(saveList, "test.txt")
	if err != nil {
		t.Error(err)
	}
	return
}

func TestAntiCrawlerCheck(t *testing.T) {
	test := assert.New(t)
	timeStart := time.Now().UnixMilli()

	//test server start
	sannySoftHtml, err := embed.Asset("data/anti-crawler/sannysoft.html")
	if err != nil {
		t.Errorf("read test html info error: %v", err.Error())
		return
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(sannySoftHtml)
	}))
	defer server.Close()
	time.Sleep(time.Second)
	timeServerStart := time.Now().UnixMilli()
	t.Logf("Server Start: %dms", timeServerStart-timeStart-1000)

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
	timePageCreate := time.Now().UnixMilli()
	t.Logf("Page Create: %dms", timePageCreate-timeServerStart-1000)

	// stealth.min.js
	stealth, err := embed.Asset("data/anti-crawler/stealth.min.js")
	if err != nil {
		t.Errorf("read stealth.min.js error: %v", err.Error())
		return
	}
	_, err = page.EvalOnNewDocument(string(stealth))
	if err != nil {
		t.Errorf("page eval stealth.min.js error: %v", err.Error())
		return
	}
	timeScriptRun := time.Now().UnixMilli()
	t.Logf("Script Run: %dms", timeScriptRun-timePageCreate)

	// navigate
	err = page.Navigate(server.URL)
	if err != nil {
		t.Errorf("page navigate %v error: %v", server.URL, err.Error())
		return
	}
	err = page.WaitLoad()
	if err != nil {
		t.Errorf("page wait load error: %v", err.Error())
		return
	}
	timeNavigate := time.Now().UnixMilli()
	t.Logf("Url Navigate: %dms", timeNavigate-timeScriptRun)

	// html analiysis
	html, err := page.HTML()
	if err != nil {
		t.Errorf("read page html error: %v", err.Error())
		return
	}
	slices := strings.Split(html, "\n")
	var flag = false
	for _, sentence := range slices {
		if strings.Contains(sentence, "failed result") {
			if flag == false {
				flag = true
				t.Log("anti crawler check failed!")
			}
			idCompiler, _ := regexp.Compile(`id=".+?"`)
			resultCompiler, _ := regexp.Compile(`">.+?</td>`)
			id := idCompiler.FindString(sentence)
			result := resultCompiler.FindString(sentence)
			t.Logf("%v -> %v", id[4:len(id)-1], result[2:len(result)-5])
		}
	}
	if flag == false {
		t.Log("anti crawler check success!")
	}
	test.True(!flag)
}
