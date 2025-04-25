package vulinbox

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"

	utils2 "github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

//go:embed html/vul_fastjson.html
var fastjson_loginPage []byte

type JsonParser func(data string) (map[string]any, error)

const magicIntranetHost = "qqqqqqqqqq.qqqqqqqqqq.qqqqqqqqqq.qqqqqqqqqq"

var networkUnstableTimes = 0

func networkUnstableTimesNext() int {
	networkUnstableTimes = (networkUnstableTimes + 1) % 3
	return networkUnstableTimes
}

func generateFastjsonParser(version string) JsonParser {
	if version == "intranet" { // 内网版本，不能成功解析 payload 中的 dnslog 且解析超时
		return func(data string) (map[string]any, error) {
			return fastjsonParser(data, "", magicIntranetHost)
		}
	}
	if version == "network-unstable" { // 网络不稳定版本，不能成功解析 payload 中的 dnslog 且解析超时
		return func(data string) (map[string]any, error) {
			return fastjsonParser(data, "network-unstable", magicIntranetHost)
		}
	}
	return func(data string) (map[string]any, error) {
		return fastjsonParser(data, "")
	}
}

var HandleDnsRequest func(domain string)

// 这里模拟fastjson的解析过程
func fastjsonParser(data string, flag string, forceDnslog ...string) (map[string]any, error) {
	networkUnstableFlag := flag == "network-unstable"
	_ = networkUnstableFlag
	// redos
	if strings.Contains(data, "regex") {
		time.Sleep(2 * time.Second)
	}
	var domain string
	// 查找dnslog
	re, err := regexp.Compile(`(\w+\.)+((dnslog\.cn)|(ceye\.io)|(vcap\.me)|(vcap\.io)|(xip\.io)|(burpcollaborator\.net)|(dgrh3\.cn)|(\w+\.eu\.org)|(gobygo\.net))|(127\.0\.0\.1:\d+)`)
	if err != nil {
		return nil, err
	}
	res := re.FindAllStringSubmatch(data, -1)
	if len(res) > 0 {
		domain = res[0][0]
		if len(forceDnslog) > 0 {
			domain = forceDnslog[0]
		}
	}
	if domain != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		go func() {
			defer cancel()
			if networkUnstableFlag {
				if networkUnstableTimesNext() == 2 {
					return
				}
			}
			if domain != magicIntranetHost {
				log.Infof("dnslog action to: %s", domain)
				if HandleDnsRequest != nil {
					HandleDnsRequest(domain)
				} else {
					netx.LookupFirst(domain, netx.WithDNSContext(ctx), netx.WithDNSNoCache(false))
				}
			} else {
				time.Sleep(2 * time.Second)
			}
		}()
		select {
		case <-ctx.Done():
		}
	}
	var js map[string]any
	err = json.Unmarshal([]byte(data), &js)
	if err != nil {
		return nil, err
	}

	return js, nil
}

func mockController(parser JsonParser, req *http.Request, data string) string {
	newErrorResponse := func(err error) string {
		response, _ := json.Marshal(map[string]any{
			"code": -1,
			"err":  err.Error(),
		})
		return string(response)
	}
	results := codec.AutoDecode(data)
	if len(results) == 0 {
		return newErrorResponse(errors.New("decode error"))
	}
	data = results[len(results)-1].Result
	_, err := parser(data)
	if err != nil {
		return newErrorResponse(err)
	}

	err = errors.New("user or password error")
	return newErrorResponse(err)
}

func jacksonParser(data string) (map[string]any, error) {
	var js map[string]any
	err := json.Unmarshal([]byte(data), &js)
	if err != nil {
		return nil, err
	}
	return js, nil
}

