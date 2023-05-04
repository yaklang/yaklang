go run spider.go -H [target address] -D [spider depth] --proxy [proxy address] --proxy-username [proxy username] --proxy-password [proxy password] --stricturl

# 命令行运行

go run cmd/spider.go
参数
-H --host 爬虫目标地址 默认127.0.0.1
-D --depth 爬虫深度 默认为3
--proxy 流量代理地址 若需要用户名密码则额外添加--proxy-username和--proxy-password
--strict-url 输入该参数则会对敏感url进行默认过滤 不进行模拟点击
--headers header数据 包括文件输入和直接输入两种模式 文件输入headers数据所保存的文件路径，其中headers数据为源代码格式；直接输入则为json字符串
--maxurl 设置最大url扫描数量，超过这个数量则停止扫描
--timeout 单页面超时时间 最大为20

# 使用yakit中的yak runner 调用rpa包运行

ch, err = rpa.Start(host, ...configopt) 发起爬虫 返回一个chan和err 该channel中输出爬虫得到的url，其中configopt为可选参数
configopt = rpa.depth(int) 获得一个设置爬虫深度的configopt
configopt = rpa.proxy(host, ...userinfo) 获得一个设置爬虫流量代理的configopt，其中host为代理地址，userinfo为选填项，可以输入代理的用户名和密码
configopt = rpa.headers(string) 获得一个设置headers的configopt，其中输入strings既可以是json格式的headers数据，也可以是存有源代码格式的headers数据的文件路径
configopt = rpa.strict_url(bool) 获得一个设置是否开启敏感url模式的configopt，true为开启，不点击敏感url，false相反
configopt = rpa.max_url(int)
configopt = rpa.white_domain(string) 获得一个设置白名单匹配规则的configopt，string匹配规则参考glob：https://github.com/gobwas/glob
configopt = rpa.black_domain(string) 获得一个设置黑名单匹配规则的configopt，string匹配规则参考glob：https://github.com/gobwas/glob
configopt = rpa.timeout(int) 获得一个设置单个页面超时时间的configopt

例：
maxurl_config = rpa.max_url(30)
depth_config = rpa.depth(3)
ch, err = rpa.Start("http://testphp.vulnweb.com/", maxurl_config, depth_config)
for result = range ch{
    println(result.Url())
}
result中包含如下参数：
result.Url()
result.Request()
result.ResponseBody()
Response()
其中Response()为空
当url未点击时，以下参数为空
result.Request()
result.ResponseBody()

# tbc

*~~1、验证码图片发送 获得结果验证~~*

~~2、敏感路径识别~~

====

3、指定url截图

~~4、伪造参数 cookie等~~

~~5、扫描指定数量url后停止扫描~~

6、分级过滤

7、headless弹窗 ?

8、rebuild

9、response 点击判断 ?

====

2.0 rpa mode

1、爆破

~~2、指定元素~~

~~3、爆破前操作~~

4、广义爆破

5、指定结束

6、验证码识别接口

7、指定截图
