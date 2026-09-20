# 旧新行为差异

基线：`d33a21b6a30945df93a663a18f3cce087b0f3718`。原测试与期望保留在基线 Git 对象和模块外 `baseline.tar`。上游测试投影只比较原先覆盖的字段；新的来源、实例与要求字段另有独立断言。所有表项是实现差异，不是声称旧基线全套测试通过：旧 POM positive2 在本机缺少隐式缓存，基线测试失败。

| 分类 | 输入/触发 | 原行为 | 当前行为与依据 |
|---|---|---|---|
| REMOVED_SCOPE | Docker/Git 入口 | 获取镜像、容器、历史材料 | 完全退出；调用方提供快照，见 MIGRATION |
| API_MIGRATION | CycloneDX 导出 | 返回第三方 SDK 对象 | 自有固定 1.5 DTO，独立离线 schema 验证 |
| BUG_FIX | 同名、异生态/来源/版本/项目 | 同名分组两层比较与猜配 | 结构化身份索引；不跨项目/快照绑定 |
| BUG_FIX | 缺少原生引用 | 从最后一个 @ 拆出虚构组件 | 保留 opaque unresolved Requirement，不添加组件 |
| BUG_FIX | dpkg OR / APK、RPM 需求 | 潜在依赖当作已安装包，可能误绑定 | 只有真实安装记录形成组件，原始 AND/OR 保留 |
| BUG_FIX | APK >=、<= | 丢失比较符 | 保留完整约束；许可证原文不拆掉 AND/OR |
| BUG_FIX | npm ^0.2.3 / ^0.0.3 | 以主版本递增生成错误上界 | 兼容展示分别限制 <0.3.0 / <0.0.4；Report 保留原约束 |
| BUG_FIX | npm 范围或无锁声明 | 范围可能进入确定版本 | 非精确版本在 Report 中未知；不产生无名根组件 |
| BUG_FIX | npm 多层 node_modules | 以名称/版本折叠实例 | 每条物理路径保留，逐级祖先查找；workspace link 有界精确解析 |
| BUG_FIX | Yarn descriptor 同版本 | name@version map 覆盖边 | descriptor 集合为原生 ID，registry source 保留 |
| BUG_FIX | Yarn realworld optionalDependencies | 三条边丢失 | jsonfile@2.4.0、jsonfile@4.0.0、klaw@1.3.1 -> graceful-fs@4.1.15；直接核对固定 fixture |
| BUG_FIX | Yarn 未知协议 | 仅日志并成功跳过 | unsupported_syntax；原先明确不扫描的非 npm 协议仍不解析为 npm 包 |
| BUG_FIX | pnpm peer 后缀 | 可能并为相同组件实例 | 原始 depPath/peer 上下文保留；冻结 v5/v6 |
| BUG_FIX | Cargo 相同 name/version 不同 source | 来源丢失、短引用任选版本 | 来源属于身份；不唯一则保留未解析引用 |
| BUG_FIX | Bundler GIT/PATH、平台包 | 同名覆盖 | 来源、revision、platform 保留；歧义目标不任选 |
| BUG_FIX | 动态 gemspec | 截断字符串可成为假版本 | 只接受冻结静态字面量，拒绝 ENV/File.read/插值/拼接 |
| BUG_FIX | pip 范围/不带版本 | 声明被忽略 | 未知版本组件/声明保留；锁定证据不伪造 |
| BUG_FIX | Python markers/extras/URL | 声明细节丢失 | 保留原始条件、extras、URL/hash；不读取环境求值 |
| BUG_FIX | Poetry 多候选 | 使用第一个候选 | 不唯一则不绑定，保留原声明 |
| BUG_FIX | Packaging Requires-Dist | 仅包元数据 | 同时保留约束和条件；MIME 部分错误转诊断 |
| BUG_FIX | Composer require 缺少目标 | 日志后丢弃 | 原始需求始终保留，不将 PHP/ext 要求伪造为安装组件 |
| BUG_FIX | Composer.json 匹配但无处理 | 返回空成功 | 读取静态声明，未知范围保留 |
| BUG_FIX | Go sum | 校验和条目形成依赖包 | 仅对准确 path/version/content-kind 关联校验；/go.mod 不作模块内容校验 |
| BUG_FIX | Go local replace | 丢弃/伪造替换版本 | 原 requirement + local source + unknown effective version |
| BUG_FIX | Go <=1.16 indirect | 按历史解析器逻辑忽略 | 声明完整保留，不声称已安装 |
| BUG_FIX | POM 相对父级坐标 | G/A/V 部分相同即可接受 | 精确匹配；候选 property 经有界展开后再比较 |
| BUG_FIX | POM 版本区间、缺属性 | 省略根或无依赖记录 | 原表达式保留，未知版本、证据不足诊断 |
| COMPATIBILITY | POM fixtures 远程仓库 | HTTP listener /本机缓存 | 同一 XML 内容搬到提供的 repository 树；无服务 |
| BUG_FIX | parent-child-properties | 相对 top-parent 与声明 parent 不符却接受，得到 api4.0 | 拒绝错坐标，显式 repository parent1.0 得 api2.0 |
| BUG_FIX | parent-dependencies | `${revision}-${changelist}` 与父 `${revision}${changelist}` 混同 | changelist=-SNAPSHOT 时二者不同；保留未解析 child，不虚构继承 |
| BUG_FIX | transitive parent license | 子许可缺失 | 显式父链 Apache-2.0 继承保留 |
| LOCAL_PATCH | POM provided | 上游预期排除，Yak 已保留 | 延续 Yak provided 记录，固定测试 adds 999 |
| BUG_FIX | Conan 畸形 ref | 仅日志并跳过 | 格式错误返回；合法 graph ID/ref/revision 不丢失 |
| BUG_FIX | 格式/预算/取消/panic | 空成功或仅日志 | 可审计诊断与不完整性；任务边界 panic -> internal_error |
| BUG_FIX | 多个 worker 耗尽共享预算 | 谁先读完决定部分结果 | 丢弃并发部分图，稳定超限诊断 |
| BUG_FIX | 重复库来源行、嵌套 JAR | 只留下第一位置/根路径 | 保留所有已识别来源区间和 `!/` 路径 |
| BUG_FIX | 虚拟文件 mode | 常规文件包含所有特殊 mode bits | 常规文件 mode=0；真实虚拟文件系统 SSA 回归 |