func mockJacksonController(req *http.Request, data string) string {
	newErrorResponse := func(err error) string {
		response, _ := json.Marshal(map[string]any{
			"code": -1,
			"err":  err.Error(),
		})
		return string(response)
	}
	results := codec.AutoDecode(data)
	if len(results) == 0 {
		return newErrorResponse(errors.New("decode error"))
	}
	data = results[len(results)-1].Result
	js, _ := jacksonParser(data)
	unrecognizedFields := map[string]struct{}{}
	allowFields := map[string]struct{}{}
	if req.URL.Path == "/fastjson/json-in-cookie" {
		for k := range js {
			if k != "id" {
				unrecognizedFields[k] = struct{}{}
				allowFields["id"] = struct{}{}
			}
		}
	} else {
		for k := range js {
			if k != "user" && k != "password" {
				unrecognizedFields[k] = struct{}{}
				allowFields["user"] = struct{}{}
				allowFields["password"] = struct{}{}
			}
		}
	}

	// 模拟Jackson的报错
	if len(js) > 2 {
		unrecognizedStr := ""
		allowFieldsStr := ""
		for k := range unrecognizedFields {
			unrecognizedStr += k + ","
		}
		for k := range allowFields {
			allowFieldsStr += k + ","
		}
		response, _ := json.Marshal(map[string]any{
			"timestamp": time.Now().Format("2006-01-02T15:04:05.000+0000"),
			"status":    500,
			"error":     "Internal Server Error",
			"message":   fmt.Sprintf("Unrecognized field %s (class com.vulbox.User), not marked as ignorable (2 known properties: %s])\n at [Source: (String)%s] (through reference chain: com.vulbox.User[%s])", unrecognizedStr, allowFieldsStr, data, unrecognizedStr),
			"path":      req.URL.Path,
		})
		return string(response)
	}
	return "ok"
}

