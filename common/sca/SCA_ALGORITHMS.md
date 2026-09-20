# SCA 最小算法与边界

基线 `d33a21b6a30945df93a663a18f3cce087b0f3718`。保留能力与实际语法见 `function-contracts.json`；源码归属及逐文件符号见 `source-extraction-map.json`。保留纯算法并不表示整个上游框架被内嵌。

## 输入、发现、调度

`pipeline.go` 先规范化配置，再按稳定逻辑路径建立文件目录，显式匹配所有选定分析器并按名称排序。同一路径可对应多个独立分析器。材料读取一次缓存，解析器取得独立游标的只读内存句柄；不写临时文件。文件类型、打开前后信息、长度、读错误都进入诊断。快照标识可由调用方提供，或由实际读取材料的路径和摘要确定。

目录、候选文件、文件大小、累计实际读取量有界；只排入受控任务。worker 数 1..64，结果槽按稳定任务索引写入，等待 worker 退出后合并。解析错误、取消与任务边界 panic 不能消失。共享读取/归档预算耗尽时舍弃并发部分图，避免结果随 worker 调度变化。受信任自定义回调不能被 Go 强制终止，调用方负责硬隔离。

发现 O(F log F)，缓存 O(B)，任务执行成本为各解析器成本之和。读取预算包含快照捕获和解析器实际 Read/ReadAt，重复读取也计费。不是通过忽略 IO 来声称单次读。

## 文本与结构

- `jsonrecord`：标准库 token 解码，UseNumber 保留 ID 数字，拒绝重复语义键、尾随数据、错误 UTF-8；记录对象位置。按输入字节、深度、节点、字段长度限额。在固定 DTO 转换之前先完成校验。
- `xmlrecord`：标准 XML token guard 限制深度、token、属性和文本；无外部实体加载。UTF-8、BOM UTF-16、ASCII、Latin-1 为明确编码分支，不带 HTML 通用编码框架。
- `locktoml`：从固定 BurntSushi 源码保留词法和标量原语，重写私有 table/array-table 结构节点，拒绝重复表、冲突键、日期和未冻结的 TOML 1.1；没有反射绑定器、encoder、环境开关。Cargo 的记录行号直接在同一次 token 解析中取得，删除旧二次行扫描。
- `lockyaml`：只实现 pnpm v5/v6 需要的映射、序列、flow、引号、转义、注释及标量。anchor/alias/tag/merge/多文档/未冻结结构显式不支持；不半解析后成功。
- `pyrequire`：有限 Python 声明词法；保留 pin/range/extras/URL/hash/marker。环境条件只作原文保存，不根据宿主环境求值。

一般词法 O(B)，结构表索引平均 O(N)，排序 O(N log N)；B 包含字符串长度，不视作常数。标准库编码到固定 DTO 会形成一次有界附加表示。所有当前默认值和部分按解析器计数的预算作用域在 `resource-policy.json` 中说明。

## 组件、观察与关系

ComponentKey 由生态、名称、实际版本、来源、架构、变体和校验构成；结构化 JSON 后摘要避免拼接边界冲突。未知版本为空；同名不是同一身份。缺摘要不与任意带摘要记录等价。

Observation 单独保留快照/项目/文件/原生 ID/来源区间/证据类别。原生引用按材料文件加命名空间，不能跨文件或项目通过 name@version 猜配。Requirement 保留 raw constraint、scope、condition、AND/OR 和已证实 resolved 引用。只有实例索引确认的关系才成为依赖边。

已安装 OS 记录的 capability 建多值索引；候选集合与 resolved 区别明确。多提供者不选最后一项。候选输出数量受边预算控制，复杂度计入候选输出 C；不承诺所有关系是固定 O(1)。RPM release/epoch、架构保留，未知版本约束不做跨生态比较。

归一化为平均 O(V+E+C) 建索引，再 O(V log V+E log E) 稳定排序。兼容 `MergePackages` 同样只做精确身份索引，移除同名全对全版本试探。图保留环，SBOM 用显式队列和访问集合，不依赖拓扑排序。

