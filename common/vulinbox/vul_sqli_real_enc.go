package vulinbox

import (
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

//go:embed vul_sqli_real_enc/login.html
var sqliEncLogin string

//go:embed vul_sqli_real_enc/users.html
var sqliEncUsers string

func (s *VulinServer) getEncryptSQLinj() []*VulInfo {

	var isLoginedFromRawViaDatabase2 = func(i any) (bool, string) {
		var params = make(map[string]any)
		switch i.(type) {
		case string:
			params = utils.ParseStringToGeneralMap(i)
		case []byte:
			params = utils.ParseStringToGeneralMap(i)
		default:
			params = utils.InterfaceToGeneralMap(i)
		}
		username := utils.MapGetString(params, "username")
		password := utils.MapGetString(params, "password")
		log.Info("username: ", username, " password: ", password)
		users, err := s.database.UnsafeSqlQuery(`select * from vulin_users where username = '` + username + "' and password = '" + password + "';")
		if err != nil {
			return false, utils.Wrapf(err, "get user by username failed: %v", username).Error()
		}
		if len(users) > 0 {
			return true, "success! your password is correct! inject success!"
		}
		return false, "failed! your password is incorrect! inject failed!"
	}

	authStorage := make(map[string]string)
	m := new(sync.Mutex)
	setToken := func(token string) {
		m.Lock()
		defer m.Unlock()
		authStorage[token] = ""
	}
	removeToken := func(token string) {
		m.Lock()
		defer m.Unlock()
		delete(authStorage, token)
	}
	haveToken := func(token string) bool {
		m.Lock()
		defer m.Unlock()
		_, exists := authStorage[token]
		return exists
	}

	vroutes := []*VulInfo{
		{
			Path: "/sqli/aes-ecb/encrypt/logout",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				token, err := request.Cookie("token")
				if err == nil && token != nil {
					removeToken(token.Value)
				}
				http.SetCookie(writer, &http.Cookie{
					Name:    "token",
					Value:   "",
					Expires: time.Unix(0, 0),
				})
			},
		},
		{
			Path: "/sqli/aes-ecb/encrypt/query/users",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				reqBytes, _ := utils.DumpHTTPRequest(request, true)
				body := lowhttp.GetHTTPPacketBody(reqBytes)

				// 解析请求体
				var encryptedReq struct {
					Key     string `json:"key"`
					IV      string `json:"iv"`
					Message string `json:"message"`
				}
				json.Unmarshal(body, &encryptedReq)

				// 解密请求数据
				// 创建错误响应函数
				createErrorResponse := func(errMsg string) {
					// 生成随机密钥和IV
					errKeyBytes := make([]byte, 16)
					errIvBytes := make([]byte, 16)
					rand.Read(errKeyBytes)
					rand.Read(errIvBytes)

					// 加密错误信息
					errResp := map[string]interface{}{
						"error": errMsg,
					}
					errJson, _ := json.Marshal(errResp)
					encryptedErr, _ := codec.AESCBCEncrypt(errKeyBytes, errJson, errIvBytes)

					// 返回加密的错误信息
					resp := map[string]string{
						"key":     hex.EncodeToString(errKeyBytes),
						"iv":      hex.EncodeToString(errIvBytes),
						"message": base64.StdEncoding.EncodeToString(encryptedErr),
					}
					respBytes, _ := json.Marshal(resp)
					writer.Header().Set("Content-Type", "application/json")
					writer.Write(respBytes)
				}

				// 解密请求数据
				keyBytes, err := hex.DecodeString(encryptedReq.Key)
				if err != nil {
					createErrorResponse("invalid key format")
					return
				}

				ivBytes, err := hex.DecodeString(encryptedReq.IV)
				if err != nil {
					createErrorResponse("invalid iv format")
					return
				}

				encryptedBytes, err := base64.StdEncoding.DecodeString(encryptedReq.Message)
				if err != nil {
					createErrorResponse("invalid message format")
					return
				}

				decryptedBytes, err := codec.AESCBCDecrypt(keyBytes, encryptedBytes, ivBytes)
				if err != nil {
					createErrorResponse("decryption failed")
					return
				}

				// 解析解密后的数据
				var reqData struct {
					Search string `json:"search"`
				}
				if err := json.Unmarshal(decryptedBytes, &reqData); err != nil {
					createErrorResponse("解析请求数据失败")
					return
				}

				// 查询数据库
				users, err := s.database.UnsafeSqlQuery(`select id, username, age from vulin_users where username != 'admin' and username != 'root' and username like '%` + reqData.Search + `%' `)
				if err != nil {
					createErrorResponse("查询数据库失败: " + err.Error())
					return
				}

				// 生成新的key和iv用于返回
				newKeyBytes := make([]byte, 16)
				newIvBytes := make([]byte, 16)
				if _, err := rand.Read(newKeyBytes); err != nil {
					createErrorResponse("生成密钥失败")
					return
				}
				if _, err := rand.Read(newIvBytes); err != nil {
					createErrorResponse("生成IV失败")
					return
				}

				// 加密返回数据
				respData := map[string]interface{}{
					"users": users,
				}
				respJson, err := json.Marshal(respData)
				if err != nil {
					createErrorResponse("序列化响应数据失败")
					return
				}

				encryptedResp, err := codec.AESCBCEncrypt(newKeyBytes, respJson, newIvBytes)
				if err != nil {
					createErrorResponse("加密响应数据失败")
					return
				}

				// 构造返回结果
				resp := map[string]string{
					"key":     hex.EncodeToString(newKeyBytes),
					"iv":      hex.EncodeToString(newIvBytes),
					"message": base64.StdEncoding.EncodeToString(encryptedResp),
				}

				// 返回JSON
				respBytes, err := json.Marshal(resp)
				if err != nil {
					createErrorResponse("序列化最终响应失败")
					return
				}

				writer.Header().Set("Content-Type", "application/json")
				writer.Write(respBytes)
			},
		},
		{
			Path: "/sqli/aes-ecb/encrypt/users",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				switch request.Method {
				case http.MethodGet:
					// 检查cookie中的token
					token, err := request.Cookie("token")
					if err != nil || token == nil || !haveToken(token.Value) {
						http.Redirect(writer, request, "./login", http.StatusFound)
						return
					}
					writer.Write([]byte(sqliEncUsers))
					return
				default:
					return
				}
			},
		},
		{
			Path:  "/sqli/aes-ecb/encrypt/login",
			Title: "SQL 注入（从登陆到 Dump 数据库）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				switch request.Method {
				case http.MethodGet:
					// 检查cookie中的token
					token, err := request.Cookie("token")
					if err == nil && token != nil && haveToken(token.Value) {
						http.Redirect(writer, request, "./users", http.StatusFound)
						return
					}
					// show login page
					writer.Write([]byte(sqliEncLogin))
					return
				case http.MethodPost:
					reqBytes, _ := utils.DumpHTTPRequest(request, true)
					body := lowhttp.GetHTTPPacketBody(reqBytes)
					i := make(map[string]any)
					fmt.Println(string(body))

					// 生成新的 key 和 iv 用于返回加密数据
					newKeyBytes := make([]byte, 16)
					newIvBytes := make([]byte, 16)
					rand.Read(newKeyBytes)
					rand.Read(newIvBytes)

					// 构造加密返回函数
					encryptResponse := func(data map[string]any) []byte {
						results, _ := json.Marshal(data)
						encryptedBytes, _ := codec.AESCBCEncrypt(newKeyBytes, results, newIvBytes)
						response := map[string]string{
							"key":     codec.EncodeToHex(newKeyBytes),
							"iv":      codec.EncodeToHex(newIvBytes),
							"message": codec.EncodeBase64(encryptedBytes),
						}
						return []byte(utils.Jsonify(response))
					}

					// 解析请求JSON
					if err := json.Unmarshal([]byte(body), &i); err != nil {
						writer.Write(encryptResponse(map[string]any{
							"error": "无效的JSON格式",
						}))
						return
					}

					// 获取并验证参数
					key, ok := i["key"].(string)
					if !ok {
						writer.Write(encryptResponse(map[string]any{
							"error": "key参数无效",
						}))
						return
					}

					iv, ok := i["iv"].(string)
					if !ok {
						writer.Write(encryptResponse(map[string]any{
							"error": "iv参数无效",
						}))
						return
					}

					message, ok := i["message"].(string)
					if !ok {
						writer.Write(encryptResponse(map[string]any{
							"error": "message参数无效",
						}))
						return
					}

					// 解码参数
					keyBytes, err := codec.DecodeHex(key)
					if err != nil {
						writer.Write(encryptResponse(map[string]any{
							"error": "key解码失败",
						}))
						return
					}

					ivBytes, err := codec.DecodeHex(iv)
					if err != nil {
						writer.Write(encryptResponse(map[string]any{
							"error": "iv解码失败",
						}))
						return
					}

					messageBytes, err := codec.DecodeBase64(message)
					if err != nil {
						writer.Write(encryptResponse(map[string]any{
							"error": "message解码失败",
						}))
						return
					}

					// 解密数据
					decrypted, err := codec.AESCBCDecrypt(keyBytes, messageBytes, ivBytes)
					if err != nil {
						writer.Write(encryptResponse(map[string]any{
							"error": "解密失败",
						}))
						return
					}
					ok, reason := isLoginedFromRawViaDatabase2(string(decrypted))
					if !ok {
						writer.Write(encryptResponse(map[string]any{
							"error": "认证失败：" + reason,
						}))
						return
					}

					// 认证成功
					// 生成随机token
					token := utils.RandStringBytes(32)

					// 设置session
					setToken(token)
					go func() {
						time.Sleep(time.Minute * 30)
						removeToken(token)
					}()

					// 返回成功结果
					// 设置 Cookie
					http.SetCookie(writer, &http.Cookie{
						Name:     "token",
						Value:    token,
						Path:     "/",
						Expires:  time.Now().Add(30 * time.Minute),
						HttpOnly: true,
					})
					writer.Header().Set("Content-Type", "application/json")
					writer.Write(encryptResponse(map[string]any{
						"echo":     string(decrypted),
						"verified": true,
						"token":    token,
						"error":    "",
					}))
					return
				default:
					return
				}
			},
		},
		{
			Path:  "/challenge-api-docs",
			Title: "动态挑战响应API靶场（20250623）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "text/html; charset=utf-8")
				html := `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <title>动态挑战-响应 API 安全靶场</title>
    <style>
        body { font-family: sans-serif; line-height: 1.6; padding: 20px; max-width: 800px; margin: auto; }
        h1, h2 { color: #333; }
        code { background-color: #f4f4f4; padding: 2px 6px; border-radius: 4px; }
        .workflow { text-align: center; margin: 20px 0; }
        .key { color: #c7254e; background-color: #f9f2f4; padding: 2px 4px; border-radius: 4px; }
    </style>
</head>
<body>
    <h1>动态挑战-响应 API 安全靶场</h1>
    <p>这是一个模拟真实世界高安全性API的靶场。它使用动态挑战-响应机制来防止重放攻击，并对业务数据进行加密传输。常规的Fuzzer很难成功请求此类型API。</p>
    
    <h2>交互流程</h2>
    <div class="workflow">
        <script src="https://cdn.jsdelivr.net/npm/mermaid/dist/mermaid.min.js"></script>
        <script>mermaid.initialize({startOnLoad:true});</script>
        <div class="mermaid">
        sequenceDiagram
            participant Client as 客户端
            participant Server as 服务器
            Client->>Server: 1. GET /api/get-challenge
            Server-->>Client: 返回 {"challenge": "...", "iv": "..."}
            Client->>Client: 2. 解密 challenge 获取 nonce<br/>3. 使用 HMAC-SHA256(nonce) 计算签名
            Client->>Server: 4. GET /api/user/info<br/>Header: X-Auth-Signature: &lt;signature&gt;
            Server->>Server: 5. 验证签名，若通过则加密业务数据
            alt 签名有效
                Server-->>Client: 200 OK {"data": "...", "iv": "..."}
            else 签名/挑战无效
                Server-->>Client: 401 / 403 错误
            end
            Client->>Client: 6. 解密 data 获取最终信息
        </div>
    </div>

    <h2>任务步骤</h2>
    <ol>
        <li>向 <code><a href="/api/get-challenge" target="_blank">/api/get-challenge</a></code> 发起GET请求，获取加密后的挑战 <code>challenge</code> 和初始化向量 <code>iv</code>。</li>
        <li>使用预共享的AES密钥和获取到的 <code>iv</code> 解密 <code>challenge</code>，得到原始的 <code>nonce</code>。
            <ul><li>AES密钥: <code class="key">YakitVulinboxAES</code></li></ul>
        </li>
        <li>使用预共享的HMAC密钥，通过HMAC-SHA256算法计算 <code>nonce</code> 的签名。
            <ul><li>HMAC密钥: <code class="key">YakitVulinboxHMACKey-SIGNATURE</code></li></ul>
        </li>
        <li>将计算出的签名（Hex格式）放入 <code>X-Auth-Signature</code> 请求头，向 <code>/api/user/info</code> 发起GET请求。</li>
        <li>如果签名正确，你将收到加密的业务数据。再次使用AES密钥和响应中的新 <code>iv</code> 解密，即可看到最终的敏感信息。</li>
    </ol>
</body>
</html>
`
				writer.Write([]byte(html))
			},
		},
	}
	return vroutes
}