func (s *VulinServer) registerFastjson() {
	r := s.router
	globalIdForUnstableNetwork := 0
	fastjsonGroup := r.PathPrefix("/fastjson").Name("Fastjson 案例").Subrouter()
	vuls := []*VulInfo{
		{
			Title:        "GET 传参案例案例",
			Path:         "/json-in-query",
			DefaultQuery: `auth={"user":"admin","password":"password"}`,
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					action := request.URL.Query().Get("action")
					if action == "" {
						writer.Write([]byte(utils2.Format(string(fastjson_loginPage), map[string]string{
							"script": `function load(){
            name=$("#username").val();
            password=$("#password").val();
            auth = {"user":name,"password":password};
            $.ajax({
                type:"get",
                url:"/fastjson/json-in-query",
                data:{"auth":JSON.stringify(auth),"action":"login"},
                success: function (data ,textStatus, jqXHR)
                {
                    $("#response").text(JSON.stringify(data));
					console.log(data);
                },
                error:function (XMLHttpRequest, textStatus, errorThrown) {      
                    alert("请求出错");
                },
            })
        }`,
						})))
						return
					}
					auth := request.URL.Query().Get("auth")
					if auth == "" {
						writer.Write([]byte("auth 参数不能为空"))
						return
					}
					response := mockController(generateFastjsonParser("1.2.43"), request, auth)
					writer.Write([]byte(response))
				} else {
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			},
		},
		{
			Title: "POST Form传参案例案例",
			Path:  "/json-in-form",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodPost {
					body := request.FormValue("auth")
					response := mockController(generateFastjsonParser("1.2.43"), request, body)
					writer.Write([]byte(response))
				} else {
					writer.Write([]byte(utils2.Format(string(fastjson_loginPage), map[string]string{
						"script": `function load(){
            name=$("#username").val();
            password=$("#password").val();
            auth = {"user":name,"password":password};
            $.ajax({
                type:"post",
                url:"/fastjson/json-in-form",
                data:{"auth":JSON.stringify(auth),"action":"login"},
				dataType: "json",
                success: function (data ,textStatus, jqXHR)
                {
                    $("#response").text(JSON.stringify(data));
					console.log(data);
                },
                error:function (XMLHttpRequest, textStatus, errorThrown) {      
                    alert("请求出错");
                },
            })
        }`,
					})))
					return
				}
			},
		},
		{
			Title: "POST Body传参案例案例",
			Path:  "/json-in-body",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodPost {
					body, err := io.ReadAll(request.Body)
					if err != nil {
						writer.WriteHeader(http.StatusBadRequest)
						writer.Write([]byte("Invalid request"))
						return
					}
					defer request.Body.Close()
					response := mockController(generateFastjsonParser("1.2.43"), request, string(body))
					writer.Write([]byte(response))
				} else {
					writer.Write([]byte(utils2.Format(string(fastjson_loginPage), map[string]string{
						"script": `function load(){
            name=$("#username").val();
            password=$("#password").val();
            auth = {"user":name,"password":password};
            $.ajax({
                type:"post",
                url:"/fastjson/json-in-body",
                data:JSON.stringify(auth),
				dataType: "json",
				contentType: "application/json",
                success: function (data ,textStatus, jqXHR)
                {
                    $("#response").text(JSON.stringify(data));
					console.log(data);
                },
                error:function (XMLHttpRequest, textStatus, errorThrown) {      
                    alert("请求出错");
                },
            })
        }`,
					})))
					return
				}
			},
		},
		{
			Title: "Cookie 传参案例案例",
			Path:  "/json-in-cookie",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					action := request.URL.Query().Get("action")
					if action == "" {
						writer.Header().Set("Set-Cookie", `auth=`+codec.EncodeBase64Url(`{"id":"-1"}`)) // Fuzz Coookie暂时没有做只能解码，不能编码
						writer.Write([]byte(utils2.Format(string(fastjson_loginPage), map[string]string{
							"script": `function load(){
            name=$("#username").val();
            password=$("#password").val();
            auth = {"user":name,"password":password};
            $.ajax({
                type:"get",
                url:"/fastjson/json-in-cookie",
                data:{"auth":JSON.stringify(auth),"action":"login"},
                success: function (data ,textStatus, jqXHR)
                {
                    $("#response").text(JSON.stringify(data));
					console.log(data);
                },
                error:function (XMLHttpRequest, textStatus, errorThrown) {      
                    alert("请求出错");
                },
            })
        }`,
						})))
						return
					}
					cookie, err := request.Cookie("auth")
					if err != nil {
						writer.Write([]byte("auth 参数不能为空"))
						return
					}
					response := mockController(generateFastjsonParser("1.2.43"), request, cookie.Value)
					writer.Write([]byte(response))
				} else {
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			},
		},
		{
			Title: "Authorization 传参案例案例",
			Path:  "/json-in-authorization",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					action := request.URL.Query().Get("action")
					if action == "" {
						writer.Write([]byte(utils2.Format(string(fastjson_loginPage), map[string]string{
							"script": `function load(){
            name=$("#username").val();
            password=$("#password").val();
            auth = {"user":name,"password":password};
			authHeaderValue = btoa(JSON.stringify(auth));
            $.ajax({
                type:"get",
                url:"/fastjson/json-in-authorization?action=login",
                headers: {"Authorization": "Basic "+authHeaderValue},
				dataType: "json",
                success: function (data ,textStatus, jqXHR)
                {
                    $("#response").text(JSON.stringify(data));
					console.log(data);
                },
                error:function (XMLHttpRequest, textStatus, errorThrown) {      
                    alert("请求出错");
                },
            })
        }`,
						})))
						return
					}
					auth := request.Header.Get("Authorization")
					if len(auth) < 6 {
						writer.Write([]byte("auth 参数不能为空"))
						return
					}
					response := mockController(generateFastjsonParser("1.2.43"), request, auth[6:])
					writer.Write([]byte(response))
				} else {
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			},
		},
		{
			Title:        "GET 传参Jackson后端案例",
			Path:         "/jackson-in-query",
			DefaultQuery: `auth={"user":"admin","password":"password"}`,
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					action := request.URL.Query().Get("action")
					if action == "" {
						writer.Write([]byte(utils2.Format(string(fastjson_loginPage), map[string]string{
							"script": `function load(){
            name=$("#username").val();
            password=$("#password").val();
            auth = {"user":name,"password":password};
            $.ajax({
                type:"get",
                url:"/fastjson/jackson-in-query",
                data:{"auth":JSON.stringify(auth),"action":"login"},
                success: function (data ,textStatus, jqXHR)
                {
                    $("#response").text(JSON.stringify(data));
					console.log(data);
                },
                error:function (XMLHttpRequest, textStatus, errorThrown) {      
                    alert("请求出错");
                },
            })
        }`,
						})))
						return
					}
					auth := request.URL.Query().Get("auth")
					if auth == "" {
						writer.Write([]byte("auth 参数不能为空"))
						return
					}
					response := mockJacksonController(request, auth)
					writer.Write([]byte(response))
				} else {
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			},
		},
		{
			Title: "网络不稳定的靶站",
			Path:  "/unstable-network",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodPost {
					if globalIdForUnstableNetwork != 2 {
						time.Sleep(3 * time.Second)
					}
					writer.Write([]byte("HTTP/1.1 200 OK"))
					globalIdForUnstableNetwork = (globalIdForUnstableNetwork + 1) % 3
				} else {
					writer.Write([]byte(utils2.Format(string(fastjson_loginPage), map[string]string{
						"script": `function load(){
            name=$("#username").val();
            password=$("#password").val();
            auth = {"user":name,"password":password};
            $.ajax({
                type:"post",
                url:"/fastjson/unstable-network",
                data:{"auth":JSON.stringify(auth),"action":"login"},
				dataType: "json",
                success: function (data ,textStatus, jqXHR)
                {
                    $("#response").text(JSON.stringify(data));
					console.log(data);
                },
                error:function (XMLHttpRequest, textStatus, errorThrown) {      
                    alert("请求出错");
                },
            })
        }`,
					})))
					return
				}
			},
		},
		{
			Title: "GET 传参且网络不稳定（无漏洞）",
			Path:  "/get-in-query-network-unstable",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					action := request.URL.Query().Get("action")
					if action == "" {
						writer.Write([]byte(utils2.Format(string(fastjson_loginPage), map[string]string{
							"script": `function load(){
            name=$("#username").val();
            password=$("#password").val();
            auth = {"user":name,"password":password};
            $.ajax({
                type:"get",
                url:"/fastjson/get-in-query-network-unstable",
                data:{"auth":JSON.stringify(auth),"action":"login"},
                success: function (data ,textStatus, jqXHR)
                {
                    $("#response").text(JSON.stringify(data));
					console.log(data);
                },
                error:function (XMLHttpRequest, textStatus, errorThrown) {      
                    alert("请求出错");
                },
            })
        }`,
						})))
						return
					}
					auth := request.URL.Query().Get("auth")
					if auth == "" {
						writer.Write([]byte("auth 参数不能为空"))
						return
					}
					response := mockController(generateFastjsonParser("network-unstable"), request, auth)
					writer.Write([]byte(response))
				} else {
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			},
		},
		{
			Title: "GET 传参且应用部署在内网的案例",
			Path:  "/get-in-query-intranet",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					action := request.URL.Query().Get("action")
					if action == "" {
						writer.Write([]byte(utils2.Format(string(fastjson_loginPage), map[string]string{
							"script": `function load(){
            name=$("#username").val();
            password=$("#password").val();
            auth = {"user":name,"password":password};
            $.ajax({
                type:"get",
                url:"/fastjson/get-in-query-intranet",
                data:{"auth":JSON.stringify(auth),"action":"login"},
                success: function (data ,textStatus, jqXHR)
                {
                    $("#response").text(JSON.stringify(data));
					console.log(data);
                },
                error:function (XMLHttpRequest, textStatus, errorThrown) {      
                    alert("请求出错");
                },
            })
        }`,
						})))
						return
					}
					auth := request.URL.Query().Get("auth")
					if auth == "" {
						writer.Write([]byte("auth 参数不能为空"))
						return
					}
					response := mockController(generateFastjsonParser("intranet"), request, auth)
					writer.Write([]byte(response))
				} else {
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			},
		},
	}
	for _, v := range vuls {
		addRouteWithVulInfo(fastjsonGroup, v)
	}
}
