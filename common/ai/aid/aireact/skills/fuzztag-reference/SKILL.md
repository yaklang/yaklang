---
name: fuzztag-reference
metadata:
  display_name_zh-CN: FuzzTag 标签全集
description: 按需查询 Yaklang FuzzTag 标签全集、别名、参数示例和宿主依赖；用于选标签、复杂模板组合或解决标签行为差异。执行模板配合 fuzztag 技能与 exec_fuzztag 工具。
---

# FuzzTag 标签全集

先选标签并确认运行条件，再用 `exec_fuzztag` 小规模渲染。需要基础组合与交付流程时，用 `loading_skills` 加载 `skill_name:"fuzztag"`。本文件列出当前仓库 100 个引擎及扩展标签、105 个别名，另含参数标签 params 的两个别名。不是每个宿主都启用所有扩展。

## 组合规则与易错点

- 标准语法 `{{name(text)}}`，无参 `{{name}}`；保留大小写和冒号。参数不是 Yak 表达式，每个标签自行解释逗号、竖线等分隔符。
- 并列标签默认笛卡尔积；嵌套标签先展开，外层逐项加工。`{{base64({{array(a|b)}})}}` 得两条。字面保留用 `{{=原文=}}`；raw 防止解析标签，不阻止外层函数拆分参数。
- 同层标签相同 `::row` 按索引配对，如 `{{array::row(A|B)}}={{int::row(1-2)}}`；列表保持等长，短列表耗尽可能补空。`::rep` 可复用末项；`::dyn` 强制每轮重算，随机值和时间默认动态行为因标签参数而异。
- template 是原文，不会自动把字面 `\n` 或 `\\` 解码。多行保留真实换行；工具传参方式查看 exec_fuzztag 的 USAGE，JSON 参数只转义一次。需要显式解析反斜杠转义时用 unquote。
- first/last/nth 操作的是一次传入的换行文本，不会自动聚合所有嵌套 list 的结果。使用 unquote 生成真实换行后再取行。
- `exec_fuzztag` 保留重复结果；limit 在生成时限制，truncated=true 时 count 不是完整总数。输出多行、空字节或结构化字符串用 format=json 保留结果边界；任意二进制最好先套 hex/base64。
- 文件标签需 enable-file-tags；词表和工作流需 profile 数据库；Codec 插件和热加载标签需对应宿主，本工具未启用。标签不存在可能输出空串，参数错误也可能返回原文或空串，必须检查样本。
- 下列例子展示参数形态；外部路径、插件、密钥与载荷是占位值。少量旧描述与实现有差异，下面按当前源码说明；运行版本变化需重新验证。

## 命名参数（运行时注入）

- **params**（别名：param、p）：`{{params(name)}}`。exec_fuzztag 传 `variables:{"name":["小王","小李"]}`；数组展开，标量单值。未提供的键返回空串。p 在该入口是参数标签，不是整数标签。

## 枚举、随机与组合

- **array**（别名：`list`）：`{{array(a|b|c)}}`。用 | 枚举字符串，自动整理周边空白。
- **array:comma**（别名：`list:comma`）：`{{array:comma(a,b,c)}}`。用逗号枚举。
- **array:auto**（别名：`list:auto`）：`{{array:auto(a,b|c)}}`。同时接受逗号与 |。
- **int**（别名：`port`、`ports`、`integer`、`i`）：`{{int(1-5|3|2)}}`。闭区间 1–5；可选位宽 3、步长 2，得 001/003/005；int(1,3,5) 枚举指定值。
- **randint**（别名：`ri`、`rand:int`、`randi`）：`{{randint(1,10,3)}}`。下界、上界、数量，可附 |位宽；当前实现随机区间上界不包含，单参设上界、下界为 0。
- **randstr**（别名：`rand:str`、`rs`、`rands`）：`{{randstr(8,8,3)}}`。最小长度、最大长度、数量；定长用相同上下限；当前变长实现不包含长度上界。
- **char**（别名：`c`、`ch`）：`{{char(a-c)}}`。枚举字符范围。
- **rangechar**（别名：`range:char`、`range`）：`{{rangechar(20,7e)}}`。以十六进制字节端点枚举；缺省结束端点 ff。
- **punctuation**（别名：`punc`）：`{{punctuation()}}`。枚举内置标点字符集。
- **network**（别名：`host`、`hosts`、`cidr`、`ip`、`net`）：`{{network(192.0.2.1/30,example.com)}}`。展开 CIDR/IP 范围或逗号分隔主机，只生成字符串。
- **uuid**：`{{uuid(3)}}`。生成指定数量 UUID；无参一条。
- **regen**（别名：`re`、`regex`、`regexp`）：`{{regen([ab]{2})}}`。枚举正则的匹配字符串；控制字符集和量词规模。
- **regen:one**（别名：`re:one`、`regex:one`、`regexp:one`）：`{{regen:one([a-z]{8})}}`。生成一个匹配样本，动态重算；适合只需一条。
- **regen:n**（别名：`re:n`、`regex:n`、`regexp:n`）：`{{regen:n([ab]{2}|2)}}`。取最多 n 条；当前流式实现取枚举前 n 条，不保证均匀随机。
- **repeat**：`{{repeat(abc|3)}}`。产生三条 abc；repeat(3) 产生三条空串，用于重复整份模板；不是拼成 abcabcabc。
- **repeatstr**（别名：`repeat:str`）：`{{repeatstr(abc|3)}}`。拼接为一条 abcabcabc。
- **repeat:range**：`{{repeat:range(abc|3)}}`。逐步拼接：abc、abcabc、abcabcabc；当前实现不含开头空串。
- **null**（别名：`nullbyte`）：`{{null(3)}}`。生成三条单 NUL 字节结果，不是一个含三个 NUL 的字符串。
- **crlf**：`{{crlf(3)}}`。生成三条 CRLF 结果，不是将三组 CRLF 连成一条。

