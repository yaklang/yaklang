// Package simulator
// @Author bcy2007  2023/8/17 16:21
package simulator

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

var header = `<head><meta charset="utf-8"><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="icon" href="/favicon.ico"><title>自动化渗透测试平台</title><script defer="defer" src="/static/js/chunk-vendors.6af4a237.js"></script><script defer="defer" src="/static/js/app.1b613dc4.js"></script><link href="/static/css/app.8dd30727.css" rel="stylesheet"><style type="text/css">.jv-node{position:relative}.jv-node:after{content:','}.jv-node:last-of-type:after{content:''}.jv-node.toggle{margin-left:13px !important}.jv-node .jv-node{margin-left:25px}
</style><style type="text/css">.jv-container{box-sizing:border-box;position:relative}.jv-container.boxed{border:1px solid #eee;border-radius:6px}.jv-container.boxed:hover{box-shadow:0 2px 7px rgba(0,0,0,0.15);border-color:transparent;position:relative}.jv-container.jv-light{background:#fff;white-space:nowrap;color:#525252;font-size:14px;font-family:Consolas, Menlo, Courier, monospace}.jv-container.jv-light .jv-ellipsis{color:#999;background-color:#eee;display:inline-block;line-height:0.9;font-size:0.9em;padding:0px 4px 2px 4px;margin:0 4px;border-radius:3px;vertical-align:2px;cursor:pointer;-webkit-user-select:none;user-select:none}.jv-container.jv-light .jv-button{color:#49b3ff}.jv-container.jv-light .jv-key{color:#111111;margin-right:4px}.jv-container.jv-light .jv-item.jv-array{color:#111111}.jv-container.jv-light .jv-item.jv-boolean{color:#fc1e70}.jv-container.jv-light .jv-item.jv-function{color:#067bca}.jv-container.jv-light .jv-item.jv-number{color:#fc1e70}.jv-container.jv-light .jv-item.jv-object{color:#111111}.jv-container.jv-light .jv-item.jv-undefined{color:#e08331}.jv-container.jv-light .jv-item.jv-string{color:#42b983;word-break:break-word;white-space:normal}.jv-container.jv-light .jv-item.jv-string .jv-link{color:#0366d6}.jv-container.jv-light .jv-code .jv-toggle:before{padding:0px 2px;border-radius:2px}.jv-container.jv-light .jv-code .jv-toggle:hover:before{background:#eee}.jv-container .jv-code{overflow:hidden;padding:30px 20px}.jv-container .jv-code.boxed{max-height:300px}.jv-container .jv-code.open{max-height:initial !important;overflow:visible;overflow-x:auto;padding-bottom:45px}.jv-container .jv-toggle{background-image:url(data:image/svg+xml;base64,PHN2ZyBoZWlnaHQ9IjE2IiB3aWR0aD0iOCIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KIAo8cG9seWdvbiBwb2ludHM9IjAsMCA4LDggMCwxNiIKc3R5bGU9ImZpbGw6IzY2NjtzdHJva2U6cHVycGxlO3N0cm9rZS13aWR0aDowIiAvPgo8L3N2Zz4=);background-repeat:no-repeat;background-size:contain;background-position:center center;cursor:pointer;width:10px;height:10px;margin-right:2px;display:inline-block;-webkit-transition:-webkit-transform 0.1s;transition:-webkit-transform 0.1s;transition:transform 0.1s;transition:transform 0.1s, -webkit-transform 0.1s}.jv-container .jv-toggle.open{-webkit-transform:rotate(90deg);transform:rotate(90deg)}.jv-container .jv-more{position:absolute;z-index:1;bottom:0;left:0;right:0;height:40px;width:100%;text-align:center;cursor:pointer}.jv-container .jv-more .jv-toggle{position:relative;top:40%;z-index:2;color:#888;-webkit-transition:all 0.1s;transition:all 0.1s;-webkit-transform:rotate(90deg);transform:rotate(90deg)}.jv-container .jv-more .jv-toggle.open{-webkit-transform:rotate(-90deg);transform:rotate(-90deg)}.jv-container .jv-more:after{content:"";width:100%;height:100%;position:absolute;bottom:0;left:0;z-index:1;background:-webkit-linear-gradient(top, rgba(0,0,0,0) 20%, rgba(230,230,230,0.3) 100%);background:linear-gradient(to bottom, rgba(0,0,0,0) 20%, rgba(230,230,230,0.3) 100%);-webkit-transition:all 0.1s;transition:all 0.1s}.jv-container .jv-more:hover .jv-toggle{top:50%;color:#111}.jv-container .jv-more:hover:after{background:-webkit-linear-gradient(top, rgba(0,0,0,0) 20%, rgba(230,230,230,0.3) 100%);background:linear-gradient(to bottom, rgba(0,0,0,0) 20%, rgba(230,230,230,0.3) 100%)}.jv-container .jv-button{position:relative;cursor:pointer;display:inline-block;padding:5px;z-index:5}.jv-container .jv-button.copied{opacity:0.4;cursor:default}.jv-container .jv-tooltip{position:absolute}.jv-container .jv-tooltip.right{right:15px}.jv-container .jv-tooltip.left{left:15px}.jv-container .j-icon{font-size:12px}
</style><link rel="stylesheet" type="text/css" href="/static/css/8552.9d3000ae.css"></head>`

