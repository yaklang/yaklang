# CrawlerX 爬虫模块使用说明

## 代码示例

    browserData = {
        "ws_address":"",
        "exe_path":"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
        // "proxy_address":"http://127.0.0.1:8083",
        "proxy_username":"",
        "proxy_password":"",
    }
    
    browserInfo = newcrawlerx.browserInfo(json.dumps(browserData))
    formFillMap = {"username":"admin","password":"password"}
    formFill = newcrawlerx.formFill(formFillMap)
    fileUpload = newcrawlerx.fileInput({"default":"/Users/chenyangbao/1.txt"})
    blackList = newcrawlerx.blackList("logout","captcha")
    vue = newcrawlerx.vueWebsite(false)
    timeout = newcrawlerx.extraWaitLoad(1000)
    
    ch,_ = newcrawlerx.startCrawler("http://testphp.vulnweb.com/",formFill, fileUpload, blackList, browserInfo, vue, timeout)
    for item = range ch{
        println(item.Method() + " " + item.Url())
    }

## Data Struct

### newcrawlerx.ReqInfo

爬虫结果数据结构

#### struct

    type ReqInfo interface {
        PtrStructMethods(指针结构方法/函数):
            func Url() return(string)
            func Method() return(string)
    
            func RequestHeaders() return(map[string]string)
            func RequestBody() return(string)
    
            func ResponseHeaders() return(map[string]string)
            func ResponseBody() return(string)
    }

#### methods

`func (*ReqInfo) Url() return(r0: string)` 爬虫结果的url

`func (*ReqInfo) Method() return(string)` 爬虫结果的请求方法

`func (*ReqInfo) RequestHeaders() return(map[string]string)` 爬虫结果的请求包头文件

`func (*ReqInfo) RequestBody() return(string)` 爬虫结果的请求包body

`func (*ReqInfo) StatusCode() return(int)` 爬虫结果的返回包状态码

`func (*ReqInfo) ResponseHeaders() return(map[string]string)` 爬虫结果的返回包头文件

`func (*ReqInfo) ResponseBody() return(string)` 爬虫结果的返回包body

## API

### newcrawlerx.startCrawler

设置爬虫参数 开始爬虫任务

#### 定义

`func newcrawlerx.startCrawler(url string, opts ...newcrawlerx.ConfigOpt) return (ch: chan newcrawlerx.ReqInfo, err: error)`

#### 参数

| 参数名  | 参数类型                     | 参数解释 |
|------|--------------------------|------|
| url  | string                   | 渗透目标 |
| opts | ...newcrawlerx.ConfigOpt | 扫描参数 |

#### 返回值

| 返回值 | 返回值类型                    | 返回值解释         |
|-----|--------------------------|---------------|
| ch  | chan newcrawlerx.ReqInfo | 爬虫结果传递channel |
| err | error                    | 错误信息          |

### newcrawlerx.browserInfo

设置浏览器参数

#### 定义

`func newcrawlerx.browserInfo(info string) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名  | 参数类型   | 参数解释  |
|------|--------|-------|
| info | string | 浏览器参数 |

<font size=1>浏览器参数为一个json字符串：

    {
        "ws_address":"",
        "exe_path":"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
        "proxy_address":"http://127.0.0.1:8083",
        "proxy_username":"",
        "proxy_password":"",
    }

其中ws_address为远程chrome浏览器地址，exe_path为chrome浏览器可执行文件的路径，这两个参数设置一个就可以，不设置则会默认下载chrome浏览器并运行

proxy_address为代理地址，proxy_username和proxy_password分别为代理的用户名和密码（需要则填写）
</font>

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.blackList

爬虫黑名单参数设置

#### 定义

`func newcrawlerx.blackList(keywords ...string) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名      | 参数类型      | 参数解释   |
|----------|-----------|--------|
| keywords | ...string | 黑名单关键词 |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.whiteList

爬虫白名单参数设置

#### 定义

`func newcrawlerx.whiteList(keywords ...string) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名      | 参数类型      | 参数解释   |
|----------|-----------|--------|
| keywords | ...string | 白名单关键词 |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.timeout

爬虫单页面超时时间设置

#### 定义

`func newcrawlerx.timeout(timeout int) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名     | 参数类型 | 参数解释    |
|---------|------|---------|
| timeout | int  | 单页面超时时间 |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.fullTimeout

爬虫全局超时时间设置

#### 定义

`func newcrawlerx.fullTimeout(timeout int) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名     | 参数类型 | 参数解释     |
|---------|------|----------|
| timeout | int  | 爬虫全局超时时间 |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.formFill

爬虫表单填写设置

#### 定义

`func newcrawlerx.formFill(formFills map[string]string) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名       | 参数类型              | 参数解释     |
|-----------|-------------------|----------|
| formFills | map[string]string | 表单填写内容字典 |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.fileInput

爬虫文件上传设置

#### 定义

`func newcrawlerx.fileInput(fileInput map[string]string) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名       | 参数类型              | 参数解释   |
|-----------|-------------------|--------|
| fileInput | map[string]string | 上传文件设置 |

<font size=1>参数map的key为关键词 value为文件路径；当key为default时 value为默认上传文件</font>

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.header

爬虫request的header设置

#### 定义

`func newcrawlerx.header(kv ...string) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名 | 参数类型      | 参数解释     |
|-----|-----------|----------|
| kv  | ...string | header内容 |

