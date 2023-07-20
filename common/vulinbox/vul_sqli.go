package vulinbox

import (
	_ "embed"
	"encoding/json"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed vul_sqli.html
var vulInSQLIViewer []byte

func sqliWriter(writer http.ResponseWriter, request *http.Request, data []*VulinUser, str ...string) {
	sqliWriterEx(false, writer, request, data, str...)
}

func sqliWriterEx(enableDebug bool, writer http.ResponseWriter, request *http.Request, data []*VulinUser, str ...string) {
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
		extraInfo = `<pre>` + strconv.Quote(strings.Join(str, "")) + `</pre> <br>`
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
	var router = s.router

	sqli := router.PathPrefix("/sqli").Name("SQL注入漏洞案例（复杂度递增）").Subrouter()

	sqli.HandleFunc("/user/by-id-safe", func(writer http.ResponseWriter, request *http.Request) {
		var a = request.URL.Query().Get("id")
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
		sqliWriter(writer, request, []*VulinUser{u})
		return
	}).Queries("id", "1").Name("不存在SQL注入的情况（数字严格校验）")
	sqli.HandleFunc("/user/id", func(writer http.ResponseWriter, request *http.Request) {
		var a = request.URL.Query().Get("id")
		u, err := s.database.GetUserByIdUnsafe(a)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(500)
			return
		}
		sqliWriter(writer, request, []*VulinUser{u})
		return
	}).Queries("id", "{id:[0-9]+}").Name("ID 为数字型的简单边界 SQL注入")
	sqli.HandleFunc("/user/id-json", func(writer http.ResponseWriter, request *http.Request) {
		var a = request.URL.Query().Get("id")
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
		sqliWriter(writer, request, []*VulinUser{u})
		return
	}).Queries("id", "{.*}").Name("参数是 JSON，JSON中字段存在SQL注入")
	sqli.HandleFunc("/user/id-b64-json", func(writer http.ResponseWriter, request *http.Request) {
		var a = request.URL.Query().Get("id")
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
		sqliWriter(writer, request, []*VulinUser{u})
		return
	}).Queries("id", "{.*}").Name("GET参数是被编码的JSON，JSON中字段存在SQL注入")

	sqli.HandleFunc("/user/id-error", func(writer http.ResponseWriter, request *http.Request) {
		var a = request.URL.Query().Get("id")
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
		sqliWriter(writer, request, []*VulinUser{u})
		return
	}).Queries("id", "{.*}").Name("ID 为数字型的简单边界SQL报错检测")

	sqli.HandleFunc("/user/cookie-id", func(writer http.ResponseWriter, request *http.Request) {
		a, err := request.Cookie("ID")
		if err != nil {
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
		u, err := s.database.GetUserByIdUnsafe(a.Value)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(500)
			return
		}
		sqliWriter(writer, request, []*VulinUser{u})
	}).Name("Cookie-ID SQL注入")

	sqli.HandleFunc("/user/name", func(writer http.ResponseWriter, request *http.Request) {
		var a = request.URL.Query().Get("name")
		u, err := s.database.GetUserByUsernameUnsafe(a)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(500)
			return
		}
		sqliWriter(writer, request, u)
		return
	}).Queries("name", "{.*}").Name("字符串为注入点的 SQL注入")

	sqli.HandleFunc("/user/name/like", func(writer http.ResponseWriter, request *http.Request) {
		db := s.database.db
		var name = LoadFromGetParams(request, "name")
		msg := `select * from vulin_users where username LIKE '%` + name + `%';`
		db = db.Raw(msg)
		if db.Error != nil {
			Failed(writer, request, db.Error.Error())
			return
		}
		var users []*VulinUser
		err := db.Scan(&users).Error
		if err != nil {
			Failed(writer, request, err.Error())
			return
		}
		sqliWriterEx(true, writer, request, users, msg)
	}).Queries("name", "{.*}").Name("字符串为注入点的 SQL注入")

	sqli.HandleFunc("/user/name/like/2", func(writer http.ResponseWriter, request *http.Request) {
		db := s.database.db
		var name = LoadFromGetParams(request, "name")
		var rowStr = `select * from vulin_users where (username LIKE '%` + name + `%') AND (age > 20);`
		db = db.Raw(rowStr)
		if db.Error != nil {
			Failed(writer, request, db.Error.Error())
			return
		}
		var users []*VulinUser
		err := db.Scan(&users).Error
		if err != nil {
			Failed(writer, request, err.Error())
			return
		}
		sqliWriterEx(true, writer, request, users, rowStr)
	}).Queries("name", "{.*}").Name("字符串注入点模糊查询(括号边界)")
	sqli.HandleFunc("/user/name/like/b64", func(writer http.ResponseWriter, request *http.Request) {
		db := s.database.db
		var name = LoadFromGetBase64Params(request, "nameb64")
		var rowStr = `select * from vulin_users where (username LIKE '%` + name + `%') AND (age > 20);`
		db = db.Raw(rowStr)
		if db.Error != nil {
			Failed(writer, request, db.Error.Error())
			return
		}
		var users []*VulinUser
		err := db.Scan(&users).Error
		if err != nil {
			Failed(writer, request, err.Error())
			return
		}
		sqliWriterEx(true, writer, request, users, rowStr)
	}).Queries("nameb64", "{.*}").Name("参数编码字符串注入点模糊查询(括号边界)")

	sqli.HandleFunc("/user/name/like/b64j", func(writer http.ResponseWriter, request *http.Request) {
		db := s.database.db
		var name = LoadFromGetBase64JSONParam(request, "data", "nameb64j")
		var rowStr = `select * from vulin_users where (username LIKE '%` + name + `%') AND (age > 20);`
		db = db.Raw(rowStr)
		if db.Error != nil {
			Failed(writer, request, db.Error.Error())
			return
		}
		var users []*VulinUser
		err := db.Scan(&users).Error
		if err != nil {
			Failed(writer, request, err.Error())
			return
		}
		sqliWriterEx(true, writer, request, users, rowStr)
	}).Queries("data", "{.*}").Name("Base64参数(JSON)嵌套字符串注入点模糊查询(括号边界)")

	sqli.HandleFunc("/user/limit/int", func(writer http.ResponseWriter, request *http.Request) {
		db := s.database.db
		var limit = LoadFromGetParams(request, "limit")
		var rowStr = `select * from vulin_users where (username LIKE '%` + "a" + `%') LIMIT ` + limit + `;`
		db = db.Raw(rowStr)
		if db.Error != nil {
			Failed(writer, request, db.Error.Error())
			return
		}
		var users []*VulinUser
		err := db.Scan(&users).Error
		if err != nil {
			Failed(writer, request, err.Error())
			return
		}
		sqliWriterEx(true, writer, request, users, rowStr)
	}).Queries("limit", "{.*}").Name("LIMIT（语句结尾）注入案例")

	sqli.HandleFunc("/user/limit/4/order1", func(writer http.ResponseWriter, request *http.Request) {
		db := s.database.db
		var data = LoadFromGetParams(request, "order")
		var rowStr = `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY username ` + data + ` LIMIT 5;`
		db = db.Raw(rowStr)
		if db.Error != nil {
			Failed(writer, request, db.Error.Error())
			return
		}
		var users []*VulinUser
		err := db.Scan(&users).Error
		if err != nil {
			Failed(writer, request, err.Error())
			return
		}
		sqliWriterEx(true, writer, request, users, rowStr)
	}).Queries("order", "{.*}").Name("ORDER注入：单个条件排序位于 LIMIT 之前")
	sqli.HandleFunc("/user/limit/4/order2", func(writer http.ResponseWriter, request *http.Request) {
		db := s.database.db
		var data = LoadFromGetParams(request, "order")
		var rowStr = `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY username ` + data + `, created_at LIMIT 5;`
		db = db.Raw(rowStr)
		if db.Error != nil {
			Failed(writer, request, db.Error.Error())
			return
		}
		var users []*VulinUser
		err := db.Scan(&users).Error
		if err != nil {
			Failed(writer, request, err.Error())
			return
		}
		sqliWriterEx(true, writer, request, users, rowStr)
	}).Queries("order", "{.*}").Name("ORDER注入：多条件排序位于 LIMIT 之前")

	sqli.HandleFunc("/user/order3", func(writer http.ResponseWriter, request *http.Request) {
		db := s.database.db
		var data = LoadFromGetParams(request, "order")
		var rowStr = `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY created_at desc, username ` + data + `;`
		db = db.Raw(rowStr)
		if db.Error != nil {
			Failed(writer, request, db.Error.Error())
			return
		}
		var users []*VulinUser
		err := db.Scan(&users).Error
		if err != nil {
			Failed(writer, request, err.Error())
			return
		}
		sqliWriterEx(true, writer, request, users, rowStr)
	}).Queries("order", "{.*}").Name("注入：多条件排序位（无LIMIT）")
	sqli.HandleFunc("/user/limit/4/orderby", func(writer http.ResponseWriter, request *http.Request) {
		db := s.database.db
		var data = LoadFromGetParams(request, "orderby")
		var rowStr = `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY ` + data + ` desc LIMIT 5;`
		db = db.Raw(rowStr)
		if db.Error != nil {
			Failed(writer, request, db.Error.Error())
			return
		}
		var users []*VulinUser
		err := db.Scan(&users).Error
		if err != nil {
			Failed(writer, request, err.Error())
			return
		}
		sqliWriterEx(true, writer, request, users, rowStr)
	}).Queries("orderby", "{.*}").Name("ORDERBY 注入：多字段")
	sqli.HandleFunc("/user/limit/4/orderby1", func(writer http.ResponseWriter, request *http.Request) {
		db := s.database.db
		var data = LoadFromGetParams(request, "orderby")
		var rowStr = `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY ` + "`" + data + "` desc" + ` LIMIT 5;`
		db = db.Raw(rowStr)
		if db.Error != nil {
			Failed(writer, request, db.Error.Error())
			return
		}
		var users []*VulinUser
		err := db.Scan(&users).Error
		if err != nil {
			Failed(writer, request, err.Error())
			return
		}
		sqliWriterEx(true, writer, request, users, rowStr)
	}).Queries("orderby", "{.*}").Name("ORDER BY注入：反引号+排序")
	sqli.HandleFunc("/user/limit/4/orderby2", func(writer http.ResponseWriter, request *http.Request) {
		db := s.database.db
		var data = LoadFromGetParams(request, "orderby")
		var rowStr = `select * from vulin_users where (username LIKE '%` + "a" + `%') ORDER BY ` + "`" + data + "`,created_at" + ` LIMIT 5;`
		db = db.Raw(rowStr)
		if db.Error != nil {
			Failed(writer, request, db.Error.Error())
			return
		}
		var users []*VulinUser
		err := db.Scan(&users).Error
		if err != nil {
			Failed(writer, request, err.Error())
			return
		}
		sqliWriterEx(true, writer, request, users, rowStr)
	}).Queries("orderby", "{.*}").Name("ORDER BY 注入：反引号+多字段")
}
