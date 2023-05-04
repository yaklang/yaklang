package crawler

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCrawler_Run(t *testing.T) {
	test := assert.New(t)
	crawler, err := NewCrawler("http://159.65.125.15/theme/revolution/js")
	if err != nil {
		panic(err)
		return
	}

	err = crawler.Run()
	if err != nil {
		test.FailNow(err.Error())
		return
	}
}

func TestHandleRequestResult(t *testing.T) {
	req, err := HandleRequestResult(true, []byte(`GET /tools/test/ HTTP/1.1
Host: calmops.com
`), []byte(`HTTP/1.1 200 OK
Connection: close
Accept-Ranges: bytes
Content-Type: text/html; charset=utf-8
Date: Tue, 20 Sep 2022 02:40:10 GMT
Etag: "ri8hst6dg"
Last-Modified: Thu, 15 Sep 2022 04:29:17 GMT
Server: Caddy
Content-Length: 8259

<!DOCTYPE html>
<html lang="en" dir="ltr">

<head>
  <meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
<meta http-equiv="X-UA-Compatible" content="IE=edge">
<meta name="theme-color" content="#389E98">
<meta name="sogou_site_verification" content="4csCltRref" />
<meta name="yandex-verification" content="ef01d9879c2e5054" />
<meta name="description" content="爬虫测试页面。爬取图片，pdf文件，日期，标题，作者" />
<meta name="keywords" content="[测试]" />
<link rel="stylesheet" href="/css/bootstrap.min.css" defer>
<link rel="icon" type="image/png" href="/icons/favicon-32x32.png" sizes="32x32" />
<link rel="icon" type="image/png" href="/icons/favicon-16x16.png" sizes="16x16" />
<script type="text/javascript" src="/build-js/vendors~app.bundle.js" async></script>
<script type="text/javascript" src="/build-js/app.bundle.js" async></script>
<script type="text/javascript" src="/js/reg-sw.js" async></script>


<script async src="https://www.googletagmanager.com/gtag/js?id=G-M9LTL9V8LV"></script>
<script>
        window.dataLayer = window.dataLayer || [];
        function gtag() { dataLayer.push(arguments); }
        gtag('js', new Date());

        gtag('config', 'G-M9LTL9V8LV');
</script>

<script async src="https://pagead2.googlesyndication.com/pagead/js/adsbygoogle.js?client=ca-pub-7432056225229759"
        crossorigin="anonymous"></script>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/katex@0.11.0/dist/katex.min.css"
        integrity="sha384-BdGj8xC2eZkQaxoQ8nSLefg4AV4/AwB3Fj+8SUSo7pnKP6Eoy18liIKTPn9oBYNG" crossorigin="anonymous">


<script defer src="https://cdn.jsdelivr.net/npm/katex@0.11.0/dist/katex.min.js"
        integrity="sha384-JiKN5O8x9Hhs/UE5cT5AAJqieYlOZbGT3CHws/y97o3ty4R7/O5poG9F3JoiOYw1" crossorigin="anonymous">
        </script>


<script defer src="https://cdn.jsdelivr.net/npm/katex@0.11.0/dist/contrib/auto-render.min.js"
        integrity="sha384-kWPLUVMOks5AQFrykwIup5lo0m3iMkkHrD0uJ4H5cjeGihAutqP0yW0J6dpFiVkI" crossorigin="anonymous"
        onload="renderMathInElement(document.body);"></script>
<link href="https://cdn.jsdelivr.net/npm/katex@0.11.0/dist/contrib/copy-tex.css" rel="stylesheet" type="text/css">
<script src="https://cdn.jsdelivr.net/npm/katex@0.11.0/dist/contrib/copy-tex.min.js"
        integrity="sha384-XhWAe6BtVcvEdS3FFKT7Mcft4HJjPqMQvi5V4YhzH9Qxw497jC13TupOEvjoIPy7" crossorigin="anonymous">
        </script>
<script>
        document.addEventListener("DOMContentLoaded", function () {
                renderMathInElement(document.body, {
                        delimiters: [{
                                left: "$$",
                                right: "$$",
                                display: true
                        },
                        {
                                left: "$",
                                right: "$",
                                display: false
                        }
                        ]
                });
        });
</script>

<title>爬虫测试页面 - Calmops </title>
<meta name="author" content="屈永强" />
<meta property="og:title" content="爬虫测试页面" />
<meta property="og:description" content="爬虫测试页面。爬取图片，pdf文件，日期，标题，作者" />
<meta property="og:type" content="article" />
<meta property="og:url" content="https://calmops.com/tools/test/" /><meta property="article:section" content="tools" />
<meta property="article:published_time" content="2021-08-11T00:00:00+00:00" />
<meta property="article:modified_time" content="2021-08-11T00:00:00+00:00" /><meta property="og:site_name" content="Calmops" />



<script type="application/javascript">
var doNotTrack = false;
if (!doNotTrack) {
        window.ga=window.ga||function(){(ga.q=ga.q||[]).push(arguments)};ga.l=+new Date;
        ga('create', 'UA-128653988-1', 'auto');

        ga('send', 'pageview');
}
</script>
<script async src='https://www.google-analytics.com/analytics.js'></script>

  <link rel="stylesheet" href="/css/text.css">
  <link rel="stylesheet" href="/css/main.css">
  <link rel="stylesheet" href="/css/header.css">
  <link rel="stylesheet" href="/css/list.css">
  <link rel="stylesheet" href="/css/single.css">
  <link rel="stylesheet" href="/css/categories.css">
  <link rel="manifest" href="/manifest.json">
</head>

<body>
  
  <header>
  <nav class="navbar navbar-expand-md navbar-light">
    <a class="navbar-brand" href="/">Calmops</a>
    <button class="navbar-toggler" type="button" data-toggle="collapse" data-target="#navbarCollapse"
      aria-controls="navbarCollapse" aria-expanded="false" aria-label="Toggle navigation">
      <span class="navbar-toggler-icon"></span>
    </button>
    <div class="collapse navbar-collapse" id="navbarCollapse">
      <ul class="navbar-nav mr-auto">
        <li class="nav-item" id="categories-link">
          <a class="nav-link" href="/categories/">Categories <span class="sr-only">(current)</span></a>
        </li>
        <li class="nav-item" id="tags-link">
          <a class="nav-link" href="/tags/">Tags</a>
        </li>
        <li class="nav-item" id="tools-link">
          <a class="nav-link" href="/categories/tools/">Tools</a>
        </li>
        <li class="nav-item" id="bycoffee-link">
          <a class="nav-link" target="_blank" href="https://www.buymeacoffee.com/netqyq">By Me a Coffee</a>
        </li>
      </ul>
      <form class="form-inline mt-2 mt-md-0">
        <input class="form-control mr-sm-2" type="text" placeholder="Search" aria-label="Search" id="search-input">
        <button class="btn btn-outline-secondary my-2 my-sm-0" type="submit" id="search-btn">Search</button>
      </form>
    </div>
  </nav>
</header>
  
  <div class="container">
    <aside class="left-aside">
      
      <ins class="adsbygoogle" style="display:block" data-ad-client="ca-pub-7432056225229759" data-ad-slot="3060291304"
        data-ad-format="auto" data-full-width-responsive="true"></ins>
      <script>
        (adsbygoogle = window.adsbygoogle || []).push({});
      </script>
    </aside>
    <div class="main" role="main">
      
<article class="">
  <h1 class="article-title">爬虫测试页面</h1>
  <p class="article-subtitle"></p>
  <div class="article-date">
    2021-08-11
  </div>
  
  <div class="article-author">
    Yongqiang
  </div>
  
  <div class="share">
    <div id="qrcode" style="width:100px; height:100px;"></div>
    <div class="qrcode-desc"><span>扫一扫获取本文链接，可在手机中查看或分享。</span></div>
  </div>
  <div class="TOC">
    <nav id="TableOfContents"></nav>
  </div>
  <div class="article-content">
    <p>来源：深圳特区报</p>
<p>作者：深圳特区报</p>
<p>出处：深圳特区报</p>
<p>来自：深圳特区报</p>
<!-- raw HTML omitted -->
<p>时间：2021-06-21 12:10</p>
<p>日期：2021-06-21</p>
<!-- raw HTML omitted -->
<p>日期：2021年06月21日</p>
<p>爬虫测试页面。爬取图片，pdf文件，日期，标题，作者</p>
<p><img src="/images/table-phone.png" alt="table-phone"></p>
<p><img src="/images/mongodb/methodology.png" alt="table-phone"></p>
<!-- raw HTML omitted -->
<p><a href="/data/recommendation-algorithms.docx">recommendation algorithms</a></p>
<p><a href="/data/cs-6476_syllabus.pdf">cs-6476_syllabus.pdf</a></p>
<p><a href="/data/Notation.pdf">Notation.pdf</a></p>
<p><a href="http://www.calmops.com/">link with slash</a></p>
<p><a href="http://www.calmops.com">link w/o slash</a></p>

    
    <ins class="adsbygoogle" style="display:block; text-align:center;" data-ad-layout="in-article"
      data-ad-format="fluid" data-ad-client="ca-pub-7432056225229759" data-ad-slot="3096321454"></ins>
    <script>
      (adsbygoogle = window.adsbygoogle || []).push({});
    </script>
  </div>
</article>
<aside class="single-aside">
  

</aside>


    </div>
    <aside class="right-aside">
      
      <ins class="adsbygoogle" style="display:block" data-ad-format="autorelaxed"
        data-ad-client="ca-pub-7432056225229759" data-ad-slot="6439211840"></ins>
      <script>
        (adsbygoogle = window.adsbygoogle || []).push({});
      </script>
    </aside>
  </div>
  <footer class="footer">
      <div class="footer-links">
            <div>©2022 Yongqiang Qu   me@yongqiang.live &nbsp; All Rights Reserved</div>
      </div>
</footer>

  
  

<script src="/js/jquery-3.3.1.slim.min.js"></script>
<script src="/js/popper.min.js"></script>
<script src="/js/bootstrap.min.js"></script>
<script src="/js/lunr.min.js"></script>
<script src="/js/dexie.min.js"></script>
<script src="/js/qrcode.min.js"></script>
</body>

</html
`))
	if err != nil {
		panic(err)
	}
	spew.Dump(req)
}

func TestHandleRequestResult2_MetaTag(t *testing.T) {
	req, err := HandleRequestResult(true, []byte(`GET /tools/test/ HTTP/1.1
Host: calmops.com
`), []byte(`HTTP/1.1 200 
Connection: close
Accept-Ranges: bytes
Content-Type: text/html
Date: Fri, 24 Feb 2023 05:29:40 GMT
Etag: W/"202-1254499436000"
Last-Modified: Fri, 02 Oct 2009 16:03:56 GMT
Content-Length: 202

<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.0 Transitional//EN">
<html>
<head>
    <META HTTP-EQUIV="Refresh" CONTENT="0;URL=example/HelloWorld.action">
</head>

<body>
<p>Loading ...</p>
</body>
</html>
`))
	if err != nil {
		panic(err)
	}
	spew.Dump(req)
}