var body = `<body><noscript><strong>We're sorry but xiaozhi_vue_standard doesn't work properly without JavaScript enabled. Please enable it to continue.</strong></noscript><div data-v-ab3e8446="" id="app"><div data-v-0e18988b="" data-v-ab3e8446="" id="loginpage" class="loginpage"><div data-v-0e18988b="" class="login-bg"><div data-v-0e18988b=""><div data-v-0e18988b="" class="img"><img data-v-0e18988b="" src="/static/img/login.2a157146.png"></div><form data-v-0e18988b="" class="el-form demo-ruleForm login-container el-form--label-left"><h3 data-v-0e18988b="" class="title">欢迎登录</h3><!----><div data-v-0e18988b="" class="el-form-item is-required"><!----><div class="el-form-item__content" style="margin-left: 0px;"><div data-v-0e18988b="" class="el-input"><!----><input type="text" autocomplete="off" placeholder="请输入用户名" class="el-input__inner"><!----><!----><!----><!----></div><!----></div></div><div data-v-0e18988b="" class="el-form-item is-required"><!----><div class="el-form-item__content" style="margin-left: 0px;"><div data-v-0e18988b="" class="el-input"><!----><input type="password" autocomplete="off" placeholder="请输入密码" class="el-input__inner"><!----><!----><!----><!----></div><!----></div></div><div data-v-0e18988b="" class="el-form-item"><!----><div class="el-form-item__content" style="margin-left: 0px;"><div data-v-0e18988b="" class="el-input" style="width: 180px;"><!----><input type="text" autocomplete="off" placeholder="请输入验证码" class="el-input__inner"><!----><!----><!----><!----></div><img data-v-0e18988b="" src="data:image/png;base64,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" id="code" alt="验证码" class="logincode"><!----></div></div><div data-v-0e18988b="" class="el-form-item" style="width: 100%;"><!----><div class="el-form-item__content" style="margin-left: 0px;"><button data-v-0e18988b="" type="button" class="el-button el-button--primary" style="width: 100%;"><!----><!----><span>登录</span></button><!----></div></div></form></div></div></div></div></body>`

func TestElementFind(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := "<html>" + header + body + "</html>"
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()
	starter := CreateNewStarter()
	err := starter.Start()
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		_ = starter.Close()
	}()
	page, err := starter.CreatePage()
	if err != nil {
		t.Error(err)
		return
	}
	err = page.Navigate(server.URL)
	if err != nil {
		t.Error(err)
		return
	}
	err = page.WaitLoad()
	if err != nil {
		t.Error(err)
		return
	}
	searchInfo := map[string]map[string][]string{
		"input": {
			"type": {
				"text",
				"password",
				"number",
				"tel",
			},
		},
	}
	elements, err := customizedGetElement(page, searchInfo)
	if err != nil {
		t.Error(err)
		return
	}
	//t.Log(ElementsToSelectors(elements...))
	//t.Log(ElementsToIds(elements...))
	tags := []string{"username", "password", "captcha"}
	result, err := CalculateRelevanceMatrix(elements, tags)
	if err != nil {
		t.Error(err)
	}
	t.Log(result)
}
