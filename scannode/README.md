# scannode

1. 本项目自带了一个脚本执行引擎，可以支持你想要的功能的编写，可以单独调试脚本，编写完成后复制到服务器即可
2. 分布式脚本编写数据流依赖本系统自带的 mq 框架
3. 现有的分布式指纹识别/爬虫是依赖服务器分发与控制的

但是需要注意的是，虽然这个分布式扫描节点依赖的服务器并不开源，但是并不意味着这一小段代码是不可用的，你可以把它当成 `common/mq` 和 `common/yak` 的典型实用案例。

稍作修改任何人都可以实现分布式服务器

## 启动与配置

1. 节点配置很简单，不需要配置核心服务器位置，只需要配置 MQ 地址即可，通信会根据代码协议进行接受任务与执行，汇报结果
2. 如果需要运行超多节点，请启用 --id 参数作为不同节点的区分

## 配置其他扫描器依赖（功能依赖）

## 编写分布式扫描脚本

### 获取参数

### 上报结果

上报结果分为几种内容:

#### 上报风险

上报风险函数定义:

`reportRisk(riskTitle: string, target: string, details: map[string]interface{}, subCategories: ...string) error`

这个函数用于上报：风险/漏洞，本质上是上报漏洞，但是某些漏洞没有目标，只有扫描风险，所以可以用这个简化设置。

#### 上报漏洞

`reportVul(vul: *assets.Vul | *tools.PocVul) error`

上报漏洞，这个漏洞对象一般是扫描器扫的结果，比如 xray 啥的，或者 pocinvoker 执行的结果，可以直接用于上报。

#### 上报弱口令

`reportWeakPassword(result: *bruteutil.BruteItemResult)`

本系统自带的爆破框架爆破的结果可以直接上报！很方便。

#### 上报指纹

`reportPort`
`reportFp`
`reportFingerprint`

支持本系统扫描指纹直接上报，非常好用了。









