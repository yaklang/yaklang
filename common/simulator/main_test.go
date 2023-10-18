// Package simulator
// @Author bcy2007  2023/8/23 14:49
package simulator

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	loginHTML = `
<html>
  <head>
    <meta charset="utf-8" />
    <meta http-equiv="X-UA-Compatible" content="IE=edge" />
    <meta name="viewport" content="width=device-width,initial-scale=1" />
    <link rel="icon" href="/favicon.ico" />
    <title>自动化渗透测试平台</title>
    <script defer="defer" src="/static/js/chunk-vendors.1f13e67f.js"></script>
    <script defer="defer" src="/static/js/app.601a8c9d.js"></script>
    <link href="/static/css/chunk-vendors.b770f8a9.css" rel="stylesheet" />
    <link href="/static/css/app.baec982c.css" rel="stylesheet" />
    <style type="text/css">
      .jv-node {
        position: relative;
      }
      .jv-node:after {
        content: ",";
      }
      .jv-node:last-of-type:after {
        content: "";
      }
      .jv-node.toggle {
        margin-left: 13px !important;
      }
      .jv-node .jv-node {
        margin-left: 25px;
      }
    </style>
    <style type="text/css">
      .jv-container {
        box-sizing: border-box;
        position: relative;
      }
      .jv-container.boxed {
        border: 1px solid #eee;
        border-radius: 6px;
      }
      .jv-container.boxed:hover {
        box-shadow: 0 2px 7px rgba(0, 0, 0, 0.15);
        border-color: transparent;
        position: relative;
      }
      .jv-container.jv-light {
        background: #fff;
        white-space: nowrap;
        color: #525252;
        font-size: 14px;
        font-family: Consolas, Menlo, Courier, monospace;
      }
      .jv-container.jv-light .jv-ellipsis {
        color: #999;
        background-color: #eee;
        display: inline-block;
        line-height: 0.9;
        font-size: 0.9em;
        padding: 0px 4px 2px 4px;
        margin: 0 4px;
        border-radius: 3px;
        vertical-align: 2px;
        cursor: pointer;
        -webkit-user-select: none;
        user-select: none;
      }
      .jv-container.jv-light .jv-button {
        color: #49b3ff;
      }
      .jv-container.jv-light .jv-key {
        color: #111111;
        margin-right: 4px;
      }
      .jv-container.jv-light .jv-item.jv-array {
        color: #111111;
      }
      .jv-container.jv-light .jv-item.jv-boolean {
        color: #fc1e70;
      }
      .jv-container.jv-light .jv-item.jv-function {
        color: #067bca;
      }
      .jv-container.jv-light .jv-item.jv-number {
        color: #fc1e70;
      }
      .jv-container.jv-light .jv-item.jv-object {
        color: #111111;
      }
      .jv-container.jv-light .jv-item.jv-undefined {
        color: #e08331;
      }
      .jv-container.jv-light .jv-item.jv-string {
        color: #42b983;
        word-break: break-word;
        white-space: normal;
      }
      .jv-container.jv-light .jv-item.jv-string .jv-link {
        color: #0366d6;
      }
      .jv-container.jv-light .jv-code .jv-toggle:before {
        padding: 0px 2px;
        border-radius: 2px;
      }
      .jv-container.jv-light .jv-code .jv-toggle:hover:before {
        background: #eee;
      }
      .jv-container .jv-code {
        overflow: hidden;
        padding: 30px 20px;
      }
      .jv-container .jv-code.boxed {
        max-height: 300px;
      }
      .jv-container .jv-code.open {
        max-height: initial !important;
        overflow: visible;
        overflow-x: auto;
        padding-bottom: 45px;
      }
      .jv-container .jv-toggle {
        background-image: url(data:image/svg+xml;base64,PHN2ZyBoZWlnaHQ9IjE2IiB3aWR0aD0iOCIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KIAo8cG9seWdvbiBwb2ludHM9IjAsMCA4LDggMCwxNiIKc3R5bGU9ImZpbGw6IzY2NjtzdHJva2U6cHVycGxlO3N0cm9rZS13aWR0aDowIiAvPgo8L3N2Zz4=);
        background-repeat: no-repeat;
        background-size: contain;
        background-position: center center;
        cursor: pointer;
        width: 10px;
        height: 10px;
        margin-right: 2px;
        display: inline-block;
        -webkit-transition: -webkit-transform 0.1s;
        transition: -webkit-transform 0.1s;
        transition: transform 0.1s;
        transition: transform 0.1s, -webkit-transform 0.1s;
      }
      .jv-container .jv-toggle.open {
        -webkit-transform: rotate(90deg);
        transform: rotate(90deg);
      }
      .jv-container .jv-more {
        position: absolute;
        z-index: 1;
        bottom: 0;
        left: 0;
        right: 0;
        height: 40px;
        width: 100%;
        text-align: center;
        cursor: pointer;
      }
      .jv-container .jv-more .jv-toggle {
        position: relative;
        top: 40%;
        z-index: 2;
        color: #888;
        -webkit-transition: all 0.1s;
        transition: all 0.1s;
        -webkit-transform: rotate(90deg);
        transform: rotate(90deg);
      }
      .jv-container .jv-more .jv-toggle.open {
        -webkit-transform: rotate(-90deg);
        transform: rotate(-90deg);
      }
      .jv-container .jv-more:after {
        content: "";
        width: 100%;
        height: 100%;
        position: absolute;
        bottom: 0;
        left: 0;
        z-index: 1;
        background: -webkit-linear-gradient(
          top,
          rgba(0, 0, 0, 0) 20%,
          rgba(230, 230, 230, 0.3) 100%
        );
        background: linear-gradient(
          to bottom,
          rgba(0, 0, 0, 0) 20%,
          rgba(230, 230, 230, 0.3) 100%
        );
        -webkit-transition: all 0.1s;
        transition: all 0.1s;
      }
      .jv-container .jv-more:hover .jv-toggle {
        top: 50%;
        color: #111;
      }
      .jv-container .jv-more:hover:after {
        background: -webkit-linear-gradient(
          top,
          rgba(0, 0, 0, 0) 20%,
          rgba(230, 230, 230, 0.3) 100%
        );
        background: linear-gradient(
          to bottom,
          rgba(0, 0, 0, 0) 20%,
          rgba(230, 230, 230, 0.3) 100%
        );
      }
      .jv-container .jv-button {
        position: relative;
        cursor: pointer;
        display: inline-block;
        padding: 5px;
        z-index: 5;
      }
      .jv-container .jv-button.copied {
        opacity: 0.4;
        cursor: default;
      }
      .jv-container .jv-tooltip {
        position: absolute;
      }
      .jv-container .jv-tooltip.right {
        right: 15px;
      }
      .jv-container .jv-tooltip.left {
        left: 15px;
	  }
      .jv-container .j-icon {
        font-size: 12px;
      }
    </style>
    <link
      rel="stylesheet"
      type="text/css"
      href="/static/css/9205.181ec003.css"
    />
  </head>
  <body>
    <noscript
      ><strong
        >We're sorry but xiaozhi_vue_standard doesn't work properly without
        JavaScript enabled. Please enable it to continue.</strong
      ></noscript
    >
	<img src="" id="testimg"/>
    <div data-v-792e9a7c="" id="app">
      <div
        data-v-785f3b18=""
        data-v-792e9a7c=""
        id="loginpage"
        class="loginpage"
      >
        <div data-v-785f3b18="" class="login-bg">
          <div data-v-785f3b18="">
            <div data-v-785f3b18="" class="img">
              <img data-v-785f3b18="" src="/static/img/login.2a157146.png" />
            </div>
            <form
              data-v-785f3b18=""
              class="el-form demo-ruleForm login-container el-form--label-left"
            >
              <h3 data-v-785f3b18="" class="title">欢迎登录</h3>
              <!---->
              <div data-v-785f3b18="" class="el-form-item is-required">
                <!---->
                <div class="el-form-item__content" style="margin-left: 0px">
                  <div data-v-785f3b18="" class="el-input" name="username">
                    <!----><input
                      type="text"
                      autocomplete="off"
                      placeholder=""
                      class="el-input__inner"
                    /><!----><!----><!----><!---->
                  </div>
                  <!---->
                </div>
              </div>
              <div data-v-785f3b18="" class="el-form-item is-required">
                <!---->
                <div class="el-form-item__content" style="margin-left: 0px">
                  <div data-v-785f3b18="" class="el-input">
                    <!----><input
                      type="password"
                      autocomplete="off"
                      placeholder="请输入密码"
                      class="el-input__inner"
                    /><!----><!----><!----><!---->
                  </div>
                  <!---->
                </div>
              </div>
              <div data-v-785f3b18="" class="el-form-item">
                <!---->
                <div class="el-form-item__content" style="margin-left: 0px">
                  <div data-v-785f3b18="" class="el-input" style="width: 180px">
                    <!----><input
                      type="text"
                      autocomplete="off"
                      placeholder="请输入验证码"
                      class="el-input__inner"
                    /><!----><!----><!----><!---->
                  </div>
                  <img
                    data-v-785f3b18=""
                    src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAHgAAAAoCAYAAAA16j4lAAAFUElEQVR4nOyafYhUVR/HP8cXHh99nIFnrpDXaosiyl5MsMaM/KMWJAgrM6lssCzKMicsNq+RqGV0YZHkGglh+sekJhhrbxS4RtGLXcWyJAvCl4Ucre6WZ8pKTG+cOye72s66M3dmBy/3A7Nn7tyzv/Ob891zzvecu4N83ychvgxodgIJjSUROOYkAsecROCYkwgccxKBY04icMxJBI45icAxJxE45gxqdgL1JpsuDgVmA7cBFwPDgAPAZqDdleauZufYn4g4nUVn08VLgHeAlgpVjgAzXGmu7+fUmkZsBM6miyOAL4GzgFeA1cAOfA4juAJ4ErgFOAqMcaX5dbNzPpWckR4OtAFTgQv1x3uADsAueFJWGzNOAitRpwOvudKcWqFOhxZ5lSvN+/oa++rHF1fVSVuXLhTV1Kcs7v+BD4HRFarsBq4rePJANXFPWoPbW1rVl54PnAvsBZa1dXWuqDbZMHk7MxDEx6p/gdsdy9vwzz1jihIEeMOxvJtrbSObLo4E7gT+AB7rpeozWuBJtbbVQBZqcfcBc4D3gYHA9YADXAAsBe6qJuiJEdze0noNUAAeBFz1hwu8qkRp6+r8IErmedtQiX8G/KSMj2N5pbxtpIBvtAka7Vje/lrjZ9PFR4DlwMuuNO+PkmuzyBlp9f1N1e8FT2475d54YAtQKngyXU3c8Ai+B5jX1tW5WV+/197SugC4FYgksGN5u/K2sUSPIBt4GHgOUCNvdhRxNeN1+WbEOE2j4MlRvdzeq8sj1cYNC5zVnR/mI2BmtUF7xrdBqK3LrLxtfAc8BHzi40daAjSX6/ILvU3KA9OAi/TnylCtwWeFWzKr7qRmksukBiPESn25odrfDx90nAN8f8r9/Xruj4xjdf8J3AscA57VbvaB5VZ3PVzeSF2q6Wunnh3G6ulfvcYBzyPYotfrM4JcJjUIIdYBN2mTNb/aGOERPBw/MClhfgNS0VMt41jejrxtbNTbgLccy/uqTqH/p8uO4HuUDzpex+cgIrin/MRTwERl6LKp4gS3ZB7ta/D+cNE9IsQqfWCjBt6kWrZJAyq8L+NzHIJXXcjbxmXA3255ct42xtYp9BBd/kfp4UrzRVea+92SecyVpnSluQmfVm0exyGYUad2G0bOSKslJgcoUW8oeHJ3LXHCI7iECDro9xOfCIbqBiIzx84IgXhJt6m2BIuA1Xk7c5Vjdfd5NFVAzTz/BSxXmnt7qqBGbDZdVFO3mkHuAFb2NXjdRmR1zNblvIIna57pwqN2j7bpYUaFHFwkBEKZKrUVW+lY3tNqSwOMATGvDuFLunz7NPW26PLKOrTZaMbpcmOUIGGBt+q1Koy63h6lAcpT89naoR/QR3GKJ4CDwIK8bVwasYl9wU+fw73W8jmk3w2L2F5DyWVSSpehwYXv/xglVniKVm5tTXtL6w/ANi3uIr3IR+UFbX5mOpYXTPmO5f2ct41HgfXAqrydmeBY3cdqjP95sM0TwbZoZ8VaghH6XdR9d0MpdJeOB9nWgZPOottbWmfpY7LzgC5gSVtX59p6NNRIsuni5MA1wzJXmnN7qXe3Pq1b60pzev9m2Rxi8bAhmyoORvCt9hATXWl++q865QOQ7foZ8Y2uNN9tTrZ9I2ekA2EKnow0kmMhMGUBJ+lnwb9ql74RnyIiWG+vBRbrw48OV5pTmp3v6UgE7oFsujhdb3+GVKiyCZjmSvNQP6fWNGIlMGWRzwfm6keCyr3/AuxSRg6fdW7JrNXInZHETuCEk0n+qzLmJALHnETgmJMIHHMSgWNOInDMSQSOOYnAMeevAAAA///P/4+h8mI+XQAAAABJRU5ErkJggg=="
                    id="code"
                    alt="验证码"
                    class="logincode"
                  /><!---->
                </div>
              </div>
              <div data-v-785f3b18="" class="el-form-item" style="width: 100%">
                <!---->
                <div class="el-form-item__content" style="margin-left: 0px">
                  <button
                    data-v-785f3b18=""
                    type="button"
                    class="el-button el-button--primary"
                    style="width: 100%"
                  >
                    <!----><!----><span>登录</span></button
                  ><!---->
                </div>
              </div>
            </form>
          </div>
        </div>
      </div>
    </div>
  </body>
</html>
`

	LoginHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width,initial-scale=1.0">
	<link rel="shortcut icon" href="#"/>
    <title>Login</title>