## 格式语义

| 格式 | 核心算法 / 不变量 | 边界与失败 |
|---|---|---|
| npm | 物理路径索引、最近祖先 node_modules 查找、显式 link 链 | 限制路径与链深度；循环/越界错误，缺目标保留需求 |
| pnpm | 原始 depPath 为实例，peer 后缀保留 | 只支持 v5/v6；不折叠 peer 实例 |
| Yarn | descriptor 集合索引、明确引用绑定 | 保留不同来源/descriptor，同一 descriptor 冲突错误 |
| Cargo | 一次 TOML 解析 + name/version/source 身份 | 短引用只有唯一候选才绑定；歧义不任选 |
| Bundler | section 与缩进状态机 | 来源、revision、platform 单独保留；动态不求值 |
| GemSpec | 只读静态字符串赋值 | Ruby 插值/表达式/外部读取拒绝 |
| Pip/Pipenv/Poetry | 固定 JSON/TOML/声明记录 | 条件和范围不成为已安装事实，缺候选不补下载 |
| Packaging | 固定发行元数据 + Requires-Dist | MIME 部分错误保留诊断，不导入 Python |
| Composer | 固定 JSON records + require 引用 | PHP/ext 作为需求；重复实例报错，缺目标不伪造 |
| POM | XML、有限属性展开、显式父/BOM/模块树 | parent/module/property 循环和操作预算；G/A/V 精确匹配 |
| Gradle | 固定 group:name:version=scope 分词校验 | 非法/超长行和 scanner 错误返回 |
| JAR | ZIP ReaderAt、强 pom.properties 优先、Manifest 次级证据 | 名称不决定身份；有界递归、条目数与展开量；包含路径不当依赖 |
| Go mod | 提取的只读 lexer + 固定指令 grammar | require/replace/indirect 原文证据；不构建 MVS 图 |
| Go sum | 完整 path/version/content-kind 校验索引 | 无独立安装组件；冲突摘要报错 |
| Go binary | 标准 debug/buildinfo.Read | 不执行，不要求真实宿主路径 |
| Conan | 固定 node ID/ref 记录映射 | revision/source 保留；畸形 ref 报错 |
| DPKG/APK | 字段记录、安装状态、AND/OR/capability 分离 | 不调用包管理器，不创造潜在安装包 |

POM 解析基于显式材料的有限 Maven 语义，不宣称覆盖任意 Maven 插件、profile 或完整远程依赖求解。Cargo 按 name、name/version、name/version/source 建三个精确索引，短引用只在候选数为一时绑定，避免对每条边重复扫描整个同名集合。

## RPM 三个后端

BDB 核对 magic/端序/页大小、hash 页目录和 overflow 链。NDB 核对槽、blob 尺寸、序号及 Adler 校验。SQLite 核对文件头、编码、sqlite_schema 中固定 Packages 表、表 B-tree 页、varint、记录 serial type 和 overflow 链。只提取 rpm Header blob，无 SQL、事务、写入、恢复、驱动注册或动态加载。

后端共享有界 ReaderAt 和单一 Header 解码：count/offset/type/字符串终止符/整数尺寸都验证后读取。显式 page-visit/read-byte/record/depth 限额截断循环或异常展开。16 个固定布局逐字段比对隔离旧 reader，另比对上游 rpm -qa 静态期望；BDB/NDB/SQLite 完整种子及 Header 语义种子进入 fuzz。

## 许可证与 SBOM

RawLicenses 保留原表达式。兼容 License 显示映射与证据分离，不能作为法律或 SPDX 等价判断。固定 CycloneDX 1.5 DTO 只写所需字段。合法摘要检查算法及精确长度；未知值转原文 property。依赖是稳定引用集合，循环不会造成递归展开，多个观察实例贡献的边合并到组件级集合。固定上游 schema 和独立 jsonschema 工具在产品模块外离线验证。
