package verificationcode

import (
	_ "embed"
	"fmt"
	"github.com/ReneKroon/ttlcache"
	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/mux"
	"github.com/segmentio/ksuid"
	"github.com/steambap/captcha"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed op.html
var opHtml []byte

//go:embed success.html
var secretHtml []byte

func Register(t *mux.Router) {
	var sessionCacher = ttlcache.NewCache()
	sessionCacher.SetTTL(30 * time.Minute)
	var defaultPass = mutate.QuickMutateSimple(`{{ri(0,9999|4)}}`)[0]
	log.Infof("default pass generated: %v", defaultPass)

	const COOKIECONST = "YSESSIONID"
	t.HandleFunc("/verification/op", func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("panic: %v", err)
				writer.WriteHeader(500)
				writer.Write([]byte(`PANIC! Please contact the administrator!`))
				return
			}
		}()
		rawBytes, _ := utils.HttpDumpWithBody(request, true)
		if request.Method == "POST" {
			var code = lowhttp.GetHTTPRequestPostParam(rawBytes, "code")
			var password = lowhttp.GetHTTPRequestPostParam(rawBytes, "password")
			var session = lowhttp.GetHTTPPacketCookie(rawBytes, COOKIECONST)
			val, ok := sessionCacher.Get(session)
			if !ok {
				writer.WriteHeader(500)
				writer.Write([]byte(`session not found`))
				return
			}
			data, ok := val.(map[string]any)["code"].(*captcha.Data)
			if !ok {
				writer.WriteHeader(500)
				writer.Write([]byte(`session-code not found`))
				return
			}
			log.Infof("data.Text: " + data.Text)
			//var newData, _ = captcha.New(150, 50)
			//val.(map[string]any)["code"] = newData
			if strings.ToLower(data.Text) != strings.ToLower(code) {
				writer.WriteHeader(500)
				writer.Write([]byte(`verification code not match`))
				return
			}

			if password != defaultPass {
				writer.Write([]byte(`{"code":500,"msg":"密码错误"}`))
			}
			writer.Write(secretHtml)
			return
		}

		if request.Method == "GET" {
			headers := writer.Header()
			headers.Set("Content-Type", "text/html; charset=UTF8")
			headers.Set("Test-Header", "Test-Value")
			uid := ksuid.New().String()
			log.Infof("/verification generate cookie: %v", uid)
			http.SetCookie(writer, &http.Cookie{
				Name:  COOKIECONST,
				Value: uid,
			})
			sessionCacher.Set(uid, map[string]any{})
			writer.Write(opHtml)
			return
		}
	})
	t.HandleFunc("/verification/code", func(writer http.ResponseWriter, request *http.Request) {
		reqRaw, _ := utils.HttpDumpWithBody(request, true)
		session := lowhttp.GetHTTPPacketCookie(reqRaw, COOKIECONST)
		if session == "" {
			writer.WriteHeader(502)
			writer.Write([]byte(`{"code":500,"msg":"验证码生成失败(NO COOKIE)"}`))
			return
		}

		v, ok := sessionCacher.Get(session)
		if !ok {
			writer.Write([]byte(`{"code":500,"msg":"验证码生成失败(COOKIE NOT GENERATED)"}`))
			writer.WriteHeader(502)
			return
		}

		kv, ok := v.(map[string]any)
		if !ok {
			writer.Write([]byte(`{"code":500,"msg":"验证码生成失败(COOKIE NOT GENERATED)"}`))
			writer.WriteHeader(502)
			return
		}

		data, ok := utils.MapGetRaw(kv, "code").(*captcha.Data)
		if !ok {
			var err error
			data, err = captcha.New(150, 50)
			if err != nil {
				spew.Dump(err)
				writer.Write([]byte(`{"code":500,"msg":"验证码生成失败(captch.New)"}`))
				writer.WriteHeader(502)
				return
			}
			kv["code"] = data
		}

		err := data.WriteImage(writer)
		if err != nil {
			writer.Write([]byte(fmt.Sprintf(`{"code":500,"msg":"验证码生成失败: %v"}`, strconv.Quote(err.Error()))))
			writer.WriteHeader(502)
			return
		}
		return
	})
}
