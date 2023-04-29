# scanfp 指纹识别使用说明

## 帮助信息

```
NAME:
   scanfp - A new cli application

USAGE:
   scanfp [global options] command [command options] [arguments...]

VERSION:
   0.0.0

COMMANDS:
     help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --hosts value, --target value, -t value       输入扫描主机，以逗号分隔例如：(192.168.1.1/24,192.168.1.1-23,10.1.1.2)
   --port value, --tcp-port value, -p value      输入想要扫描的端口，支持单个端口和范围，例如（80,443,21-25,8080-8082） (default: "22,80,443,3389,3306,8080-8082,9000-9002,7000-7002")
   --udp-port value                              想要扫描的 UDP 端口，支持单个端口和范围
   --rule-path value, --rule value, -r value     手动加载规则文件夹
   --concurrent value, --thread value, -c value  并发速度，同时有多少个扫描过程进行？ (default: 20)
   --web                                         主动开启 web 扫描模式
   --request-timeout value                       单个请求的超时时间（Seconds） (default: 6)
   --json value, -o value                        详细结果输出 json 到文件
   --help, -h                                    show help
   --version, -v                                 print the version
```

## 1. 从源码中使用 scanfp

在根目录下直接运行

`go run common/fp/cmd/scanfp.go --target 47.52.100.105/27 --port 80 --json result.json`

即可进行扫描，上述命令是扫描 47.53.100.105/27 这个网段的，根据实际需要，可以修改各种参数，参数说明见帮助信息

## 2. 编译

根目录下执行

`go build -o scanfp common/fp/cmd/scanfp.go`

可以编译出 scanfp 供分发执行

如果需要编译不同平台的 scanfp，则可以考虑参考

```
GOOS=linux GOARCH=amd64 go build -o scanfp_linux_amd64  common/fp/cmd/scanfp.go

GOOS=windows GOARCH=amd64 go build -o scanfp_win_amd64  common/fp/cmd/scanfp.go
```

## 3. scanfp 指纹系统介绍

### 兼容 Nmap 模式指纹

可兼容 Nmap 服务指纹模式

### Web 指纹

格式简单，可以支持多种情况的 Web 指纹识别模块

#### 如何进行基础 Web 指纹的编写？

从一些简单的案例开始：

有些 Shiro 的网站，他的 Set-Cookie 会携带相关指纹，我们如何编写？(YAML 格式指纹)

```
- methods:
    - headers:
        - key: Set-Cookie
          value:
            product: shiro
            regexp: 'isRememberMe'
        - key: Set-Cookie
          value:
            product: shiro
            regexp: 'shiro-session-redis'
```

上述指纹内容包含：

methods 表示这个指纹的开始：意思是，这个指纹会使用如下方法：

headers 在 methods 内，代表使用 headers 识别来测试指纹内容

key 表示 http header 的 key, 表示对哪个 Http 头进行指纹识别：

