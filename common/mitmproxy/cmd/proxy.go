package main

import (
	"context"
	"fmt"
	"github.com/kataras/golog"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mitmproxy"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

var (
	sigExitOnce = new(sync.Once)
)

func init() {
	go sigExitOnce.Do(func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
		defer signal.Stop(c)

		for {
			select {
			case <-c:
				fmt.Printf("exit by signal [SIGTERM/SIGINT/SIGKILL]")
				os.Exit(1)
				return
			}
		}
	})
}

var defaultCa = []byte(`-----BEGIN CERTIFICATE-----
MIIC+jCCAeKgAwIBAgIQMqGHpwK/+AzII8h9AHyc3jANBgkqhkiG9w0BAQsFADAW
MRQwEgYDVQQDEwtDQS1mb3ItTUlUTTAgFw05OTEyMzExNjAwMDBaGA8yMTIxMDEz
MTE0MzkwMFowFjEUMBIGA1UEAxMLQ0EtZm9yLU1JVE0wggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQDuoQ91wkjhVkdKidY1aIRcvPy4LATJPLSrNq6uz86v
QcJSq6Do44dA9GwPog0WW80eNTto+fdd1RVcr7ValJNN6mDoWddpmcugUrQB2SNe
l7+mDKH6gbahJMjvohNOM2qeJ/Q+l0tgHXigMzueUuI8cATwGMPCM0M3DjjoUegy
f6Uz+nsuKYRb10Biy1Pa5uB0cI1DD/4Q+r1SvenLqwTGIV5c8COfSAMiZBGyzLsP
J21pQhOWNywJ4yf6v+JwMYeUJhH/SCKL3RWI/vyeUStcJZv4oeXURHNfobb8PObF
WoqAZ/o+Uv4rs176JRe36vEiWrM3HvKVA0d2+MdGrAKNAgMBAAGjQjBAMA4GA1Ud
DwEB/wQEAwIBpjAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBQeLJpfIiNWlEaQ
ilBR7NaKWNolLTANBgkqhkiG9w0BAQsFAAOCAQEA4jX7bdo/x8Qj5LSCLbbMcAkX
rzL54uLKij69BllBnbs+hVxJbdJLbFUtwXHiuLLs5Cw8MgWDcbgVvDczLq8S6owr
FPujGPq6wtScTrPExvkSSYDuyiTCDsPUuSMfNPNjO5zBpR0aS3NW1um07PleJ/Dq
LP1qFw7nq6nIkuDkk9tsp/k5Dx7Bn5M5gW5XUy+cw+FkGPf9WqUFlAV7xx+giP2b
SkF60e1hsSy7ANTpyNbxrd0VZMLtudZXx3L6gXuPo4GIWfbklPZFomd+Faee4k6p
ENTr6yGB3OEYF35HouUNmiO4t/9FI3BlUi5FJelsGScCM6fHGdhSmswvFFjskQ==
-----END CERTIFICATE-----`)

var defaultKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA7qEPdcJI4VZHSonWNWiEXLz8uCwEyTy0qzaurs/Or0HCUqug
6OOHQPRsD6INFlvNHjU7aPn3XdUVXK+1WpSTTepg6FnXaZnLoFK0AdkjXpe/pgyh
+oG2oSTI76ITTjNqnif0PpdLYB14oDM7nlLiPHAE8BjDwjNDNw446FHoMn+lM/p7
LimEW9dAYstT2ubgdHCNQw/+EPq9Ur3py6sExiFeXPAjn0gDImQRssy7DydtaUIT
ljcsCeMn+r/icDGHlCYR/0gii90ViP78nlErXCWb+KHl1ERzX6G2/DzmxVqKgGf6
PlL+K7Ne+iUXt+rxIlqzNx7ylQNHdvjHRqwCjQIDAQABAoIBAQDLdZu+5eZJ6sxi
K1/uraydfa1kQnPaON46VSdfeWNaXpEW96r8pnK92SkBs0PBWohrRyved7KH2JSc
MFxKXP+zoTD7Kw7VxQGvMpS0NrVHg88t/vtkoZBbmQeR+fjH5mLzclF3xHvJ+ZbN
0KD2fujSaxhqtlLCk/6tRH0U6DE4S6O04hUXdSWvM3SvG2gEGpnvzYddjAVObkx7
mvO6enTBZ+NdxMj3RgSwTcr7/RUXUvKbD8iAA5Fk28T4+3FQENs+rqXIFx8TaKYK
pF1CrrgytPsd1+z6oDUyX9k4XfL5FZq6RXY3W08cKj4vUKXbe9FD7eI6SFBh1gav
6/26BDapAoGBAPGu7OQQ3gFdPCud2dt+AgVDXUJzwrhc17IPBfBxH93Dj9g46s3s
kXorZ+hwGHhYBnD3uJpUYATkQB7hL8Kz2WUNJ0vZ/mqZep9l/QfJAyIwJOcSgOae
RTNaMFc/vZC3PYeSoDviNlCo5IS9svLxkXhT/Q2e/1bvAUgGRwPCVDzTAoGBAPzD
0cHXv4H8LCNj/bTgWaCC2yDBtQAqOqcDumVlJOPBbVFyaHbENaH1oG9560KTd4wu
O/xXZnZNewplxKvfxq/8K3ouuLYqs1gdCnEMe8EXMqBhLkd8FlKn4GR9p0en1dXk
5o5Qvj5eTmpn620wHg6HGe7Snc3xR0s3ivOfKacfAoGACEdqvAFL6ZYNCp10qg0t
+ootNqqKgBBGH0ZeeLcXVVxuoASLHpS9Awdbnt3AKNczGUmTHE5Jn8FF5Qjnvu60
Qr7pmrKUAYjSZ4Vx3oNnRROLIBNFMSE406KCR2rajouIYw2FyaddHvQ6J8XrzGC0
EAAoif/pVUwIqjP02M8eXZsCgYEA3ZgS2Xz6oMtiKriroJobGTP/Ra1ssDNVbjw/
ekr810spOoExggWr+0wqlfBtxtUftl6GKki5RDfTCZ+ElyW8u2Y4+4ngV5wB1NrI
36kRCYv7z0zDVNo9e8M/Xvol4BUMy9M8KUIyNt1Yo8JtTDEl+JiKrKwqunSviwqr
n79GtgMCgYAOfzePQhv4XPqQleiCa/bHDIjrCKztcm7rMmvgOQa0mRpnc9f3VvOa
KG58Etfww2/Nqzz3sW4VY+uS5J41+RP1k9MvqOLyrSz8DFde17rJ4o9yVemSQINc
ZAyZFnxPSNCNR7H+x4/RPee2qZAcV7pxqyl/9N4rA+kZGiwNe1T9mQ==
-----END RSA PRIVATE KEY-----`)

func main() {
	app := cli.NewApp()

	app.Commands = []cli.Command{}

	app.Flags = []cli.Flag{
		cli.IntFlag{
			Name:  "port",
			Value: 8088,
		},
	}

	app.Before = func(context *cli.Context) error {
		log.SetLevel(golog.InfoLevel)
		return nil
	}

	app.Action = func(c *cli.Context) error {
		log.Info("start to startup mitmproxy...")
		ca, key, _ := crep.GetDefaultCaAndKey()
		proxy, err := mitmproxy.NewMITMProxy(
			mitmproxy.WithCaCert(ca, key),
			mitmproxy.WithHijackRequest(func(isHttps bool, req *http.Request, raw []byte) []byte {
				//log.Infof("hijack req[%v] [https:%v] len: %v", req.Method, isHttps, len(raw))
				//println(string(raw))
				return raw
			}),
			mitmproxy.WithHijackResponse(func(isHttps bool, req *http.Request, raw []byte, remoteAddr string) []byte {
				//log.Infof("hijack rsp[%v] [https:%v] len: %v", remoteAddr, isHttps, len(raw))
				return raw
			}),
			mitmproxy.WithMirrorResponse(func(isHttps bool, req *http.Request, rsp *http.Response, remoteAddr string) {
				//urlReq, _ := lowhttp.ExtractURLFromHTTPRequest(req, isHttps)
				//var urlStr = urlReq.String()
				//log.Infof("mirror[%v] %v => %v", urlStr, req.RemoteAddr, remoteAddr)
			}),
			mitmproxy.WithWebHook(func(req *http.Request) []byte {
				// 如果把这个代理当成 xray 的 webhook
				// 可以在这个函数中处理请求
				println("--------------------------------------------")
				println("------------- WEBHOOK IS ME ----------------")
				println("--------------------------------------------")
				raw, _ := httputil.DumpRequest(req, true)
				println(string(raw))
				return nil
			}),
		)
		if err != nil {
			return err
		}

		return proxy.Run(context.Background())
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		return
	}
}
