# simulator模块使用说明：

# 自动化爆破说明

## 爆破输入

`BruteForceResult, err = simulator.defaultBrute(targetUrl, ...ConfigOpt)`

对目标url进行爆破 返回爆破结果和错误信息

爆破模块相关参数通过ConfigOpt进行输入：

- `configOpt = simulator.captchaUrl(captchaUrl string)` 设置验证码识别url

默认的验证码数据接口匹配使用ddddocr的ocr_api_server项目：[ocr_api_server](https://github.com/sml2h3/ocr_api_server)

- `configOpt = simulator.usernameList(usernameList []string)` 设置爆破的用户名列表

- `configOpt = simulator.passwordList(passwordList []string)` 设置爆破的密码列表

- `configOpt = simulator.wsAddress(wsAddress string)` 设置远端chrome浏览器的ws地址

- `configOpt = simulator.proxy(proxy string)` 设置浏览器代理地址

- `configOpt = simulator.proxyDetails(proxy, username, password string)` 设置浏览器代理地址和代理的用户名密码

- `configOpt = simulator.usernameSelector(selector string)` 用户指定登陆名称输入框的selector

- `configOpt = simulator.passwordSelector(selector string)` 用户指定登陆密码输入框的selector

- `configOpt = simulator.captchaInputSelector(selector string)` 用户指定验证码输入框的selector

- `configOpt = simulator.captchaImgSelector(selector string)` 用户指定验证码图片的selector

- `configOpt = simulator.submitButtonSelector(selector string)` 用户指定登陆提交按钮的selector


## 爆破输出

BruteForceResult包括如下输出：

- `string = BruteForceResult.Username()` 爆破成功的用户名，失败为空

- `string = BruteForceResult.Password()` 爆破成功的密码，失败为空

- `string = BruteForceResult.Cookie()` 爆破成功的cookie，失败为空

- `string = BruteForceResult.LoginPngB64()` 爆破成功页面截图的base64编码，失败为空

- `[]string = BruteForceResult.Log()` 爆破过程中的日志信息，字符串列表格式

# 自动化爆破示例

    url = "http://192.168.0.58/#/login"
    userlist = ["admin"]
    passlist = ["luckyadmin123"]
    userOpt = simulator.usernameList(userlist)
    passOpt = simulator.passwordList(passlist)
    // captchaUrl = simulator.captchaUrl("http://192.168.0.115:9898/ocr/b64/json")
    // chromeAddress = simulator.wsAddress("http://192.168.0.115:7317/")
    result, err = simulator.defaultBrute(url, userOpt, passOpt, scanMode)
    
    println(result.Username())
    println(result.Password())
    println(result.Cookie())
    println(result.Log())


# simulator.simple

新增simulator.simple接口

相比于之前复杂的操作，对接口进行简化

## api

### func

`simulator.simple.createBrowser(opts ...BrowserConfigOpt) *Browser` 根据参数创建浏览器，并完成初始化

其中 浏览器的参数设置如下：

`simulator.simple.wsAddress(wsAddress string) BrowserConfigOpt` 设置远程浏览器地址

`simulator.simple.proxy(proxy string, proxyInfo ...string) BrowserConfigOpt` 设置浏览器代理
- proxy为代理地址，必填
- proxyInfo为代理的用户名和密码，选填

`simulator.simple.noSandBox(bool) BrowserConfigOpt` 设置no-sandbox

`simulator.simple.headless(bool) BrowserConfigOpt` 设置headless

`simulator.simple.requestModify(url string, modifyTarget ModifyTarget, mofidyResult interface{}) BrowserConfigOpt` 设置需要修改的request包
- url为需要修改包内容对应的请求url中的关键字，支持正则
- modifyTarget为修改的位置 包括：
  - `simulator.simple.headersModifyTarget` 对请求包的headers进行添加操作
  - `simulator.simple.hostModifyTarget` 对请求包的host进行修改操作
  - `simulator.simple.bodyModifyTarget` 对请求包的body进行直接修改操作
- modifyResult为修改的具体内容，内容的结构会随着modifyTarget的不同而不同：
  - 对请求包的headers进行添加操作时，modifyResult可以是[]string结构，也可以是map[string]string结构。
    - 当modifyResult结构是[]string时，切片从头开始两个字符串为一组，一组中第一个字符串为headers的key，第二个字符串为headers的value，例如[]string{"testHeaders", "testValue"}
    - 当modifyResult结构是map[string]string时，map结构中的key为headers的key，结构中的value为headers的value，例如map[string]string{"testHeaders":"testValue"}
  - 对请求包的host进行修改操作时，modifyResult为string类型，内容是所要修改成为的host
  - 对请求包的body进行直接修改操作时，modifyResult为string类型，内容是所要修改成为的body

`simulator.simple.responseModify(url string, modifyTarget ModifyTarget, mofidyResult interface{}) BrowserConfigOpt` 设置需要修改的response包
- url为需要修改包内容对应的请求url中的关键字，支持正则
- modifyTarget为修改的位置 包括：
    - `simulator.simple.headersModifyTarget` 对响应包的headers进行添加操作
    - `simulator.simple.bodyModifyTarget` 对响应包的body进行直接修改操作
    - `simulator.simple.bodyReplaceTarget` 对响应包的body进行替换操作
- modifyResult为修改的具体内容，内容的结构会随着modifyTarget的不同而不同：
  - 对响应包的headers进行添加操作时，modifyResult结构同请求包中headers进行添加操作时的modifyResult的结构
  - 对响应包的body进行直接修改操作时，modifyResult为string类型，内容是所要修改成为的body
  - 对响应包的body进行替换操作时，modifyResult为[]string类型，切片从头开始两个字符串为一组，一组中第一个字符串为body中待替换的字符串，第二个字符串为其对应被替换成的字符串

### type Browser func

`func (b *Browser) Navigate(urlStr string) *Page` 创建一个页面并访问指定的url，返回该页面

### type Page func

`func (p *Page) Navigate(urlStr string)` 使该页面访问指定url

`func (p *Page) Click(selector string) error` 点击页面中selector对应元素

`func (p *Page) Input(selector, inputStr string) error` 在页面中的selector对应元素中输入参数

`func (p *Page) HTML() (string, error)` 返回当前页面的HTML代码

`func (p *Page) ScreenShot() (string, error)` 返回当前页面截图的base64编码

## simulator.simple Yak代码示例

    replaceStr = []string{"0","1"}
    replaceModify = simulator.simple.responseModify("uapws/login.ajax", simulator.simple.bodyReplaceTarget, replaceStr)
    headless = simulator.simple.headless(false)
    browser = simulator.simple.createBrowser(headless, replaceModify)
    page = browser.Navigate("http://192.168.0.111:8099/uapws/")
    page.Input("#password", "123321")
    page.Click("#dijit_form_Button_0_label")
    time.Sleep(2)