## 明确边界

不宣称新实现支持 TOML 所有版本、YAML 任意对象/别名/标签或完整 Maven 环境。TOML 1.1 与日期值没有用于冻结 Cargo/Poetry 字段；相关 corpus 明确拒绝。pnpm 只冻结 v5/v6，别名/anchor/merge/tag 等返回不支持。pip 的 `-r`/`-c`/安装器选项此前不形成包含图，本实现显式拒绝，未把它们当成普通包。

`RawLicenses` 是新 Report 和 SBOM 的原文依据；旧 `License` 列表保留兼容显示归一化，不能作为 SPDX 等价性判定。未知摘要不冒充合法 CycloneDX hash，保留原始 property。

JAR 上游离线 fixture 同时声明 Implementation-Title=Spring Framework 和 Bundle-Name=Spring Core；Yak 基线优先 Bundle-Name，本实现延续该优先级并把 Manifest 推导坐标标为 inferred。SHA1/artifactId 网络查询用例只断言本地证据；无本地坐标时为空，不补远程结果。显式 pom.properties 优先级不再依赖归档文件名。

## 最后边界复核

- `BUG_FIX`：块读取保留没有换行的最后一行，并跳过记录前空行；DPKG 独立最小记录以有/无终止换行得到相同版本。
- `BUG_FIX`：DPKG 缺失 Status 或已安装记录缺失名称/版本时返回诊断。旧 negative-dpkg 输入和空组件期望保留，增加错误断言；不再把损坏材料当空成功。
- `BUG_FIX`：POM 缺父级且零组件时仍返回 evidence_insufficient；不依赖存在第一条组件才传递诊断。
- `RESOURCE`：RPM 单独读入限制与扫描配置取较小值；Header 字段长度与遍历步数检查在复制字符串前执行，内部循环检查取消。文件路径等未知字段仍验证语法边界，计入遍历步数而非组件表达式节点数。
- `COMPATIBILITY`：elfsplit 的已生成分组表只移除两个已经删除的 SCA 包名；新算法包保持保守默认分组。未在缺少原始未拆分 ELF 的情况下伪造重新生成的分组。
