package vulinbox

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"github.com/samber/lo"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed html/vul_sqli.html
var vulInSQLIViewer []byte

func sqliWriter(writer http.ResponseWriter, request *http.Request, data []interface{}, str ...string) {
	sqliWriterEx(false, writer, request, data, str...)
}

func sqliWriterEx(enableDebug bool, writer http.ResponseWriter, request *http.Request, data []interface{}, str ...string) {
	raw, err := json.Marshal(data)
	if err != nil {
		Failed(writer, request, err.Error())
		return
	}

	if request.URL.Query().Get("debug") == "" {
		str = nil
	}
	var extraInfo string
	if len(str) > 0 {
		var buf bytes.Buffer
		for _, s := range str {
			buf.WriteString(`<pre>` + strconv.Quote(s) + "</pre> <br>")
		}
		extraInfo = buf.String()
	}
	var debugstyle string
	if !enableDebug {
		debugstyle = `style='display: none;'`
	} else {
		debugstyle = `style='margin-bottom: 24px;'`
	}
	unsafeTemplateRender(writer, request, string(vulInSQLIViewer), map[string]any{
		"userjson":   string(raw),
		"extra":      extraInfo,
		"debugstyle": debugstyle,
	})
}

func (s *VulinServer) registerSQLinj() {
	router := s.router

	sqli := router.Name("SQL注入漏洞案例（复杂度递增）").Subrouter()

	vroutes := []*VulInfo{
		{
			DefaultQuery: "id=1",
			Path:         "/user/by-id-safe",
			Title:        "不存在SQL注入的情况（数字严格校验）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				a := request.URL.Query().Get("id")
				i, err := strconv.ParseInt(a, 10, 64)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				u, err := s.database.GetUserById(int(i))
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				sqliWriter(writer, request, []interface{}{u})
				return
			},
			RiskDetected:   false,
			ExpectedResult: map[string]int{"参数:id未检测到闭合边界": 1},
		},
		{
			DefaultQuery: "id=1",
			Path:         "/user/id",
			Title:        "ID 为数字型的简单边界 SQL注入",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				a := request.URL.Query().Get("id")
				u, err := s.database.GetUserByIdUnsafe(a)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				sqliWriter(writer, request, []interface{}{u})
				return
			},
			RiskDetected:   true,
			ExpectedResult: map[string]int{"疑似SQL注入：【参数：数字型[id] 无边界闭合】": 1, "存在基于UNION SQL 注入: [参数名:id 原值:1]": 1},
		},
		{
			DefaultQuery: `id={"uid":1,"id":"1"}`,
			Path:         "/user/id-json",
			Title:        "参数是 JSON,JSON中字段存在SQL注入",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				a := request.URL.Query().Get("id")
				var jsonMap map[string]any
				err := json.Unmarshal([]byte(a), &jsonMap)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				a, ok := jsonMap["id"].(string)
				if !ok {
					writer.Write([]byte("Failed to retrieve the 'id' field"))
					writer.WriteHeader(500)
					return
				}

				u, err := s.database.GetUserByIdUnsafe(a)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				sqliWriter(writer, request, []interface{}{u})
				return
			},
			RiskDetected:   true,
			ExpectedResult: map[string]int{"疑似SQL注入：【参数：数字型[id] 无边界闭合】": 1, "存在基于UNION SQL 注入: [参数名:id 原值:1]": 1},
		},
		{
			DefaultQuery: "id=eyJ1aWQiOjEsImlkIjoiMSJ9",
			Path:         "/user/id-b64-json",
			Title:        "GET参数是被编码的JSON，JSON中字段存在SQL注入",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				a := request.URL.Query().Get("id")
				decodedB64, err := codec.DecodeBase64(a)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				var jsonMap map[string]any
				err = json.Unmarshal(decodedB64, &jsonMap)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				a, ok := jsonMap["id"].(string)
				if !ok {
					writer.Write([]byte("Failed to retrieve the 'id' field"))
					writer.WriteHeader(500)
					return
				}

				u, err := s.database.GetUserByIdUnsafe(a)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				sqliWriter(writer, request, []interface{}{u})
				return
			},
			RiskDetected:   true,
			ExpectedResult: map[string]int{"疑似SQL注入：【参数：数字型[id] 无边界闭合】": 1, "存在基于UNION SQL 注入: [参数名:id 原值:1]": 1},
		},
		{
			DefaultQuery: "id=1",
			Path:         "/user/id-error",
			Title:        "ID 为数字型的简单边界SQL报错检测",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				a := request.URL.Query().Get("id")
				u, err := s.database.GetUserByIdUnsafe(a)
				if err != nil {
					writer.Write([]byte(`You have an error in your SQL syntax; check the manual that corresponds to your MySQL server version for the right syntax to use near ''1'' LIMIT 0,1' at line 1`))
					writer.WriteHeader(500)
					return
				}
				_, err = json.Marshal(u)
				if err != nil {
					writer.Write([]byte(`You have an error in your SQL syntax; check the manual that corresponds to your MySQL server version for the right syntax to use near ''1'' LIMIT 0,1' at line 1`))
					writer.WriteHeader(500)
					return
				}
				sqliWriter(writer, request, []interface{}{u})
				return
			},
			RiskDetected:   true,
			ExpectedResult: map[string]int{"疑似SQL注入：【参数：数字[id] 无边界闭合】": 1, "存在基于UNION SQL 注入: [参数名:id 值:[1]]": 1},
		},
		{
			DefaultQuery: "",
			Path:         "/user/cookie-id",
			Title:        "Cookie-ID SQL注入",
			Headers: []*ypb.KVPair{
				{
					Key:   "Cookie",
					Value: "ID=1",
				},
			},
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				raw, _ := utils.HttpDumpWithBody(request, true)
				id := lowhttp.GetHTTPPacketCookieFirst(raw, "ID")
				if id == "" && lowhttp.GetHTTPRequestQueryParam(raw, "skip") != "1" {
					cookie := http.Cookie{
						Name:     "ID",
						Value:    "1",                                // 设置 cookie 的值
						Expires:  time.Now().Add(7 * 24 * time.Hour), // 设置过期时间
						HttpOnly: false,                              // 仅限 HTTP 访问，不允许 JavaScript 访问
					}
					http.SetCookie(writer, &cookie)
					writer.Header().Set("Location", "/user/cookie-id?skip=1")
					if request.URL.Query().Get("skip") == "1" {
						writer.WriteHeader(200)
						writer.Write([]byte("Cookie set"))
					} else {
						writer.WriteHeader(302)
					}
					return
				}
				if strings.Contains(id, "%") {
					idUnesc, err := url.QueryUnescape(id)
					if err != nil {
						writer.Header().Set("Location", "/user/cookie-id?skip=1")
						writer.WriteHeader(302)
						return
					}
					id = idUnesc
				}
				u, err := s.database.GetUserByIdUnsafe(id)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				sqliWriter(writer, request, []interface{}{u})
			},
			RiskDetected:   true,
			ExpectedResult: map[string]int{"疑似SQL注入：【参数：数字[ID] 双引号闭合】": 1, "存在基于UNION SQL 注入: [参数名:ID 值:[1]]": 1},
		},
		{
			DefaultQuery: "name=admin",
			Path:         "/user/name",
			Title:        "字符串为注入点的 SQL注入",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				a := request.URL.Query().Get("name")
				u, err := s.database.GetUserByUsernameUnsafe(a)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				sqliWriter(writer, request, lo.Map(u, func(i map[string]interface{}, _ int) interface{} {
					return i
				}))
				return
			},
			RiskDetected:   true,
			ExpectedResult: map[string]int{"疑似SQL注入：【参数：字符串[name] 单引号闭合】": 1, "存在基于UNION SQL 注入: [参数名:name 原值:admin]": 1},
		},
		{
			DefaultQuery: "name=a",
			Path:         "/user/name/like",
			Title:        "字符串注入点模糊查询",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				name := LoadFromGetParams(request, "name")
				rowStr := `select * from vulin_users where username LIKE '%` + name + `%';`
				users, err := s.database.UnsafeSqlQuery(rowStr)
				if err != nil {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr, err.Error())
				} else {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr)
				}
			},
			RiskDetected:   true,
			ExpectedResult: map[string]int{"疑似SQL注入：【参数：字符串[name] like注入( %' )】": 1, "存在基于UNION SQL 注入: [参数名:name 值:[a]]": 1},
		},
		{
			DefaultQuery: "name=a",
			Path:         "/user/name/like/2",
			Title:        "字符串注入点模糊查询(括号边界)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				name := LoadFromGetParams(request, "name")
				rowStr := `select * from vulin_users where (username LIKE '%` + name + `%') AND (age > 20);`
				users, err := s.database.UnsafeSqlQuery(rowStr)
				if err != nil {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr, err.Error())
				} else {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr)
				}
			},
			RiskDetected:   true,
			ExpectedResult: map[string]int{"疑似SQL注入：【参数：字符串[name] like注入( %' )】": 1},
		},
		{
			DefaultQuery: "nameb64=YQ==",
			Path:         "/user/name/like/b64",
			Title:        "参数编码字符串注入点模糊查询(括号边界)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				name := LoadFromGetBase64Params(request, "nameb64")
				rowStr := `select * from vulin_users where (username LIKE '%` + name + `%') AND (age > 20);`
				users, err := s.database.UnsafeSqlQuery(rowStr)
				if err != nil {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr, err.Error())
				} else {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr)
				}
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "data=eyJuYW1lYjY0aiI6ImEifQ==",
			Path:         "/user/name/like/b64j",
			Title:        "Base64参数(JSON)嵌套字符串注入点模糊查询(括号边界)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				name := LoadFromGetBase64JSONParam(request, "data", "nameb64j")
				rowStr := `select * from vulin_users where (username LIKE '%` + name + `%') AND (age > 20);`
				users, err := s.database.UnsafeSqlQuery(rowStr)
				if err != nil {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr, err.Error())
				} else {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr)
				}
			},
			RiskDetected:   true,
			ExpectedResult: map[string]int{"疑似SQL注入：【参数：字符串[data] like注入( %' )】": 1},
		},
		{
			DefaultQuery: "limit=1",
			Path:         "/user/limit/int",
			Title:        "LIMIT（语句结尾）注入案例",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				limit := LoadFromGetParams(request, "limit")
				rowStr := `select * from vulin_users where (username LIKE '%` + "a" + `%') LIMIT ` + limit + `;`
				users, err := s.database.UnsafeSqlQuery(rowStr)
				if err != nil {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr, err.Error())
				} else {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr)
				}
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "order=desc",
			Path:         "/user/limit/4/order1",
			Title:        "ORDER注入：单个条件排序位于 LIMIT 之前",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				data := LoadFromGetParams(request, "order")
				rowStr := `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY username ` + data + ` LIMIT 5;`
				users, err := s.database.UnsafeSqlQuery(rowStr)
				if err != nil {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr, err.Error())
				} else {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr)
				}
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "order=desc",
			Path:         "/user/limit/4/order2",
			Title:        "ORDER注入：多条件排序位于 LIMIT 之前",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				data := LoadFromGetParams(request, "order")
				rowStr := `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY username ` + data + `, created_at LIMIT 5;`
				users, err := s.database.UnsafeSqlQuery(rowStr)
				if err != nil {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr, err.Error())
				} else {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr)
				}
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "order=desc",
			Path:         "/user/order3",
			Title:        "注入：多条件排序位（无LIMIT）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				data := LoadFromGetParams(request, "order")
				rowStr := `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY created_at desc, username ` + data + `;`
				users, err := s.database.UnsafeSqlQuery(rowStr)
				if err != nil {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr, err.Error())
				} else {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr)
				}
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "orderby=username",
			Path:         "/user/limit/4/orderby",
			Title:        "ORDERBY 注入：多字段",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				data := LoadFromGetParams(request, "orderby")
				rowStr := `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY ` + data + ` desc LIMIT 5;`
				users, err := s.database.UnsafeSqlQuery(rowStr)
				if err != nil {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr, err.Error())
				} else {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr)
				}
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "orderby=id",
			Path:         "/user/limit/4/orderby1",
			Title:        "ORDER BY注入：反引号+排序",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				data := LoadFromGetParams(request, "orderby")
				rowStr := `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY ` + "`" + data + "` desc" + ` LIMIT 5;`
				users, err := s.database.UnsafeSqlQuery(rowStr)
				if err != nil {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr, err.Error())
				} else {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr)
				}
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "orderby=id",
			Path:         "/user/limit/4/orderby2",
			Title:        "ORDER BY 注入：反引号+多字段",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				data := LoadFromGetParams(request, "orderby")
				rowStr := `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY ` + "`" + data + "`,created_at" + ` LIMIT 5;`
				users, err := s.database.UnsafeSqlQuery(rowStr)
				if err != nil {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr, err.Error())
				} else {
					sqliWriterEx(true, writer, request, utils.InterfaceToSliceInterface(users), rowStr)
				}
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "id=1",
			Path:         "/user/id-time-blind",
			Title:        "时间盲注",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				a := request.URL.Query().Get("id")
				u, err := s.database.GetUserByIdUnsafe(a)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(500)
					return
				}
				sqliWriter(writer, request, []interface{}{u})
				return
			},
			RiskDetected: true,
		},
	}
	for _, v := range vroutes {
		addRouteWithVulInfo(sqli, v)
	}
}
