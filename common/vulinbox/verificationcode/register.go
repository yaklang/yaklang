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
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed op.html
var opHtml []byte

//go:embed op1.html
var op1Html []byte

//go:embed success.html
var secretHtml []byte

var (
	sessionCacher = ttlcache.NewCache()
	defaultPass   = mutate.QuickMutateSimple(`{{ri(0,9999|4)}}`)[0]
)

const COOKIECONST = "YSESSIONID"

func init() {
	sessionCacher.SetTTL(30 * time.Minute)
	log.Infof("default pass generated: %v", defaultPass)
}

func Register(t *mux.Router) (*mux.Router, []string) {
	verificationGroup := t.PathPrefix("/verification").Name("验证码场景").Subrouter()

	const (
		ordinaryCase1 = `{"Title":"有验证码拦截的表单提交", "Path":"/op", "DefaultQuery": "", "RiskDetected": false, "Headers": [], "ExpectedResult": {}}`
		ordinaryCase2 = `{"Title":"有验证码拦截的表单提交（逻辑问题）", "Path":"/bad/op", "DefaultQuery": "", "RiskDetected": false, "Headers": [], "ExpectedResult": {}}`
	)
	// 最普通的案例
	verificationGroup.HandleFunc("/op", func(writer http.ResponseWriter, request *http.Request) {
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
	}).Name(ordinaryCase1)
	verificationGroup.HandleFunc("/code", func(writer http.ResponseWriter, request *http.Request) {
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

	const COOKIECONST_BAD = "__YSESSIONID1"
	verificationGroup.HandleFunc("/bad/op", func(writer http.ResponseWriter, request *http.Request) {
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
			var session = lowhttp.GetHTTPPacketCookie(rawBytes, COOKIECONST_BAD)
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
			uid := ksuid.New().String()
			log.Infof("/verification generate cookie: %v", uid)
			http.SetCookie(writer, &http.Cookie{
				Name:  COOKIECONST_BAD,
				Value: uid,
			})
			data, err := captcha.New(150, 50)
			if err != nil {
				spew.Dump(err)
				writer.Write([]byte(`{"code":500,"msg":"验证码生成失败(captch.New)"}`))
				return
			}
			http.SetCookie(writer, &http.Cookie{
				Name:  "_ccc",
				Value: codec.EncodeBase64(data.Text),
			})
			sessionCacher.Set(uid, map[string]any{
				"code": data,
			})
			writer.Write(op1Html)
			return
		}
	}).Name(ordinaryCase2)
	verificationGroup.HandleFunc("/bad/code", func(writer http.ResponseWriter, request *http.Request) {
		reqRaw, _ := utils.HttpDumpWithBody(request, true)
		session := lowhttp.GetHTTPPacketCookie(reqRaw, COOKIECONST_BAD)
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
			writer.Write([]byte(`{"code":500,"msg":"请访问原主页生成验证码"}`))
			writer.WriteHeader(502)
			return
		}

		err := data.WriteImage(writer)
		if err != nil {
			writer.Write([]byte(fmt.Sprintf(`{"code":500,"msg":"验证码生成失败: %v"}`, strconv.Quote(err.Error()))))
			writer.WriteHeader(502)
			return
		}
		return
	})
	return verificationGroup, []string{
		ordinaryCase1, ordinaryCase2,
	}
}