</head>
<body>

<form action="/login" method="post">
    用户名称:<input type="text" name="username"><br>
    密码:<input type="password" name="password"><br>
    <input type="submit" value="Login">
</form>

</body>
</html>`

	ResultHTML = `<!doctype html>
<html>
<head>
    <meta charset="UTF-8">	
	<link rel="shortcut icon" href="#"/>
    <title>Upload</title>
</head>
<body>
  <div id="result">login success!</div>
</body>
</html>`
)

func TestHttpBruteForce(t *testing.T) {
	base := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			_ = r.ParseForm()
			username := r.PostForm.Get("username")
			password := r.PostForm.Get("password")
			fmt.Println(username, password)
			if username == "admin" && password == "admin" {
				w.Header().Add("Location", "result")
				w.WriteHeader(302)
				_, _ = w.Write([]byte{})
			} else {
				_, _ = w.Write([]byte(LoginHTML))
			}
		} else if r.URL.Path == "/login" {
			_, _ = w.Write([]byte(LoginHTML))
		} else if r.URL.Path == "/result" {
			_, _ = w.Write([]byte(ResultHTML))
		} else {
			if strings.Contains(r.URL.Path, ".ico") {
				return
			}
			w.Header().Add("Location", "login")
			w.WriteHeader(302)
			_, _ = w.Write([]byte{})
		}
	}))
	defer base.Close()
	time.Sleep(time.Second)
	opts := []BruteConfigOpt{
		WithExePath(`/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`),
		//WithCaptchaUrl(`http://192.168.3.20:8008/runtime/text/invoke`),
		//WithCaptchaMode(`common_arithmetic`),
		WithUsernameList("admin"),
		WithPasswordList("admin", "luckyadmin123"),
		WithExtraWaitLoadTime(1000),
		//WithUsernameSelector("#loginpage > div > div > form > div:nth-child(2) > div > div > input"),
		//WithPasswordSelector("#loginpage > div > div > form > div:nth-child(3) > div > div > input"),
		//WithCaptchaSelector("#loginpage > div > div > form > div:nth-child(4) > div > div > input"),
	}
	log.SetLevel(log.DebugLevel)
	ch, err := HttpBruteForce(base.URL, opts...)
	//ch, err := HttpBruteForce("http://192.168.3.20/#/login", opts...)
	if err != nil {
		t.Error(err)
	}
	for item := range ch {
		t.Logf(`[bruteforce] %s:%s login %v`, item.Username(), item.Password(), item.Status())
		t.Logf(`[bruteforce] login generated info: %v`, item.Info())
		if item.Status() == true {
			t.Logf(`after login url: %v`, item.LoginSuccessUrl())
			t.Log(item.Base64())
		}
	}
}

