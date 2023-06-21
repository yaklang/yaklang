# 目录
- [自动化爆破说明](#brute)
- [simulator.simple](#simulator.simple)
- [httpbrute 基于模拟点击的http自动化爆破使用说明](#httpbrute)
  - [代码实例](#httpbrute_1)
  - [数据接口](#httpbrute_2)
    - [httpbrute.BruteResult](#BruteResult)
  - [API](#httpbrute_3)
    - [httpbrute.httpBruteForce](#httpBruteForce)
    - [httpbrute.username](#username)
    - [httpbrute.usernameList](#usernameList)
    - [httpbrute.password](#password)
    - [httpbrute.passwordList](#passwordList)
    - [httpbrute.wsAddress](#wsAddress)
    - [httpbrute.proxy](#proxy)
    - [httpbrute.captchaUrl](#captchaUrl)
    - [httpbrute.captchaMode](#captchaMode)
    - [httpbrute.usernameSelector](#usernameSelector)
    - [httpbrute.passwordSelector](#passwordSelector)
    - [httpbrute.captchaInputSelector](#captchaInputSelector)
    - [httpbrute.captchaImgSelector](#captchaImgSelector)
    - [httpbrute.submitButtonSelector](#submitButtonSelector)
    - [httpbrute.loginDetectMode](#loginDetectMode)

# simulator模块使用说明：

# <a id="brute">~~自动化爆破说明~~</a>
下版本将会删除该自动化爆破，新的模拟点击http爆破见[下方](#httpbrute)

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


# <a id="simulator.simple">simulator.simple</a>

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

# <a id="httpbrute">httpbrute 基于模拟点击的http自动化爆破使用说明</a>

## <a id="httpbrute_1">代码实例</a>

    urlStr = "http://192.168.0.100/#/login"
    captchaUrl = "http://192.168.0.200:9898/ocr/b64/json"
    
    opts = [
        bruteforce.captchaUrl(captchaUrl),
        bruteforce.username("admin"),
        bruteforce.password("admin", "123321"),
    ]
    
    ch, err = bruteforce.httpBruteForce(urlStr, opts...)
    for item := range ch {
    yakit.Info(`[bruteforce] %s:%s login %v with info: %s`, item.Username(), item.Password(), item.Status(), item.Info())

## <a id="httpbrute_2">Data Struct</a>

### <a id="BruteResult">httpbrute.BruteResult</a>

爆破结果数据结构

#### struct

    type BruteResult interface {
        PtrStructMethods(指针结构方法/函数):
            func Username() return (string)
            func Password() return (string)
            func Status() return (bool)
        
            func Info() return (string)
            func Base64() return (string)
    }

#### method

`func (*BruteResult) Username() return (r0: string)` 爆破测试的用户名

`func (*BruteResult) Password() return (r0: string)` 爆破测试的密码

`func (*BruteResult) Status() return (r0: bool)` 本次爆破是否成功

`func (*BruteResult) Info() return (r0: string)` 本次爆破过程的部分信息

`func (*BruteResult) Base64() return (r0: string)` 爆破成功时浏览器页面截图的base64编码

## <a id="httpbrute_3">API</a>

### <a id="httpBruteForce">httpbrute.httpBruteForce</a>

设置爆破参数 开始爆破任务

#### 定义

`httpbrute.httpBruteForce(url string, opts ...httpbrute.BruteConfigOpt) return (ch: chan httpbrute.BruteResult, err: error)`

#### 参数

| 参数名  | 参数类型                        | 参数解释 |
|------|-----------------------------|------|
| url  | string                      | 爆破目标 |
| opts | ...httpbrute.BruteConfigOpt | 爆破参数 |

#### 返回值

| 返回值 | 返回值类型                      | 返回值解释         |
|-----|----------------------------|---------------|
| ch  | chan httpbrute.BruteResult | 爆破结果传递channel |
| err | error                      | 错误信息          |

### <a id="username">httpbrute.username</a>

设置爆破的用户名

#### 定义

`httpbrute.username(username ...string) return (r0: httpbrute.BruteConfigOpt)`

#### 参数

| 参数名      | 参数类型      | 参数解释    |
|----------|-----------|---------|
| username | ...string | 待爆破的用户名 |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | httpbrute.BruteConfigOpt | 参数设置函数 |

### <a id="usernameList">httpbrute.usernameList</a>

设置爆破的用户名

#### 定义

`httpbrute.usernameList(usernameList []string) return (r0: httpbrute.BruteConfigOpt)`

#### 参数

| 参数名          | 参数类型     | 参数解释      |
|--------------|----------|-----------|
| usernameList | []string | 待爆破的用户名切片 |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | httpbrute.BruteConfigOpt | 参数设置函数 |

### <a id="password">httpbrute.password</a>

设置爆破的密码

#### 定义

`httpbrute.password(password ...string) return (r0: httpbrute.BruteConfigOpt)`

#### 参数

| 参数名      | 参数类型      | 参数解释   |
|----------|-----------|--------|
| password | ...string | 待爆破的密码 |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | httpbrute.BruteConfigOpt | 参数设置函数 |

### <a id="passwordList">httpbrute.passwordList</a>

设置爆破的密码

#### 定义

`httpbrute.passwordList(passwordList []string) return (r0: httpbrute.BruteConfigOpt)`

#### 参数

| 参数名          | 参数类型     | 参数解释     |
|--------------|----------|----------|
| passwordList | []string | 待爆破的密码切片 |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | httpbrute.BruteConfigOpt | 参数设置函数 |

### <a id="wsAddress">httpbrute.wsAddress</a>

设置浏览器的ws地址

#### 定义

`httpbrute.wsAddress(wsAddress string) return (r0: httpbrute.BruteConfigOpt)`

#### 参数

| 参数名       | 参数类型   | 参数解释     |
|-----------|--------|----------|
| wsAddress | string | 浏览器的ws地址 |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | httpbrute.BruteConfigOpt | 参数设置函数 |

### <a id="proxy">httpbrute.proxy</a>

设置浏览器代理

#### 定义

`httpbrute.proxy(proxy string, details ...string) return (r0: httpbrute.BruteConfigOpt)`

#### 参数

| 参数名     | 参数类型      | 参数解释              |
|---------|-----------|-------------------|
| proxy   | string    | 浏览器代理地址           |
| details | ...string | 代理的用户名和密码（如果有则填写） |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | httpbrute.BruteConfigOpt | 参数设置函数 |

### <a id="captchaUrl">httpbrute.captchaUrl</a>

设置验证码图片识别链接

#### 定义

`httpbrute.captchaUrl(captchaUrl string) return (r0: httpbrute.BruteConfigOpt)`

默认的验证码数据接口匹配使用ddddocr的ocr_api_server项目：[ocr_api_server](https://github.com/sml2h3/ocr_api_server)

这里默认操作类型ocr，数据类型b64，返回类型json，所以使用该项目时默认接口为：http://{host}:{port}/ocr/b64/json

#### 参数

| 参数名        | 参数类型      | 参数解释    |
|------------|-----------|---------|
| captchaUrl | string    | 浏览器代理地址 |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | httpbrute.BruteConfigOpt | 参数设置函数 |

### <a id="captchaMode">httpbrute.captchaMode</a>

设置验证码图片识别模式（可选）

<font size=5><b>一般情况下不会使用该接口</b></font>

#### 定义

`httpbrute.captchaMode(captchaMode string) return (r0: httpbrute.BruteConfigOpt)`

#### 参数

| 参数名         | 参数类型      | 参数解释    |
|-------------|-----------|---------|
| captchaMode | string    | 验证码识别模式 |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | httpbrute.BruteConfigOpt | 参数设置函数 |

### <a id="usernameSelector">httpbrute.usernameSelector</a>

设置输入用户名的element的selector

#### 定义

`httpbrute.usernameSelector(usernameSelector string) return (r0: httpbrute.BruteConfigOpt)`

#### 参数

| 参数名              | 参数类型      | 参数解释                |
|------------------|-----------|---------------------|
| usernameSelector | string    | 用户名element的selector |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | httpbrute.BruteConfigOpt | 参数设置函数 |

### <a id="passwordSelector">httpbrute.passwordSelector</a>

设置输入密码的element的selector

#### 定义

`httpbrute.passwordSelector(passwordSelector string) return (r0: httpbrute.BruteConfigOpt)`

#### 参数

| 参数名              | 参数类型      | 参数解释               |
|------------------|-----------|--------------------|
| passwordSelector | string    | 密码element的selector |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | httpbrute.BruteConfigOpt | 参数设置函数 |

### <a id="captchaInputSelector">httpbrute.captchaInputSelector</a>

设置输入验证码的element的selector

#### 定义

`httpbrute.captchaInputSelector(captchaSelector string) return (r0: httpbrute.BruteConfigOpt)`

#### 参数

| 参数名             | 参数类型   | 参数解释                |
|-----------------|--------|---------------------|
| captchaSelector | string | 验证码element的selector |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | httpbrute.BruteConfigOpt | 参数设置函数 |

### <a id="captchaImgSelector">httpbrute.captchaImgSelector</a>

设置验证码图片的element的selector

#### 定义

`httpbrute.captchaImgSelector(captchaImgSelector string) return (r0: httpbrute.BruteConfigOpt)`

#### 参数

| 参数名                | 参数类型   | 参数解释                  |
|--------------------|--------|-----------------------|
| captchaImgSelector | string | 验证码图片element的selector |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | httpbrute.BruteConfigOpt | 参数设置函数 |

### <a id="submitButtonSelector">httpbrute.submitButtonSelector</a>

设置提交请求按钮对应element的selector

#### 定义

`httpbrute.submitButtonSelector(buttonSelector string) return (r0: httpbrute.BruteConfigOpt)`

#### 参数

| 参数名             | 参数类型   | 参数解释                   |
|-----------------|--------|------------------------|
| buttonSelector  | string | 提交请求按钮element的selector |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | httpbrute.BruteConfigOpt | 参数设置函数 |

### <a id="loginDetectMode">httpbrute.loginDetectMode</a>

设置确认成功登陆的检测类型

#### 定义

`httpbrute.loginDetectMode(detectMode loginDetectMode, degree ...float64) return (r0: httpbrute.BruteConfigOpt)`

#### 参数

| 参数名        | 参数类型            | 参数解释        |
|------------|-----------------|-------------|
| detectMode | loginDetectMode | 确认成功登陆的检测类型 |
| degree     | ...float64      | 附加参数        |

loginDetectMode包括三种：
- `httpbrute.urlChangeMode` 通过url变化判断是否登陆成功
- `httpbrute.htmlChangeMode` 通过html页面的变化程度判断是否登陆成功
- `httpbrute.defaultChangeMode` 同时使用以上两种判断方法，两种方法都通过才确定登陆成功（默认）

当使用了html页面变化程度进行判断时，可以通过degree设置判断相似程度的阈值，值越小表示尝试登陆后的页面相似程度越小，默认为0.6

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | httpbrute.BruteConfigOpt | 参数设置函数 |