
<p align="center">
  <a href="https://yaklang.io/"><img src="imgs/yaklang-logo.png" style="width: 400px"/></a> 
 <h2 align="center">为网络安全而生的领域编程语言</h2>
<p align="center">
<img src="https://img.shields.io/github/issues-pr/yaklang/yaklang">
<a href="https://github.com/yaklang/yaklang/releases"><img src="https://img.shields.io/github/downloads/yaklang/yaklang/total">
<a href="https://github.com/yaklang/yaklang/graphs/contributors"><img src="https://img.shields.io/github/contributors-anon/yaklang/yaklang">
<a href="https://github.com/yaklang/yaklang/releases/"><img src="https://img.shields.io/github/release/yaklang/yaklang">
<a href="https://github.com/yaklang/yaklang/issues"><img src="https://img.shields.io/github/issues-raw/yaklang/yaklang">
<a href="https://deepwiki.com/yaklang/yaklang"><img src="https://deepwiki.com/badge.svg" alt="Ask DeepWiki"></a>
<a href="https://github.com/yaklang/yaklang/blob/main/LICENSE.md"><img src="https://img.shields.io/github/license/yaklang/yaklang">
</p>

<p align="center">
  <a href="#快速开始">快速开始</a> •
  <a href="https://yaklang.com/docs/intro">官方文档</a> •
  <a href="https://github.com/yaklang/yaklang/issues">问题反馈</a> •
  <a href="https://yaklang.com/api-manual/intro">接口手册</a> •
  <a href="#贡献你的代码">贡献代码</a> •
  <a href="#社区 ">加入社区</a> •
  <a href="#项目架构">项目架构</a> 
</p>

<p align="center">
 :book:语言选择： <a href="https://github.com/yaklang/yaklang/blob/main/README_EN.md">English</a> • 
  <a href="https://github.com/yaklang/yaklang/blob/main/README.md">中文</a> 
</p>

---
# CDSL-Yakang 简介

CDSL：Cybersecurity Domain Specific Language，全称网络安全领域编程语言。

Yaklang 团队综合“领域限定语言”的思想，构建了CDSL的概念，并以此为核心构建了Yak(又称Yaklang)语言来构建基础设施和语言生态。

Yak 是一门针对网络安全领域研发的易书写，易分发的高级计算机编程语言。Yak具备强类型、动态类型的经典类型特征，兼具编译字节码和解释执行的运行时特征。

Yak语言的运行时环境只依赖于YakVM，可以实现“一次编写，处处运行”的特性，只要有YakVM部署的环境，都可以快速执行Yak语言程序。

<h3 align="center">
  <img src="imgs/yaklang-cdsl.png" style="width: 800px" alt="yaklang-cdsl.png" ></a>
</h3>

Yak语言起初只作为一个“嵌入式语言”在宿主程序中存在，后在电子科技大学网络空间安全学院学术指导下，由 Yaklang.io 研发团队进行长达两年的迭代与改造，实现了YakVM虚拟机让语言可以脱离“宿主语言”独立运行，并与2023年完全开源。 支持目前主流操作系统：macOS，Linux，Windows。


## Yaklang 的优势

基于CDSL概念构建的网络安全领域编程语言Yak，具备了几乎DSL所有的优势，它被设计为针对安全能力研发领域的专用编程语言，实现了常见的大多数安全能力，可以让各种各样的安全能力彼此之间“互补，融合，进化”；提高安全从业人员的生产力。

CDSL在网络安全领域提供的能力具备很多优势：
- 简洁性：使用CDSL构建的安全产品更能实现业务和能力的分离，并且解决方案更加直观；

- 易用性：非专业的人员也可以使用CDSL构建安全产品，而避免安全产品工程化中的信息差；

- 灵活性：CDSL一般被设计为单独使用和嵌入式使用均可，用户可以根据自己的需求去编写DSL脚本以实现特定的策略和检测规则，这往往更能把用户的思路展示出来，而不必受到冗杂知识的制约；

除此之外，作为一门专门为网络安全研发设计的语言，Yak语言除了满足一些基础的语言本身需要具备的特性之外，还具有很多特殊功能，可以帮助用户快速构建网络安全应用：

1. 中间人劫持库函数

2. 复杂端口扫描和服务指纹识别

3. 网络安全领域的加解密库

4. 支持中国商用密码体系：支持SM2椭圆曲线公钥密码算法，SM4分组密码算法，SM3密码杂凑算法等

<h3 align="center">
  <img src="imgs/yaklang-fix.jpg" style="width: 800px" alt="yaklang-fix.jpg" ></a>
</h3>

## 项目架构

![yaklang-architecture](imgs/yaklang-arch.jpg)

## 快速开始

- ### 通过 Yakit 来使用 Yaklang

