# CrawlerX 爬虫模块使用说明

## 目录

- [Example](#example)
- [Data Structure](#data-structure)
    - [crawlerx.ReqInfo](#crawlerx-reqinfo)
- [API](#api)
    - [crawlerx.StartCrawler](#crawlerx-startcrawler)
    - [crawlerx.PageScreenShot](#crawlerx-pagescreenshot)
    - [crawlerx.browserInfo](#crawlerx-browserinfo)
    - [crawlerx.maxUrl](#crawlerx-maxurl)
    - [crawlerx.maxDepth](#crawlerx-maxdepth)
    - [crawlerx.concurrent](#crawlerx-concurrent)
    - [crawlerx.blacklist](#crawlerx-blacklist)
    - [crawlerx.whitelist](#crawlerx-whitelist)
    - [crawlerx.pageTimeout](#crawlerx-pagetimeout)
    - [crawlerx.fullTimeout](#crawlerx-fulltimeout)
    - [crawlerx.extraWaitLoadTime](#crawlerx-extrawaitloadtime)
    - [crawlerx.formFill](#crawlerx-formfill)
    - [crawlerx.fileInput](#crawlerx-fileinput)
    - [crawlerx.headers](#crawlerx-headers)
    - [crawlerx.rawHeaders](#crawlerx-rawheaders)
    - [crawlerx.cookies](#crawlerx-cookies)
    - [crawlerx.rawCookie](#crawlerx-rawcookie)
    - [crawlerx.scanRangeLevel](#crawlerx-scanrangelevel)
    - [crawlerx.scanRepeatLevel](#crawlerx-scanrepeatlevel)
    - [crawlerx.ignoreQueryName](#crawlerx-ignorequeryname)
    - [crawlerx.sensitiveWords](#crawlerx-sensitivewords)
    - [crawlerx.leakless](#crawlerx-leakless)
    - [crawlerx.localStorage](#crawlerx-localstorage)
    - [crawlerx.invalidSuffix](#crawlerx-invalidsuffix)
    - [crawlerx.stealth](#crawlerx-stealth)
    - [crawlerx.runtimeID](#crawlerx-runtimeid)
    - [crawlerx.evalJs](#crawlerx-evaljs)
    - [crawlerx.jsResultSend](#crawlerx-jsresultsend)
    - [crawlerx.vue](#crawlerx-vue)
    - [crawlerx.response](#crawlerx-response)

## <span id="example">Example</span>

    yakit.AutoInitYakit()

    targetUrl = cli.String("targetUrl")
    wsAddress = cli.String("wsAddress")
    exePath = cli.String("exePath")
    proxy = cli.String("proxy")
    proxyUsername = cli.String("proxyUsername")
    proxyPassword = cli.String("proxyPassword")
    pageTimeout = cli.Int("pageTimeout")
    fullTimeout = cli.Int("fullTimeout")
    formFill = cli.String("formFill")
    fileUpload = cli.String("fileUpload")
    header = cli.String("header")
    cookie = cli.String("cookie")
    scanRange = cli.String("scanRange")
    scanRepeat = cli.String("scanRepeat")
    maxUrl = cli.Int("maxUrl")
    maxDepth = cli.Int("maxDepth")
    ignoreQuery = cli.String("ignoreQuery")
    extraWaitLoad = cli.Int("extraWaitLoad")
    
    blacklist = cli.String("blacklist")
    whitelist = cli.String("whitelist")
    sensitiveWords = cli.String("sensitiveWords")
    leakless = cli.String("leakless", cli.setDefault("default"))
    concurrent = cli.Int("concurrent", cli.setDefault(3))
    rawHeaders = cli.String("rawHeaders")
    rawCookie = cli.String("rawCookie")
    
    func stringToDict(tempStr) {
        result = make(map[string]string, 0)
        items = tempStr.Split(";")
        for _, item := range items {
            if item.Contains(":") {
                kv := item.Split(":")
                result[kv[0]] = kv[1]
            }
        }
        return result
    }
    
    scanRangeMap = {
        "AllDomainScan": crawlerx.AllDomainScan,
        "SubMenuScan": crawlerx.SubMenuScan,
    }
    
    scanRepeatMap = {
        "UnLimitRepeat": crawlerx.UnLimitRepeat,
        "LowRepeatLevel": crawlerx.LowRepeatLevel,
        "MediumRepeatLevel": crawlerx.MediumRepeatLevel,
        "HighRepeatLevel": crawlerx.HighRepeatLevel,
        "ExtremeRepeatLevel": crawlerx.ExtremeRepeatLevel,
    }
    
    browserInfo = {
        "ws_address":"",
        "exe_path":"",
        "proxy_address":"",
        "proxy_username":"",
        "proxy_password":"",
    }
    if wsAddress != "" {
        browserInfo["ws_address"] = wsAddress
    }
    if exePath != "" {
        browserInfo["exe_path"] = exePath
    }
    if proxy != "" {
        browserInfo["proxy_address"] = proxy
        if proxyUsername != "" {
            browserInfo["proxy_username"] = proxyUsername
            browserInfo["proxy_password"] = proxyPassword
        }
    }
    browserInfoOpt = crawlerx.browserInfo(json.dumps(browserInfo))
    
    pageTimeoutOpt = crawlerx.pageTimeout(pageTimeout)
    
    fullTimeoutOpt = crawlerx.fullTimeout(fullTimeout)
    
    concurrentOpt = crawlerx.concurrent(concurrent)
    
    opts = [
        browserInfoOpt,
        pageTimeoutOpt,
        fullTimeoutOpt,
        concurrentOpt,
    ]
    
    if formFill != "" {
        formFillInfo = stringToDict(formFill)
        formFillOpt = crawlerx.formFill(formFillInfo)
        opts = append(opts, formFillOpt)
    }
    
    if fileUpload != "" {
        fileUploadInfo = stringToDict(fileUpload)
        fileUploadOpt = crawlerx.fileInput(fileUploadInfo)
        opts = append(opts, fileUploadOpt)
    }
    
    if header != "" {
        headerInfo = stringToDict(header)
        headerOpt = crawlerx.headers(headerInfo)
        opts = append(opts, headerOpt)
    }
    
    if rawHeaders != "" {
        opts = append(opts, crawlerx.rawHeaders(rawHeaders))
    }
    
    if rawCookie != "" {
        opts = append(opts, crawlerx.rawCookie(rawCookie))
    }
    
    if cookie != "" {
        cookieInfo = stringToDict(cookie)
        cookieOpt = crawlerx.cookies(cookieInfo)
        opts = append(opts, cookieOpt)
    }
    
    if scanRange != "" {
        scanRangeItem = scanRangeMap[scanRange]
        scanRangeOpt = crawlerx.scanRangeLevel(scanRangeItem)
        opts = append(opts, scanRangeOpt)
    }
    
    if scanRepeat != "" {
        scanRepeatItem = scanRepeatMap[scanRepeat]
        scanRepeatOpt = crawlerx.scanRepeatLevel(scanRepeatItem)
        opts = append(opts, scanRepeatOpt)
    }
    
    if maxUrl != 0 {
        opts = append(opts, crawlerx.maxUrl(maxUrl))
    }
    
    if maxDepth != 0 {
        opts = append(opts, crawlerx.maxDepth(maxDepth))
    }
    
    if extraWaitLoad != 0 {
        opts = append(opts, crawlerx.extraWaitLoadTime(extraWaitLoad))
    }
    
    if ignoreQuery != "" {
        queries = ignoreQuery.Split(",")
        opts = append(opts, crawlerx.ignoreQueryName(queries...))
    }
    
    if blacklist != "" {
        opts = append(opts, crawlerx.blacklist(blacklist.Split(",")...))
    }
    
    if whitelist != "" {
        opts = append(opts, crawlerx.whitelist(whitelist.Split(",")...))
    }
    
    if sensitiveWords != "" {
        opts = append(opts, crawlerx.sensitiveWords(sensitiveWords.Split(",")))
    }
    
    if leakless != "" {
        opts = append(opts, crawlerx.leakless(leakless))
    }
    
    ch, err = crawlerx.StartCrawler(targetUrl, opts...)
    for item = range ch{
        yakit.Info(item.Method() + " " + item.Url())
    }


## <span id="data-structure">Data Structure</span>

### <span id="crawlerx-reqinfo">crawlerx.ReqInfo</span>

爬虫结果数据结构

#### struct

    type ReqInfo interface {
        PtrStructMethods(指针结构方法/函数):
            func Url() return(string)
            func Method() return(string)
    
            func RequestHeaders() return(map[string]string)
            func RequestBody() return(string)
    
            func StatusCode() return(int)
            func ResponseHeaders() return(map[string]string)
            func ResponseBody() return(string)
    }

#### methods

`func (*ReqInfo) Url() return(r0: string)` 爬虫结果的url

`func (*ReqInfo) Method() return(string)` 爬虫结果的请求方法

`func (*ReqInfo) RequestHeaders() return(map[string]string)` 爬虫结果的请求包头文件

`func (*ReqInfo) RequestBody() return(string)` 爬虫结果的请求包body

`func (*ReqInfo) RequestRaw() return([]byte, error)` 爬虫结果的原生请求包

`func (*ReqInfo) StatusCode() return(int)` 爬虫结果的返回包状态码

`func (*ReqInfo) ResponseHeaders() return(map[string]string)` 爬虫结果的返回包头文件

`func (*ReqInfo) ResponseBody() return(string)` 爬虫结果的返回包body

`func (*ReqInfo) Type() return(string)` 爬虫结果的类型 如hijack_result/event url/js url/file upload result

`func (*ReqInfo) From() return(string)` 该url来源链接，即从哪个链接得到的该链接

## <span id="api">API</span>

### <span id="crawlerx-startcrawler">crawlerx.StartCrawler</span>

设置爬虫参数 开始爬虫任务

#### 定义

`func crawlerx.StartCrawler(url: string, opts: ...crawlerx.ConfigOpt) return (ch: chan crawlerx.ReqInfo, err: error)`

#### 参数

| 参数名  | 参数类型                  | 参数解释 |
|------|-----------------------|------|
| url  | string                | 渗透目标 |
| opts | ...crawlerx.ConfigOpt | 扫描参数 |

#### 返回值

| 返回值 | 返回值类型                 | 返回值解释         |
|-----|-----------------------|---------------|
| ch  | chan crawlerx.ReqInfo | 爬虫结果传递channel |
| err | error                 | 错误信息          |

### <span id="crawlerx-pagescreenshot">crawlerx.PageScreenShot</span>

访问固定页面 加载完成后截图

#### 定义

`func crawlerx.PageScreenShot(url: string, opts: ...crawlerx.ConfigOpt) return (screenshot: string, err: error)`

#### 参数

| 参数名  | 参数类型                  | 参数解释    |
|------|-----------------------|---------|
| url  | string                | 截图目标url |
| opts | ...crawlerx.ConfigOpt | 访问参数    |

#### 返回值

| 返回值        | 返回值类型  | 返回值解释           |
|------------|--------|-----------------|
| screenshot | string | 目标url页面截图的b64编码 |
| err        | error  | 错误信息            |

### <span id="crawlerx-browserinfo">crawlerx.browserInfo</span>

设置浏览器参数

#### 定义

`func crawlerx.browserInfo(info: string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名  | 参数类型   | 参数解释  |
|------|--------|-------|
| info | string | 浏览器参数 |

浏览器参数为一个json字符串：

    {
        "ws_address":"",
        "exe_path":"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
        "proxy_address":"http://127.0.0.1:8083",
        "proxy_username":"",
        "proxy_password":"",
    }

其中ws_address为远程chrome浏览器地址，exe_path为chrome浏览器可执行文件的路径，这两个参数设置一个就可以，不设置则会默认下载chrome浏览器并运行

proxy_address为代理地址，proxy_username和proxy_password分别为代理的用户名和密码（需要则填写）

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-maxurl">crawlerx.maxUrl</span>

最大爬虫数量设置

#### 定义

`func crawlerx.maxUrl(maxUrlNum: int) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名       | 参数类型 | 参数解释      |
|-----------|------|-----------|
| maxUrlNum | int  | 最大爬取url数量 |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-maxdepth">crawlerx.maxDepth</span>

设置最大爬取深度

#### 定义

`func crawlerx.maxDepth(depth: int) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名   | 参数类型 | 参数解释   |
|-------|------|--------|
| depth | int  | 最大爬虫深度 |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-concurrent">crawlerx.concurrent</span>

最大浏览器打开页面数量（相当于并行数量）

#### 定义

`func crawlerx.concurrent(concurrentNumber: int) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名              | 参数类型 | 参数解释        |
|------------------|------|-------------|
| concurrentNumber | int  | 最大浏览器打开页面数量 |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-blacklist">crawlerx.blackList</span>

爬虫黑名单参数设置

#### 定义

`func crawlerx.blackList(keywords: ...string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名      | 参数类型      | 参数解释   |
|----------|-----------|--------|
| keywords | ...string | 黑名单关键词 |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-whitelist">crawlerx.whiteList</span>

爬虫白名单参数设置

#### 定义

`func crawlerx.whiteList(keywords: ...string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名      | 参数类型      | 参数解释   |
|----------|-----------|--------|
| keywords | ...string | 白名单关键词 |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-pagetimeout">crawlerx.pageTimeout</span>

爬虫单页面超时时间设置

#### 定义

`func crawlerx.pageTimeout(timeout: int) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名     | 参数类型 | 参数解释    |
|---------|------|---------|
| timeout | int  | 单页面超时时间 |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-fulltimeout">crawlerx.fullTimeout</span>

爬虫全局超时时间设置

#### 定义

`func crawlerx.fullTimeout(timeout: int) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名     | 参数类型 | 参数解释     |
|---------|------|----------|
| timeout | int  | 爬虫全局超时时间 |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-extrawaitloadtime">crawlerx.extraWaitLoadTime</span>

设置页面的额外等待时间 因为有些时候通过devtools拿到的页面状态为加载完成 但是实际上页面仍然在渲染部分内容
此时可以通过该函数进行额外的等待时间的设置

#### 定义

`func crawlerx.extraWaitLoadTime(timeout: int) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名     | 参数类型 | 参数解释                   |
|---------|------|------------------------|
| timeout | int  | 额外等待时间 (单位Millisecond) |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-formfill">crawlerx.formFill</span>

爬虫表单填写设置

#### 定义

`func crawlerx.formFill(formFills: map[string]string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名       | 参数类型              | 参数解释     |
|-----------|-------------------|----------|
| formFills | map[string]string | 表单填写内容字典 |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-fileinput">crawlerx.fileInput</span>

爬虫文件上传设置

#### 定义

`func crawlerx.fileInput(fileInput: map[string]string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名       | 参数类型              | 参数解释   |
|-----------|-------------------|--------|
| fileInput | map[string]string | 上传文件设置 |

参数map的key为关键词 value为文件路径；当key为default时 value为默认上传文件

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-headers">crawlerx.headers</span>

爬虫request的header设置

#### 定义

`func crawlerx.headers(headers: map[string]string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名     | 参数类型              | 参数解释     |
|---------|-------------------|----------|
| headers | map[string]string | header内容 |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-rawheaders">crawlerx.rawHeaders</span>

爬虫request的header设置

#### 定义

`func crawlerx.rawHeaders(headersInfo: string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名         | 参数类型   | 参数解释     |
|-------------|--------|----------|
| headersInfo | string | header内容 |

输入为数据包中的原生headers字符串

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-cookies">crawlerx.cookies</span>

爬虫request的cookie设置

#### 定义

`func crawlerx.cookies(cookies: map[string]string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名     | 参数类型              | 参数解释       |
|---------|-------------------|------------|
| cookies | map[string]string | cookie内容   |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-rawcookie">crawlerx.rawCookie</span>

爬虫request的cookie设置

#### 定义

`func crawlerx.rawCookie(cookieInfo: string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名        | 参数类型   | 参数解释     |
|------------|--------|----------|
| cookieInfo | string | cookie内容 |

输入为数据包中的原生cookie字符串

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-scanrangelevel">crawlerx.scanRangeLevel</span>

爬虫爬取范围

#### 定义

`func crawlerx.scanRangeLevel(scanRange: crawlerx.scanRangeLevel) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名       | 参数类型                    | 参数解释     |
|-----------|-------------------------|----------|
| scanRange | crawlerx.scanRangeLevel | 爬虫爬取范围等级 |

`crawlerx.scanRangeLevel` 包括以下几种：

`crawlerx.AllDomainScan` 表示爬取全域名 （默认）

`crawlerx.SubMenuScan` 表示爬取目标URL和子目录

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-scanrepeatlevel">crawlerx.scanRepeatLevel</span>

爬虫结果重复过滤设置

#### 定义

`func crawlerx.scanRepeatLevel(scanRepeat: crawlerx.repeatLevel) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名        | 参数类型                 | 参数解释       |
|------------|----------------------|------------|
| scanRepeat | crawlerx.repeatLevel | 爬虫结果重复过滤等级 |

`crawlerx.repeatLevel` 包括以下几种：

`crawlerx.UnLimitRepeat` 对page，method，query-name，query-value和post-data敏感

`crawlerx.LowRepeatLevel` 对page，method，query-name和query-value敏感（默认）

`crawlerx.MediumRepeatLevel` 对page，method和query-name敏感

`crawlerx.HighRepeatLevel` 对page和method敏感

`crawlerx.ExtremeRepeatLevel` 对page敏感

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-ignorequeryname">crawlerx.ignoreQueryName</span>

url中的query名称查重忽略设置

#### 定义

`func crawlerx.ignoreQueryName(queryNames: ...string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名        | 参数类型      | 参数解释             |
|------------|-----------|------------------|
| queryNames | ...string | 需要跳过查重筛查的query名称 |

例如现在存在如下几个url：

- http://xxx.com/abc/def?name=aaa&age=10&token=123456

- http://xxx.com/abc/def?name=aaa&age=10&token=456789

这两条url可能由于一些特殊情况，导致query中的token不一致，但是页面内容相同，但是两个url毕竟不一致，所以程序默认会认为两个不一样的url都需要进行访问

此时为了避免这种情况我们可以将token输入crawlerx.ignoreQueryName，让程序在进行url去重时忽略token：

    ... ...
    ignore = crawlerx.ignoreQueryName("token")
    ch = crawlerx.StartCrawler(urlStr, ignore)
    ... ...

此时上面两个url在去重检测时会被认为是同一个url，只会对其中一个进行访问

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-sensitivewords">crawlerx.sensitiveWords</span>

敏感词设置，遇到元素中存在敏感词则不会进行点击

#### 定义

`func crawlerx.sensitiveWords(words: []string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名   | 参数类型     | 参数解释     |
|-------|----------|----------|
| words | []string | 需要过滤的敏感词 |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-leakless">crawlerx.leakless</span>

浏览器是否自动进程关闭设置
浏览器自动进程关闭进行在windows下会报病毒 默认在windows下会关闭 如在windows下开启请关闭相关安全软件
当关闭时 如果强制关闭爬虫进程时chrome.exe会存在后台 过多时需要手动进行关闭
默认是default, 强制开启为true，强制关闭为false

#### 定义

`func crawlerx.leakless(leakless: string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名      | 参数类型   | 参数解释     |
|----------|--------|----------|
| leakless | string | 自动进程关闭设置 |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-localstorage">crawlerx.localStorage</span>

在当前域名下的localstorage中存储键值对

#### 定义

`func crawlerx.localStorage(storage: map[string]string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名     | 参数类型              | 参数解释                      |
|---------|-------------------|---------------------------|
| storage | map[string]string | 要存在当前域名下localstorage的键值映射 |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-invalidsuffix">crawlerx.invalidSuffix</span>

设置非法后缀，遇到url拥有该后缀时不进行访问

#### 定义

`func crawlerx.invalidSuffix(suffix: []string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名    | 参数类型     | 参数解释          |
|--------|----------|---------------|
| suffix | []string | 不进行爬虫的url后缀切片 |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-stealth">crawlerx.stealth</span>

设置是否运行反-反爬虫代码

#### 定义

`func crawlerx.stealth(stealth: bool) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名     | 参数类型 | 参数解释          |
|---------|------|---------------|
| stealth | bool | 设置是否运行反-反爬虫代码 |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-runtimeid">crawlerx.runtimeID</span>

设置爬虫的runtimeID

#### 定义

`func crawlerx.runtimeID(id: string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名 | 参数类型   | 参数解释         |
|-----|--------|--------------|
| id  | string | 设置的runtimeID |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-evaljs">crawlerx.evalJs</span>

设置在爬到固定页面时执行指定的js代码

#### 定义

`func crawlerx.evalJs(target: string, evalJs string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名     | 参数类型   | 参数解释           |
|---------|--------|----------------|
| target  | string | 执行对应js代码的目标url |
| evalJs  | string | 要执行的js代码内容     |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-jsresultsend">crawlerx.jsResultSend</span>

设置在获得运行js代码结果后的处理（通常指结果传出）

#### 定义

`func crawlerx.jsResultSend(send: func(string)) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名  | 参数类型         | 参数解释                 |
|------|--------------|----------------------|
| send | func(string) | 对js代码执行结果的字符串进行操作的函数 |

```
stack = make([]string, 0)
strFunc = func(s){
    stack = append(stack, s)
}
opt = crawlerx.jsResultSend(strFunc)
```

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-vue">crawlerx.vue</span>

强制设置爬虫模式为vue模式，即事件驱动爬虫

#### 定义

`func crawlerx.vue(vue: bool) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名 | 参数类型 | 参数解释      |
|-----|------|-----------|
| vue | bool | 是否执行vue模式 |

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |


### <span id="crawlerx-response">crawlerx.response</span>

设置指定url的response

#### 定义

`func crawlerx.response(targetUrl: string, response: string) return (r0: crawlerx.ConfigOpt)`

#### 参数

| 参数名       | 参数类型   | 参数解释         |
|-----------|--------|--------------|
| targetUrl | string | 指定特定响应内容的url |
| response  | string | 指定的响应内容字符串   |

response为原生response：
```
"HTTP/1.1 200\r\nSet-Cookie: JSESSSIONID=E8ECA470AF9F5385159DE0E8E9BD6726; Path=/; HttpOnly\r\nContent-Type: text/html; charset=utf-8\r\nDate: Wed, 01 Nov2023 03:44:53GMT\r\nContent-Length: 35\r\n\r\ne165421110ba03099a1c393373c5b43\n\r\n"
```

#### 返回值

| 返回值 | 返回值类型              | 返回值解释  |
|-----|--------------------|--------|
| r0  | crawlerx.ConfigOpt | 参数设置函数 |