<font size=1>奇数项为header的key，偶数项为对应的值</font>

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.headers

爬虫request的header设置

#### 定义

`func newcrawlerx.headers(headers map[string]string) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名     | 参数类型              | 参数解释     |
|---------|-------------------|----------|
| headers | map[string]string | header内容 |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.cookie

爬虫request的cookie设置

#### 定义

`func newcrawlerx.cookie(domain string, kv ...string) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名     | 参数类型        | 参数解释       |
|---------|-------------|------------|
| domain  | string      | cookie作用域名 |
| kv      | ...string   | cookie内容   |

<font size=1>kv的奇数项为cookie的key，偶数项为对应的值</font>

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.cookies

爬虫request的cookie设置

#### 定义

`func newcrawlerx.cookies(domain string, cookies map[string]string) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名     | 参数类型              | 参数解释       |
|---------|-------------------|------------|
| domain  | string            | cookie作用域名 |
| cookies | map[string]string | cookie内容   |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.scanRange

爬虫爬取范围

#### 定义

`func newcrawlerx.scanRange(scanRange newcrawlerx.scanRangeLevel) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名       | 参数类型                        | 参数解释     |
|-----------|-----------------------------|----------|
| scanRange | newcrawlerx.scanRangeLevel  | 爬虫爬取范围等级 |

<font size=1>`newcrawlerx.scanRangeLevel` 包括以下几种：

`newcrawlerx.AllDomainScan` 表示爬取全域名 （默认）

`newcrawlerx.SubMenuScan` 表示爬取目标URL和子目录</font>

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.scanRepeat

爬虫结果重复过滤设置

#### 定义

`func newcrawlerx.scanRepeat(scanRepeat newcrawlerx.limitLevel) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名        | 参数类型                   | 参数解释       |
|------------|------------------------|------------|
| scanRepeat | newcrawlerx.limitLevel | 爬虫结果重复过滤等级 |

<font size=1>`newcrawlerx.limitLevel` 包括以下几种：

`newcrawlerx.UnLimitRepeat` 对page，method，query-name，query-value和post-data敏感

`newcrawlerx.LowRepeatLevel` 对page，method，query-name和query-value敏感（默认）

`newcrawlerx.MediumRepeatLevel` 对page，method和query-name敏感

`newcrawlerx.HighRepeatLevel` 对page和method敏感

`newcrawlerx.ExtremeRepeatLevel` 对page敏感</font>

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.maxUrl

最大爬虫数量设置

#### 定义

`func newcrawlerx.maxUrl(maxUrlNum int) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名       | 参数类型 | 参数解释      |
|-----------|------|-----------|
| maxUrlNum | int  | 最大爬取url数量 |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.ignoreQuery

url中的query名称查重忽略设置

#### 定义

`func newcrawlerx.ignoreQuery(queryNames ...string) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名        | 参数类型      | 参数解释             |
|------------|-----------|------------------|
| queryNames | ...string | 需要跳过查重筛查的query名称 |

<font size=1>例如现在存在如下几个url：

- http://xxx.com/abc/def?name=aaa&age=10&token=123456

- http://xxx.com/abc/def?name=aaa&age=10&token=456789

这两条url可能由于一些特殊情况，导致query中的token不一致，但是页面内容相同，但是两个url毕竟不一致，所以程序默认会认为两个不一样的url都需要进行访问

此时为了避免这种情况我们可以将token输入newcrawlerx.ignoreQuery，让程序在进行url去重时忽略token：

    ... ...
    ignore = newcrawlerx.ignoreQuery("token")
    ch = newcrawlerx.startCrawler(ignore)
    ... ...

此时上面两个url在去重检测时会被认为是同一个url，只会对其中一个进行访问</font>
    

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.vueWebsite

设置扫描目标为vue站点
由于vue站点中的链接并不是直接写在href或src中，需要不同的策略进行获取，所以这里需要进行设置

#### 定义

`func newcrawlerx.vueWebsite(vue bool) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名  | 参数类型 | 参数解释      |
|------|------|-----------|
|  vue | bool | 是否为vue网站  |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.extraWaitLoad

设置页面的额外等待时间 因为有些时候通过devtools拿到的页面状态为加载完成 但是实际上页面仍然在渲染部分内容
此时可以通过该函数进行额外的等待时间的设置

#### 定义

`func newcrawlerx.extraWaitLoad(timeout int) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名     | 参数类型 | 参数解释                   |
|---------|------|------------------------|
| timeout | int  | 额外等待时间 (单位Millisecond) |

#### 返回值

| 返回值  | 返回值类型                    | 返回值解释  |
|------|--------------------------|--------|
| r0   | newcrawlerx.ConfigOpt    | 参数设置函数 |

### newcrawlerx.maxDepth

设置最大爬取深度

#### 定义

`func newcrawlerx.maxDepth(depth int) return (r0: newcrawlerx.ConfigOpt)`

#### 参数

| 参数名   | 参数类型 | 参数解释   |
|-------|------|--------|
| depth | int  | 最大爬虫深度 |

#### 返回值

| 返回值 | 返回值类型                 | 返回值解释  |
|-----|-----------------------|--------|
| r0  | newcrawlerx.ConfigOpt | 参数设置函数 |