## 文本处理与行选择

- **upper**：`{{upper(Abc)}}`。转大写。
- **lower**：`{{lower(Abc)}}`。转小写。
- **randomupper**（别名：`random:upper`、`random:lower`）：`{{randomupper(abc)}}`。随机改变字母大小写。
- **trim**：`{{trim(  abc  )}}`。去掉两端空白。
- **substr**：`{{substr(abcd|1,2)}}`。字节起点、可选长度，得 bc；多字节文字注意 UTF-8 字节边界。
- **split**（别名：`splitn`）：`{{split(a:b:c|:|-1)}}`。文本 | 分隔符 | 索引；从 0 计数，负数从末尾计数；越界返回空串。
- **nth**（别名：`index`、`get`）：`{{nth({{unquote(a\nb\nc)}}|1)}}`。按单次参数内的真实换行取行，零基索引，支持负数；示例得 b。
- **first**（别名：`head`）：`{{first({{unquote(a\nb)}})}}`。取单次参数的第一行。
- **last**（别名：`tail`）：`{{last({{unquote(a\nb)}})}}`。取单次参数的最后一行。
- **quote**：`{{quote(abc)}}`。用 Go strconv.Quote 加引号并转义。
- **unquote**：`{{unquote(a\nb)}}`。解析引号和反斜杠转义，示例产生真实换行。
- **padding:zero**（别名：`zeropadding`、`zp`）：`{{padding:zero(abc|5)}}`。补到指定字节长度；当前正数右补为 abc00，负数左补为 00abc；长度不得小于原串。
- **padding:null**（别名：`nullpadding`、`np`）：`{{padding:null(abc|5)}}`。补 NUL 至指定字节长度；当前正负参数都左补，与旧描述不同；长度不得小于原串。
- **jsonpath**：`{{jsonpath({{={"key":"value"}=}}|$.key)}}`。读取 JSONPath；追加 |新值 则替换。含保留符号的 JSON 用 raw。

## 编码、字符集和压缩