func TestElementScan(t *testing.T) {
	test := assert.New(t)
	base := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(loginHTML))
	}))
	defer base.Close()
	time.Sleep(time.Second)
	bruteForce, err := NewHttpBruteForceCore(base.URL)
	if err != nil {
		t.Error(err)
		return
	}
	err = bruteForce.init()
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		err := bruteForce.starter.Close()
		if err != nil {
			log.Errorf(`browser close error: %v`, err.Error())
		}
	}()
	err = bruteForce.elementDetect()
	if err != nil {
		t.Error(err)
		return
	}
	test.Equal(bruteForce.UsernameSelector, "#loginpage>div:nth-child(1)>div:nth-child(1)>form:nth-child(2)>div:nth-child(2)>div:nth-child(1)>div:nth-child(1)>input:nth-child(1)")
	test.Equal(bruteForce.PasswordSelector, "#loginpage>div:nth-child(1)>div:nth-child(1)>form:nth-child(2)>div:nth-child(3)>div:nth-child(1)>div:nth-child(1)>input:nth-child(1)")
	test.Equal(bruteForce.CaptchaSelector, "#loginpage>div:nth-child(1)>div:nth-child(1)>form:nth-child(2)>div:nth-child(4)>div:nth-child(1)>div:nth-child(1)>input:nth-child(1)")
	test.Equal(bruteForce.CaptchaImgSelector, "#code")
	test.Equal(bruteForce.LoginButtonSelector, "#loginpage>div:nth-child(1)>div:nth-child(1)>form:nth-child(2)>div:nth-child(5)>div:nth-child(1)>button:nth-child(1)")
}
