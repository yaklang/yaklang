# CrawlerX 爬虫模块使用说明

`chan, err = crawlerx.StartCrawler(url string, opts ...configopt)` 创建爬虫模块 返回结果输出通道和错误信息

`err = crawlerx.StartCrawlerV2(url string, opts ...configopt)` 新爬虫模块 不会返回爬虫结果 通过代理通道返回

configopt为爬虫的可选择参数 包括以下种类

`configopt = crawlerx.proxy(url string)` 或 `cw.SetProxy(url string, username string, password string)` 设置代理信息

`configopt = crawlerx.maxUrl(int)` 设置最大url爬虫数量

`configopt = crawlerx.whiteList(string)` 设置白名单关键词 支持正则

`configopt = crawlerx.blackList(string)` 设置黑名单关键词 支持正则

`configopt = crawlerx.timeout(int)` 设置单页面超时时间 默认30s

`configopt = crawlerx.maxDepth(int)` 设置最大爬虫深度 默认为3层

`configopt = crawlerx.formFill(key string, value string)` 设置自定义输入框输入 当遇见关键词key包含时默认输入对应value

`configopt = crawlerx.header(key string, value string)` 设置请求头信息 单条形式输入

`configopt = crawlerx.headers(map[string]string)` 设置请求头信息 字典形式输入

`configopt = crawlerx.concurrent(int)` 设置最大并行页面 默认20

`configopt = crawlerx.cookie(domain string, key string, value string)` 设置cookie 单条输入

`configopt = crawlerx.cookies(domain string, value map[string]string)` 设置cookie 字典输入

`configopt = crawlerx.checkDanger()` 设置危险url不进行点击，检测到url中存在某些关键词时自动跳过

`configopt = crawlerx.tags(tagpath string)` 设置标签文件路径，当设置标签文件路径后可以从url包信息中获取标签信息

`configopt = crawlerx.fullTimeout(timeout int)` 设置全局最大超时时间，0表示无最大超时时间，默认360秒

`configopt = crawlerx.chromeWS(wsAddress string)` 设置远程连接chrome地址

`configopt = crawlerx.remote(bool)` 设置是否远程获取爬虫结果

`configopt = crawlerx.extraHeaders(headers ...string)` 设置额外的headers，该headers会在页面生成时设置，对该页面产生的所有请求加入该header

`crawlerx.extraHeaders("anoTestHeaders", "anotherExtraHeaders")`

- - -

`configopt = crawlerx.scanRange(int)` 设置爬取范围 其中

`crawlerX.AllDomainScan`表示爬取全域名 

`crawlerX.SubMenuScan`表示爬取目标URL和子目录

默认为`crawlerX.AllDomainScan`

- - -

`configopt = crawlerx.scanRepeat(int)` 设置重复url判定等级 其中

以URL：http://www.abc.com/test.php?login=admin 为例

page = http://www.abc.com/test.php

method = GET

query-name = login

query-value = admin

`crawlerX.HighRepeatLevel` 对page敏感

`crawlerX.MediumRepeatLevel` 对page和method敏感

`crawlerX.LowRepeatLevel` 对page method和query-name敏感

`crawlerX.UnLimitRepeat` 对page method query-name和query-value敏感

默认为`crawlerX.UnLimitRepeat`

- - -
在从通道获取爬虫结果时

通道中返回的值包括以下方法

`Url() string` URL

`Method() string` 请求方法

`RequestHeaders() map[string]string` 请求头

`RequestBody() string` 请求body

`ResponseHeaders() map[string][]string` 响应头

`ResponseBody() string` 响应body

`Tag() []string` 标签列表

# 使用范例：

    blackConfig = crawlerx.blackList("cart")
    maxUrlConfig = crawlerx.maxUrl(30)
    ch, err = crawlerx.StartCrawler("http://testphp.vulnweb.com/",blackConfig,maxUrlConfig)
    for item = range ch{
        println(item.Url())
    }

# 标签使用范例
    testConfig = crawlerx.tags("/Users/chenyangbao/Project/yak/common/crawlerx/tag/rules/rule.yml")
    ch, err = crawlerx.StartCrawler("http://testphp.vulnweb.com/", testConfig)
    for item = range ch{
        println(item.Url(), item.Tag())
    }

# V2

    depth = crawlerx.maxDepth(5)
    proxy = crawlerx.proxy("http://127.0.0.1:8083")
    ws = crawlerx.chromeWS("http://192.168.0.115:7317")
    remoteUrl = crawlerx.remote(true)
    err = crawlerx.StartCrawlerV2("http://testphp.vulnweb.com/",proxy,remoteUrl,ws,depth)

- - -

# 自定义标签说明

在使用为url添加标签的过程中，需要手动制定标签文件的位置

自定义标签文件时，标签文件采取YAML格式编写

一条基本的标签YAML内容如下：

```
- NAME: file_download_pre_test
  RULES:
    - ORIGIN: response.url_param
      RULE_TYPE: re
      RULE: (path|file|url|Data|src|temp)=
    - ORIGIN: response.url_param
      RULE_TYPE: SCRIPT
      RULE: ORIGIN.lastIndexOf(".")>-1
```

NAME: 标签名称

RULES：规则内容 规则内容包括：

ORIGIN：数据来源 包括
- response.url 响应url
- response.html 响应页面html内容
- response.responseHeader 响应头内容 map[string]string 格式
- response.url_param url参数
- response.path url路径

RULE_TYPE: 规则类型（不分大小写） 规则类型包括：
- re 正则匹配 此时RULE内容为正则匹配的表达式

- json 字典匹配 此时规则中还会出现KEY关键词，用于匹配字典的Key，此时RULE内容为字典对应key需要比对的对应value，例如：
```
- NAME: file_download_pre_test
  RULES:
    - ORIGIN: response.responseHeader
      RULE_TYPE: JSON
      KEY: content-disposition
      RULE: attachment
```

- script 脚本匹配 此时RULE内容为可执行的JS脚本内容 脚本的返回值类型为bool，例如：
```
- NAME: http_struts2_url
  RULES:
    - ORIGIN: response.path
      RULE_TYPE: SCRIPT
      RULE: ORIGIN.endsWith(".do")
```

- xpath 路径匹配 此时RULE内容为html页面中需要存在的element结构，例如：
```
- NAME: http_file_upload_pre_test
  RULES:
    - ORIGIN: response.html
      RULE_TYPE: xpath
      RULE: 'input[type=file]'
```

当一条规则下的所有RULES都为true，才会判断存在该标签

# 指定页面截图

```
    ws = crawlerx.chromeWS("http://192.168.0.115:7317")
    code,err = crawlerx.PageScreenShot("http://testphp.vulnweb.com/",ws)
    println(code)
```