- **base64enc**（别名：`base64encode`、`base64e`、`base64`、`b64`）：`{{base64enc(abc)}}`。Base64 编码。
- **base64dec**（别名：`base64decode`、`base64d`、`b64d`）：`{{base64dec(YWJj)}}`。Base64 解码。
- **hexenc**（别名：`hex`、`hexencode`）：`{{hexenc(abc)}}`。十六进制编码。
- **hexdec**（别名：`hexd`、`hexdecode`）：`{{hexdec(616263)}}`。十六进制解码。
- **base64tohex**（别名：`b642h`、`base642hex`）：`{{base64tohex(YWJj)}}`。Base64 转十六进制。
- **hextobase64**（别名：`h2b64`、`hex2base64`）：`{{hextobase64(616263)}}`。十六进制转 Base64。
- **urlenc**（别名：`urlencode`、`url`）：`{{urlenc(abc)}}`。所有字符百分号编码，示例 %61%62%63。
- **urlescape**（别名：`urlesc`）：`{{urlescape(abc=)}}`。仅转义 URL 特殊字符，示例 abc%3D（十六进制大小写不影响解码）。
- **urldec**（别名：`urldecode`、`urld`）：`{{urldec(%61%62%63)}}`。URL 解码。
- **doubleurlenc**（别名：`doubleurlencode`、`durlenc`、`durl`）：`{{doubleurlenc(abc)}}`。两次 URL 强制编码。
- **doubleurldec**（别名：`doubleurldecode`、`durldec`、`durldecode`）：`{{doubleurldec(%2561%2562%2563)}}`。两次 URL 解码。
- **htmlenc**（别名：`htmlencode`、`html`、`htmle`、`htmlescape`）：`{{htmlenc(abc)}}`。十进制 HTML 实体编码。
- **htmlhexenc**（别名：`htmlhex`、`htmlhexencode`、`htmlhexescape`）：`{{htmlhexenc(abc)}}`。十六进制 HTML 实体编码。
- **htmldec**（别名：`htmldecode`、`htmlunescape`）：`{{htmldec(&#97;&#98;&#99;)}}`。HTML 实体解码。
- **unicode:encode**（别名：`unicode`、`unicode:enc`）：`{{unicode:encode(你好)}}`。JSON Unicode 转义。
- **unicode:decode**（别名：`unicode:dec`）：`{{unicode:decode(\u4f60\u597d)}}`。还原 JSON Unicode 转义。
- **gb18030**：`{{gb18030(你好)}}`。UTF-8 转 GB18030 字节。
- **gb18030toUTF8**：`{{gb18030toUTF8({{hexdec(c4e3bac3)}})}}`。GB18030 转 UTF-8。名称大小写须匹配。
- **gzip:encode**（别名：`gzip:enc`、`gzipc`、`gzip`）：`{{gzip:encode(abc)}}`。Gzip 压缩，返回二进制字节串。
- **gzip:decode**（别名：`gzip:dec`、`gzipdec`、`gzipd`）：`{{gzip:decode({{gzip:encode(abc)}})}}`。Gzip 解压。
- **zlib:encode**（别名：`zlib:enc`、`zlibc`、`zlib`）：`{{zlib:encode(abc)}}`。Zlib 压缩，返回二进制字节串。
- **zlib:decode**（别名：`zlib:dec`、`zlibdec`、`zlibd`）：`{{zlib:decode({{zlib:encode(abc)}})}}`。Zlib 解压。

## 哈希摘要

- **md5**：`{{md5(abc)}}`。MD5 摘要的十六进制结果。
- **sha1**：`{{sha1(abc)}}`。SHA1 摘要的十六进制结果；SHA-1 是单次摘要。
- **sha224**：`{{sha224(abc)}}`。SHA224 摘要的十六进制结果。
- **sha256**：`{{sha256(abc)}}`。SHA256 摘要的十六进制结果。
- **sha384**：`{{sha384(abc)}}`。SHA384 摘要的十六进制结果。
- **sha512**：`{{sha512(abc)}}`。SHA512 摘要的十六进制结果。
- **sm3**：`{{sm3(abc)}}`。SM3 摘要的十六进制结果。

## 日期与时间

- **date**：`{{date(YYYY-MM-dd,Asia/Shanghai)}}`。当前日期；可指定 Java 风格格式和时区。
- **datetime**（别名：`time`）：`{{datetime(YYYY-MM-dd HH:mm:ss,Asia/Shanghai)}}`。当前日期时间；同样支持格式和时区。
- **date:range**：`{{date:range(20260928,20260930,Asia/Shanghai)}}`。逐日枚举闭区间，尝试保持起始日期格式。
- **timestamp**：`{{timestamp(ms)}}`。当前时间戳；单位 s/ms/us/ns，默认秒。

## 词表、文件与宿主扩展

