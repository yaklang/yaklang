package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSyntaxflow_Rule(t *testing.T) {
	t.Run("golang-template-ssti", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var extractHostRegexp = regexp.MustCompile("")

func extractPacketToGenerateParams(isHttps bool, req []byte) map[string]interface{} {
	res := make(map[string]interface{})
	res["https"] = fmt.Sprint(isHttps)
	var target = ""
	var packetRaw = req
	results := extractHostRegexp.FindSubmatchIndex(req)
	if len(results) > 3 {
		start, end := results[2], results[3]
		target = string(req[start:end])
		isMultipart := false
		header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req, func(line string) {
			if !isMultipart {
				isMultipart = strings.Contains(strings.ToLower(line), "multipart/form-data")
			}
		})
		header = strings.ReplaceAll(header, target, "{{params(target)}}")
		header = strings.ReplaceAll(header, "")
		if !isMultipart {
			// 不是上传数据包的话，就处理一下转义就行
			body = bytes.ReplaceAll(body, []byte(""))
		} else {
			// 如果是上传数据包，需要能识别出来上传的内容并重新进行编码
		}
		packetRaw = lowhttp.ReplaceHTTPPacketBody([]byte(header), body, false)
	}
	res["target"] = target
	res["packetTemplate"] = string(packetRaw)
	return res
}

var BatchPoCTemplate, _ = template.New("BatchPoCTemplate").Parse()

var OrdinaryPoCTemplate, _ = template.New("OrdinaryPoCTemplate").Parse()

func (s *Server) GenerateCSRFPocByPacket(ctx context.Context, req *ypb.GenerateCSRFPocByPacketRequest) (*ypb.GenerateCSRFPocByPacketResponse, error) {
	poc, err := yaklib.GenerateCSRFPoc(
		req.GetRequest(),
		yaklib.CsrfOptWithHTTPS(req.IsHttps),
		yaklib.CsrfOptWithAutoSubmit(req.GetAutoSubmit()),
	)
	if err != nil {
		return nil, err
	}
	return &ypb.GenerateCSRFPocByPacketResponse{Code: []byte(poc)}, nil
}

func (s *Server) GenerateYakCodeByPacket(ctx context.Context, req *ypb.GenerateYakCodeByPacketRequest) (*ypb.GenerateYakCodeByPacketResponse, error) {
	multipartReq := lowhttp.IsMultipartFormDataRequest(req.GetRequest())
	if multipartReq {
		// 处理上传数据包
		return nil, utils.Errorf("multipart/form-data; need generate specially!")
	}

	switch req.GetCodeTemplate() {
	case ypb.GenerateYakCodeByPacketRequest_Ordinary:
		var buf bytes.Buffer
		err := OrdinaryPoCTemplate.Execute(&buf, extractPacketToGenerateParams(req.GetIsHttps(), req.GetRequest()))
		if err != nil {
			return nil, utils.Errorf("generate yak code[ordinary] failed: %s", err)
		}
		return &ypb.GenerateYakCodeByPacketResponse{Code: buf.Bytes()}, nil
	case ypb.GenerateYakCodeByPacketRequest_Batch:
		var buf bytes.Buffer
		err := BatchPoCTemplate.Execute(&buf, extractPacketToGenerateParams(req.GetIsHttps(), req.GetRequest()))
		if err != nil {
			return nil, utils.Errorf("generate yak code[ordinary] failed: %s", err)
		}
		return &ypb.GenerateYakCodeByPacketResponse{Code: buf.Bytes()}, nil
	default:
		var buf bytes.Buffer
		err := OrdinaryPoCTemplate.Execute(&buf, extractPacketToGenerateParams(req.GetIsHttps(), req.GetRequest()))
		if err != nil {
			return nil, utils.Errorf("generate yak code[ordinary] failed: %s", err)
		}
		return &ypb.GenerateYakCodeByPacketResponse{Code: buf.Bytes()}, nil
	}
}

	`,
			`
template?{<fullTypeName>?{have: 'text/template'}} as $template;
template?{<fullTypeName>?{have: 'html/template'}} as $template;
$template.New() as $new

$new.Must() as $tmpl
$new.ParseFiles().* as $tmpl
$template.Must() as $tmpl
$template.ParseFiles().* as $tmpl
$tmpl.Execute(* #-> as $target);
$tmpl.ExecuteTemplate(*<slice(index=3)> #-> as $target);

template?{<fullTypeName>?{have: 'text/template'}} as $temptext;
*temp*?{<fullTypeName>?{have: 'text/template'}} as $temptext;

$temptext.New().Parse() as $target;
$temptext.ParseFiles().* -> as $target;

$target #{
    include: "* & $new",
	exclude: "*?{opcode:const}",
}-> as $high;
			`,
			map[string][]string{
				"high": {"ExternLib-template", "Undefined-template.New", "Undefined-template.New"},
			},
			ssaapi.WithLanguage(ssaconfig.GO),
		)
	})
	t.Run("golang-http-ssrf", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `
package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func NewDefaultHTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
					return nil
				},
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
				MaxVersion:         tls.VersionTLS13,
				Renegotiation:      tls.RenegotiateFreelyAsClient,
			},
			DisableKeepAlives:  true,
			DisableCompression: true,
			MaxConnsPerHost:    50,
		},
		Timeout: 15 * time.Second,
		Jar:     jar,
	}
}

func main() {
	app := cli.NewApp()

	app.Commands = []cli.Command{}

	app.Flags = []cli.Flag{
		cli.IntFlag{
			Name:  "port,p",
			Value: 8084,
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	addr := fmt.Sprintf("127.0.0.1:%v", c.Int("port"))

	defaultClient := NewDefaultHTTPClient()

	r := mux.NewRouter()
	// ?url=http://www.baidu.com
	r.HandleFunc("/ssrf", func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			writer.WriteHeader(200)
		}()
		urlIns, err := lowhttp.ExtractURLFromHTTPRequest(request, false)
		if err != nil {
			writer.Write([]byte("empty"))
			return
		}
		targetUrl := urlIns.Query().Get("url")
		log.Infof("start to trigger ssrf: %v", targetUrl)
		rsp, err := defaultClient.Get(targetUrl)
		if err != nil {
			writer.Write([]byte(err.Error()))
			return
		}
		raw, err := utils.HttpDumpWithBody(rsp, true)
		if err != nil {
			writer.Write([]byte(err.Error()))
			return
		}
		writer.Write(raw)
		return
	})
	server := &http.Server{
		Handler:      r,
		Addr:         addr,
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  10 * time.Second,
	}
	err := server.ListenAndServe()
	if err != nil {
		return err
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		return
	}
}

		`,
			`
http?{<fullTypeName>?{have: 'net/http'}} as $entry;

$entry.ResponseWriter as $input
$entry.Request as $input


http?{<fullTypeName()>?{have: "net/http"}} as $http

*.Do(<slice(index=0)>* as $client)
*.Get(<slice(index=0)>* as $client)
*.Post(<slice(index=0)>* as $client)

$client?{* #{until: `+"`"+`*<fullTypeName()>?{have: "net/http/Client"}`+"`"+`}->} as $func

$func.Get(* #-> as $param);

$param #{
	until: "* & $input" 
}-> as $mid 
			`,
			map[string][]string{
				"mid": {"Parameter-request"},
			},
			ssaapi.WithLanguage(ssaconfig.GO),
		)
	})
}