value: 表示一个指纹识别对象，可以支持生成 CPE 对象（[什么是 CPE 呢？](https://nvd.nist.gov/products/cpe)）

```
cpe 可以支持多个分部： 
    一种定义：   fmt.Sprintf("cpe:/%s:%s:%s:%s:%s:%s:%s", c.Part, c.Vendor, c.Product, c.Version, c.Update, c.Edition, c.Language)
    另一种定义： fmt.Sprintf("cpe:2.3:%v:%v:%v:%v:%v:%v:%s", c.Part, c.Vendor, c.Product, c.Version, c.Update, c.Edition, c.Language)


最常见的四个分部的意义解释
part: 表示设备类型 
    1. a 为产品 (一般来说都是这个) 
    2. o 为操作平台
    3. h 为硬件
vendor 一般指厂商
product 产品名
version 指产品版本
``` 

在上述指纹中，product: 表示生成的 CPE 的 product 分部内容是 shiro, 如果编写成 shiro，则 cpe 就是 cpe:/a:*:shiro:*

regexp 表示进行正则匹配的内容，当然有时候，正则会匹配到版本，我们如何设置 cpe version 的动态内容呢？

我们查看如下内容：varnish 这个指纹

```
- methods:
  - headers:
    - key: Via
      value:
        product: varnish
        regexp: 'varnish(?: \(Varnish/([\d.]+)\))?'
        version_index: 1
    - key: X-Varnish
      value:
        product: varnish
    - key: X-Varnish-Action
      value:
        product: varnish
    - key: X-Varnish-Age
      value:
        product: varnish
    - key: X-Varnish-Cache
      value:
        product: varnish
    - key: X-Varnish-Hostname
      value:
        product: varnish
```

这个指纹中出现了 version_index, 这个字段的意义表示，从正则匹配的分组中取编号为 1 的分组，把这个分组的内容写的 version 的位置。

除此之外，这个指纹的意义也很明显：

```
如果 Via 中有 varnish: Varnishxxx 
或有 X-Varnish / X-Varnish-Action / X-Varnish-Cache / X-Varnish-Age 
    / X-Varnish-Hostname 这些 HTTP Header，则输出 cpe:/a:*:varnish:* 这个 CPE 指纹
```

#### 编写针对特定 URL 的指纹

使用场景，需要主动访问某个 URL，才能触发的指纹

```
- path: /console/login/LoginForm.jsp
  methods:
    - keywords:
        - product: weblogic
          vendor: oracle
          version_index: 1
          regexp: 'WebLogic Server.*: ([0-9\.]+)</p>'
        - product: weblogic_server
          vendor: oracle
          version_index: 1
          regexp: 'WebLogic Server.*: ([0-9\.]+)</p>'
```

比如上述指纹，主动访问 path 的 /console/login/LoginForm.jsp 来触发 weblogic 相关登录页面，然后在匹配 Weblogic Server 字段来匹配 Weblogic 指纹

#### 支持 MD5

```
- methods:
    - md5s:
        - md5: 1af585e6c8cc77a6ca1832b608fd20aa
          product: xxxx
          version: 1.11.1
```

上面指纹内容是，如果匹配到响应内容符合 1af585e6c8cc77a6ca1832b608fd20aa 这个指纹，则输出 cpe:/a:*:xxxx:1.11.1

#### 支持多步骤指纹，next_step 字段（略）


#### 针对页面关键字的指纹

有些网站我们只需要确定网站内容有关键字，如何编写指纹呢？

```
- methods:
    - keywords:
        - product: phpmyadmin
          vendor: phpmyadmin
          regexp: 'phpmyadmin\.css\.php\?'
```

这个指纹将会输出 cpe:/a:phpmyadmin:phpmyadmin111:*:*:*:* 的 CPE

## 支持 半开扫描 / SYN 端口扫描 （4.8）

暂时支持（Linux 和 MacOS ）

scanfp 增加了 syn 端口扫描能力

SYN 端口扫描可以非常快，和 massscan 一个原理，并且本系统可以做到比 massscan 更快，或者更准

"快" 和 "准" 不一定可以兼得，为了准确率，有时要牺牲快速，在本系统中，这部分能力会更能体现在配置中

```
NAME:
   scanfp synscan - SYN 端口扫描

USAGE:
   scanfp synscan [command options] [arguments...]

OPTIONS:
   --target value, --host value, -t value
   --port value, -p value                     (default: "22,80,443,3389,3306,8080-8082,9000-9002,7000-7002")
   --wait value, --waiting value              在 SYN 包发送完毕之后等待多长时间进行收尾（Seconds） (default: 10)
   --packet-per-second value                  每秒发多少个 SYN 包 (default: 100)
   --syn-repeat value                         默认 SYN 包重复发多少个（防止漏报） (default: 7)
   --fingerprint, --fp, -x                    开启指纹扫描
   --request-timeout value                    单个请求的超时时间（Seconds） (default: 10)
   --rule-path value, --rule value, -r value  手动加载规则文件/文件夹
   --only-rule                                只加载这个文件夹中的 Web 指纹
   --fp-json value, --fpo value               详细结果输出 json 到文件
   --output value                             输出端口开放的信息到文件
```

### 端口扫描 关键选项说明：

#### --fp 开启指纹识别--request-timeout / --rule-path / --only-rule / --fp-json(--json)  这些选项和指纹识别的选项功能一样

这个选项打开之后，端口扫描的结果会直接交给指纹识别进行识别

#### --packet-per-second: 每秒发送多少个 SYN 探测包？

这个选项直接影响到扫描速度，当每秒发送 1000 个包和每秒发送 100 个包，速度不一样，但是丢包率不一样，会导致准确率略有下降

举例，在一个 C 段扫描常见端口中，1000 个包每秒最终结果是 15 个端口左右，100 个包一秒最终结果是 20 个端口；

当然受网络质量和链路质量影响，丢包时有发生，SYN 扫描利用的半开扫描并不是完整的 TCP 链接，所以并不能保证可靠性。胜在速度一定比 TCP 扫描快，消耗资源少于 TCP 连接。

#### --syn-repeat: syn 重复发包数量

这个选项也会影响到扫描速度，但是影响最大的是准确度，因为 SYN 扫描的连接不可靠性，多次重复发包确认可以提高准确度

#### --output 实时并且单独输出开放端口扫描结果

这个选项可以输出开放端口到文件，格式是一个字典文件，每一行是一个 IP:Port，例如如下内容

```
47.52.100.142:80
47.52.100.85:80
47.52.100.219:3306
47.52.100.191:3306
47.52.100.191:22
47.52.100.39:443
47.52.100.182:80
47.52.100.43:22
47.52.100.122:443
47.52.100.152:443
47.52.100.153:80
47.52.100.34:443
47.52.100.219:22
47.52.100.122:22
47.52.100.128:443
47.52.100.72:443
52.100.191:80
```

## 爆破工具

支持多目标的分布式爆破工具。

爆破并发问题：不应该针对一个目标进行大规模，高并发的爆破。SSH / FTP / 各种网站，客户端认证都有防止告诉爆破的机制。本系统为了解决这个问题采用了两个方案：

1. 支持针对很多目标同时进行爆破，但是严格控制单个目标的爆破速率
2. 单目标爆破支持随机延迟，通过 --min-delay 和 --max-delay 进行控制最大最小延迟，防止被 ban 或者给业务造成压力

爆破的自定义问题：

不同的爆破错误码的处理方案应该不通，常见问题有下：

1. 如果检测到主机端口根本不开放，就不应该进行爆破了
2. 如果检测到用户名根本不可用，可以剔除该用户名
3. 可以通过设置阈值来设置多次错误，无法爆破导致的资源浪费
4. 有些爆破场景，比如 redis，根本不支持用户名设置，可以人为解决这个问题

### 帮助信息

```
   scanfp brute -

USAGE:
   scanfp brute [command options] [arguments...]

OPTIONS:
   --target value, -t value    可以输入文件，也可以输入模版以简化字典，见下一章内容
   --username value, -u value  (default: "dataex/dicts/user.txt") 可以输入文件，也可以输入模版以简化字典，见下一章内容
   --password value, -p value  (default: "dataex/dicts/3389.txt") 可以输入文件，也可以输入模版以简化字典，见下一章内容
   --min-delay value           (default: 1)  随机最小延迟，针对单个目标进行爆破的时候，不应该速率太大，这个可以设置最小延迟（秒）
   --max-delay value           (default: 2)  随机最小延迟，针对单个目标进行爆破的时候，不应该速率太大，这个可以设置最大延迟（秒）
   --target-concurrent value   (default: 200)  同时针对多少个目标进行爆破？
   --type value, -x value      爆破的类型
   --ok-to-stop                如果一个目标发现了成功的结果，则停止对这个目标的爆破
   --finished-to-end value     爆破的结果如果多次显示'Finished' 就停止爆破，这个选项控制阈值 (default: 10)
   --divider value             用户(username), 密码(password)，输入的分隔符，默认是（,） (default: ",") (非字典模式专用)
```

### 简化字典问题

单行模版渲染可支持 如果输入 `admin{{i(1-10)}}` 可被解析为如下内容

```
admin1
admin2
admin3
admin4
...
admin8
admin9
admin10
``` 

如果输入 `admin{{i(1-10)}}{{i(2-5)}}`

则会生成如下字典

```
admin12
admin13
admin14
admin15
admin22
admin23
admin24
admin25
admin32
admin33
admin34
admin35
...
admin92
admin93
admin94
admin95
admin102
admin103
admin104
admin105
```

可用标签如下

```
# aliasAutoFuzz("INT", []string{"i", "p", "port", "int", "integer"}, [][2]string{{"{{", "}}"}, {"__", "__"}})
# aliasAutoFuzz("CHAR", []string{"c", "char", "ch"}, [][2]string{{"{{", "}}"}, {"__", "__"}})
# aliasAutoFuzz("RANDINT", []string{"ri", "rand", "randi", "randint"}, [][2]string{{"{{", "}}"}, {"__", "__"}})
# aliasAutoFuzz("RANDSTR", []string{"rs", "rands", "randstr"}, [][2]string{{"{{", "}}"}, {"__", "__"}})
# aliasAutoFuzz("NETWORK", []string{"n", "host", "net", "network"}, [][2]string{{"{{", "}}"}, {"__", "__"}})

{{i(1,2,3,4,5)}} 等效 {{p(1,2,3,4,5)}} {{port(1,2,3,4,5)}} {{int(1,2,3,4,5)}} {{integer(1,2,3,4,5)}}
{{i(1-5)}} {{i(1,2,3-5)}} {{i1(1,2,3-5)}} {{i2(1,2,3-5)}}
解析为
1
2
3
4
5

{{c(a-z)}} 等效为 {{char(a-z)}} {{ch(a-z)}} {{c1(a-z)}}
解析为
a
b
c
d
e
...
x
y
z

{{ri(1-100)}} 表示取一个 1-100 的随机值

{{rs(100)}} 表示拿一个长度为 100 的随机字符串

{{net(47.52.100.1/24)}} 等效为 47.52.100.1/24 这个网段的展开，详情可执行一下

go run common/fp/cmd/scanfp.go brute --type ssh --target '{{net:(47.52.100.105/24)}}:22' -u 'root' -p 'dataex/dicts/3389.txt'

这个命令感受一下，这可以针对一个整个 C 段进行 ssh 爆破
```

### 爆破工具使用案例

#### 基础的使用

```
go run common/fp/cmd/scanfp.go brute --type ssh --target '127.0.0.1:22' -u 'root' -p 'admin,admin2'
```

使用 root 作为用户名，"admin,admin2" 作为密码，针对 127.0.0.1:22 进行 ssh 爆破


```
go run common/fp/cmd/scanfp.go brute --type ssh --target '127.0.0.1:22' -u 'dataex/dicts/user.txt' -p 'dataex/dicts/3389.txt'
```

使用字典作为用户名和密码对 127.0.0.1:22 进行爆破

#### 高级特性

```
go run common/fp/cmd/scanfp.go brute --type ssh --target '{{net:(47.52.100.105/24)}}:22' -u 'root' -p 'dataex/dicts/3389.txt'
```

以上命令行针对 47.52.100.105/24 这个网段进行 ssh 爆破，使用 22 默认端口，使用 root 作为用户名，`dataex/dicts/3389.txt` 字典作为爆破使用的密码。

#### 高级特性，针对多个复杂目标

```
go run common/fp/cmd/scanfp.go brute --type ssh --target '{{net:(47.52.100.105/24)}}:{{port:(22-26)}}' -u 'root' -p 'dataex/dicts/3389.txt'
```

以上命令行针对 47.52.100.105/24 这个网段进行 ssh 爆破，使用 22-26 作为目标爆破端口，其他同上。

#### 我没有字典，如何使用？

```
go run common/fp/cmd/scanfp.go brute --type ssh --target '{{net:(47.52.100.105/24)}}:22' -u 'root,admin,ops' -p 'admin,admin{{i(1-9)}},admin{{i1(1-9)}}admin{{i2(1-9)}}'
```

针对 47.52.100.105/24 这个网段进行 ssh 爆破，使用用户名字典为

```
root
admin
ops
```

使用密码字典为

```
admin
admin1
admin2
admin3
admin4
admin5
admin6
admin7
admin8
admin9
admin11
admin12
admin13
admin14
admin15
admin16
admin17
admin18
admin19
admin21
admin22
admin23
admin24
admin25
admin26
admin27
admin28
admin29
...
admin96
admin97
admin98
admin99
```

执行即可看到效果

### 自定义你的爆破函数

自带一些爆破函数，但是效果不一定可以保证，如果想要爆破复杂 IoT 设备，或者 Web 服务，需要自己编写你自己的认证函数，查看如下文件

`common/utils/bruteutils/auth_func.go`

可以参考 SSH 爆破的内容，对其他爆破进行补充。

1. 如果爆破成功，设置 Ok 为 true , 这个选项是必须的，否则无法识别爆破结果

如果可以的话，设置好 *BruteItemResult 中的其他字段，比如

1. 如果不需要输入用户名，设置 OnlyNeedPassword 为 true
2. 如果确认这个目标不可用，或者目标爆破已经结束，设置 Finished 为 true
3. 如果用户名不可用，设置 UserEliminated 为 true

将会让你的爆破更有效率

### Happy Bruting