Yakit (https://github.com/yaklang/yakit) 是 Yaklang.io 团队官方出品的开源 Yaklang IDE，它可以帮助你快速上手 Yaklang 语言。

同时 Yakit 也能将绝大部分安全工程师需要的核心功能图形化。他是免费的，你可以通过 [下载安装 Yakit](https://www.yaklang.com/products/download_and_install)，来开始使用 Yaklang。

关于Yakit的更多内容可移步：[Yakit官网文档](https://yaklang.io/products/intro/)查看


- ### 通过命令行来安装使用

通过命令行来安装使用 Yaklang 请遵循：**https://www.yaklang.com/** 或 **https://www.yaklang.io/** 的指引，或直接执行

#### MacOS / Linux

```bash
bash <(curl -sS -L http://oss.yaklang.io/install-latest-yak.sh)
```

#### Windows

```bash
powershell (new-object System.Net.WebClient).DownloadFile('https://yaklang.oss-cn-beijing.aliyuncs.com/yak/latest/yak_windows_amd64.exe','yak_windows_amd64.exe') && yak_windows_amd64.exe install && del /f yak_windows_amd64.exe
```

## 社区

1. 你可以在 Yaklang 或者Yakit 的 issues 中添加你想讨论的内容或者你想表达的东西，英文或中文均可，我们会尽快回复
2. 国内用户可以添加运营 WeChat 加入群组

<h3 align="center">
  <img src="imgs/yaklang-wechat.jpg" style="width: 200px" alt="yaklang-wechat.jpg" ></a>
</h3>


## 贡献你的代码

这是一个高级话题，在贡献你的代码之前，确保你对 Yaklang 整个项目结构有所了解。

在贡献代码时，如果你希望修改 Yaklang 或 YakVM 本身的核心语法部分，最好与研发团队取得联系。

如果您仅仅想要增加库的功能，或者修复一些库的 Bug，那么您可以直接提交 PR，当然 PR 中最好包含对应的单元测试，这很有助于提升我们的代码质量。

## 项目成员

### Maintainer

[v1ll4n](https://github.com/VillanCh): Yak Project Maintainer.

### yaklang 核心开发者 / Active yaklang core developers

1. [z3](https://github.com/OrangeWatermelon)
2. [Longlone](https://github.com/way29)
3. [Go0p](https://github.com/Go0p)
4. [Matrix-Cain](https://github.com/Matrix-Cain)
5. [bcy2007](https://github.com/bcy2007)
6. [naiquan](https://github.com/naiquann)
7. [Rookie-is](https://github.com/Rookie-is)
8. [wlingze](https://github.com/wlingze)


## 开源许可证

本仓库代码版本使用 AGPL 开源协议，这是一个严格的开源协议，且具有传染性，如果您使用了本仓库的代码，那么您的代码也必须开源。

1. 强制开源网络服务:要求提供网络服务的源代码必须开源。保证开源理念在网络环境下的实践。
2. 其他条款与 GPL 相同:开源免费、开源修改、衍生开源等。

本项目开源仓库仅应该作为个人开源和学习使用。

## 鸣谢

本项目经由[电子科技大学](https://www.uestc.edu.cn)张小松([网络空间安全学院](https://www.scse.uestc.edu.cn/))教授学术指导。

<h3 align="center">
<img src="imgs/lab-logo.png" style="width: 400px"/>
</h3>

### 基础理论学科

1. Alonzo Church, "A set of postulates for the foundation of logic", Annals of Mathematics, 33(2), 346-366, 1932.
2. Dana Scott, Christopher Strachey, "Toward a mathematical semantics for computer languages", Proceedings of the Symposium on Computers and Automata, Microwave Research Institute Symposia Series Vol. 21, New York, 1971.
3. Henk Barendregt, Wil Dekkers, Richard Statman, lambda Calculus with Types, Perspectives in Logic. Cambridge University Press, 2013.
4. Braun, M., Buchwald, S., Hack, S., Leißa, R., Mallon, C., Zwinkau, A. (2013). Simple and Efficient Construction of Static Single Assignment Form. In: Jhala, R., De Bosschere, K. (eds) Compiler Construction. CC 2013. Lecture Notes in Computer Science, vol 7791. Springer, Berlin, Heidelberg.

### 工程技术

1. Terence Parr, "The Definitive ANTLR 4 Reference", Pragmatic Bookshelf, 2013.
2. Terence Parr, "Simplifying Complex Networks Using Temporal Pattern Mining: The Case of AT&T's Observed Data Network", Dissertation, 1995.
3. Terence Parr, Russell Quong, "ANTLR: A Predicated-LL(k) Parser Generator", Journal of Software Practice and Experience, July 1995.
4. Google Ins, "Protocol Buffers", https://developers.google.com/protocol-buffers, 2020.
5. Google Ins, "gRPC", https://grpc.io/, 2020.
6. Microsoft Ins, "Monaco Editor", https://microsoft.github.io/monaco-editor/, 2020.


## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=yaklang/yaklang&type=Date)](https://star-history.com/#yaklang/yaklang&Date)