- **payload**（别名：`x`）：`{{payload(groupName)}}`。从 profile 数据库读词表，按行展开并去重；组名或 folder/*。
- **payload:nodup**：`{{payload:nodup(groupName)}}`。按行展开并保留重复项；名称 nodup 不代表去重。
- **payload:full**：`{{payload:full(groupName)}}`。整块读取，不拆行、不去重。
- **file**：`{{file(/tmp/a.txt|/tmp/b.txt)}}`。每个文件整块返回；exec_fuzztag 需 enable-file-tags:true。
- **file:line**（别名：`fileline`、`file:lines`）：`{{file:line(/tmp/words.txt)}}`。按行展开；支持 | 分隔多文件；需开启文件标签。
- **file:dir**（别名：`filedir`）：`{{file:dir(/tmp/templates)}}`。递归读取目录中文件内容为多结果；需开启文件标签。
- **codec**：`{{codec(pluginName|abc)}}`。调用已注册的 Yakit Codec 插件；第一个 | 后全部是参数。exec_fuzztag 未启用。
- **codec:line**：`{{codec:line(pluginName|abc)}}`。调用 Codec 插件后按行展开；exec_fuzztag 未启用。
- **ghostbits**：`{{ghostbits(abc|0x4E)}}`。高字节扩展编码；可选 highByte，属于 Codec 标签集；本工具未启用。
- **codecflow**：`{{codecflow(flowName|abc)}}`。执行数据库中保存的 Codec 工作流；由 codegrpc 注册，依赖本地工作流。
- **yak**：`{{yak(handle|abc)}}`。调用宿主热加载函数；仅对应宿主可用，本工具未启用。
- **yak:dyn**：`{{yak:dyn(handle|abc)}}`。动态重算热加载函数；宿主必须注入，本工具未启用。

## 文件头字节

- **ico**：`{{ico()}}`。ICO 文件头，属于字节片段，不代表完整有效图片。
- **tiff**：`{{tiff()}}`。MM 与 II 两种字节序签名，属于字节片段，不代表完整有效图片。
- **tiff:mm**：`{{tiff:mm()}}`。MM 大端签名，属于字节片段，不代表完整有效图片。
- **tiff:ii**：`{{tiff:ii()}}`。II 小端签名，属于字节片段，不代表完整有效图片。
- **bmp**：`{{bmp()}}`。BMP 文件头，属于字节片段，不代表完整有效图片。
- **gif**：`{{gif()}}`。GIF 文件头，属于字节片段，不代表完整有效图片。
- **png**：`{{png()}}`。PNG 文件头，属于字节片段，不代表完整有效图片。
- **jpg**（别名：`jpeg`）：`{{jpg()}}`。JFIF 与 Exif 两种头，属于字节片段，不代表完整有效图片。
- **jpg:jfif**（别名：`jpeg:jfif`）：`{{jpg:jfif()}}`。JFIF 头，属于字节片段，不代表完整有效图片。
- **jpg:exif**（别名：`jpeg:exif`）：`{{jpg:exif()}}`。Exif 头，属于字节片段，不代表完整有效图片。

## 专项安全数据

- **fuzz:username**（别名：`fuzz:user`）：`{{fuzz:username(admin|1)}}`。基于输入用户名和等级构造变体。
- **fuzz:password**（别名：`fuzz:pass`）：`{{fuzz:password(password|1)}}`。基于输入密码和等级构造变体。
- **headerauth**：`{{headerauth()}}`。生成回显链使用的固定 Accept-Language 头，不是通用 HTTP 认证编码器。
- **shiro:cbc**：`{{shiro:cbc(key,payload)}}`。Base64 密钥、Base64 载荷；生成 Shiro CBC 加密载荷，含随机 IV。
- **shiro:gcm**：`{{shiro:gcm(key,payload)}}`。Base64 密钥、Base64 载荷；生成 Shiro GCM 加密载荷。
- **yso:exec**：`{{yso:exec(command)}}`。生成多种 Java 反序列化命令执行载荷字节；这里只生成数据。
- **yso:dnslog**：`{{yso:dnslog(domain.example|flag)}}`。生成 DNSLog 相关链；参数为域名与前缀。
- **yso:urldns**：`{{yso:urldns(domain.example)}}`。生成 URLDNS 序列化载荷。
- **yso:find_gadget_by_dns**：`{{yso:find_gadget_by_dns(domain.example)}}`。构造用于探测类/链的 DNS 载荷。
- **yso:find_gadget_by_bomb**：`{{yso:find_gadget_by_bomb(all)}}`。类名或 all，构造用于探测链的载荷。
- **yso:headerecho**：`{{yso:headerecho(key|value)}}`。构造设置响应头的回显载荷。
- **yso:bodyexec**：`{{yso:bodyexec(command)}}`。构造 class body exec 方式的多种链。

## 查看当前版本及维护全集

本表名称和别名来自 `mutate.GetAllFuzztags()`、`FileTag()`、`CodecTag()`、`HotPatchFuzztag()` 和 `HotPatchDynFuzztag()`；codecflow 由 `common/yak/yaklib/codec/codegrpc/codec_grpc_methods.go` 注册，params/param/p 由 `Fuzz_WithParams` 注入。标签实现、参数与示例主要在 `common/mutate/fuzztag.go`，组合执行在 `common/fuzztagx/`。

Yakit 标签帮助或 gRPC `GetAllFuzztagInfo` 可查询当前运行引擎注册的名称、描述、参数和示例；它未必包含按宿主注入的 File/Codec/HotPatch/params 扩展。在线[标签手册](https://yaklang.com/docs/yakexamples/fuzztag/)和[同步原理](https://yaklang.com/en/blog/fuzztag-capability-upgrade/)用于补充，实际行为以当前源码和渲染结果为准。升级注册表时同步更新名称、别名、参数形态及本表的行为差异说明。
