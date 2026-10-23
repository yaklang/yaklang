# coremail jsp+jar：249 条逐文件错误台账

## 当前结论（2026-10-23 审计修正）

**未完成。** 最近全量为 `051047`：语法失败 25，exit=1，stub 1635。其后只做了定向重放，没有新的全量验收。没有以重放或测试代替验收；未跑 ekp、未 push。

上一轮“当前只剩约 5 条”已撤回：13 个 legacy `valueOf(String)` 被错误地显式写进 enum，javac 报 duplicate method；23 个缺词法元数据类曾被随意补无界泛型，可能改变物理描述符。这两条放行已撤回，原协议反例仍拒绝。

### 新证据（不是全量验收）

- `audit/ReservedFactory.javac.log`：显式 enum valueOf(String) javac exit=1，method already defined。13 条目前保持拒绝，尚未加入任何新 suppression。
- `audit/obscure4/plain.log`：`class Plain extends Enum<Plain>` 被 javac 拒：`classes cannot directly extend java.lang.Enum`（构造器 `Enum` 也无法调用）。所以这 13 个类既不能作为 enum 声明 `valueOf(String)`（重复声明），也不能作为普通类继承 `Enum`；在当前源码层面确实不可表达。仍未加 suppression，计入 syntax_failures。
- `TestEnumLegacyValueOfCannotBeSilentlyReplaced`：有效的原字节码运行确认 null/缺名均为 IllegalArgumentException，不能静默换成默认 Enum.valueOf；新测试要求拒绝而非非法源码。
- `TestMissingEnclosingMethodPreservesPhysicalDescriptors`：RED→GREEN，`none` 与 `source,lines,vars` 两种编译元数据；改用原始 descriptor 渲染并保留必要的参数桥；javac 重编译和 Java driver（反射检查 Bound 参数/返回）一致。不是为 T 猜测 Object bound。
- `audit/replay-descriptor`：20 个无 EnclosingMethod 的原文件生成，3 个 method_index=0 且无构造器的 Comparators$1 继续拒绝。20 个中 LinkedHashCache$1 仍含一个旧构造器 stub，不能算完全修复。
- 枚举 clinit 的改名 backing 赋值泄漏已针对修复，3 条（es/dq/ez）在本表重开，等待新全量。
- 2026-10-23 04:4x 重放（不是全量）：`Comparison` 已生成，六个比较常量折回常量体，方向与字节码一致。证书允许常量体保留 static 自身行（可带 ACC_ENUM）和指向共享 marker 的第二行。`TestRenamedEnumConstantBody` exit 0。
- `NestingType`、`CompilePhaseAdapter` 仍拒绝。`<clinit>` 用 `CallSite.callStatic` 加 `castToType` 赋值，没有声明处的 `new`。JLS 16.5：enum 常量是用实例创建表达式初始化的 static final 字段。未加 suppression。
- 15 条静态归属重放：13 条已生成。`a/ct` 与 `module/n` 仍拒绝。本机 javac：限定名、static import、两个单类型 import 都非法；另一个同名类型自己也不能写限定名。简单名被占时不再把调用绑到错误类型。
- 全量 `051047`（二进制 `b8d01f6063a4c69b…`，exit=1）：语法失败 54→25，空单元 0，FrontendRaw 失败 0，stub 1635。25 条构成：jaxb 编译器协议 13、常量体身份 5（Comparison 两份、es、dq、ez）、Comparators$1 三份、静态归属 2、Groovy 构造来源 2。未新增 suppression。
- 2026-10-23 12:1x 子集端到端对比（不是全量验收）：`audit/subset-const5/src` 放四个 jar（wmsvr/apiws/mailsms/webinst），同一份输入分别跑旧二进制 `b8d01f60…`（051047 那个）和新二进制 `783be31b…`（含 `3a31605`）。
  - 旧：`files=3100 syntax_failures=4 decompiler_refusals=4`，四条分别是 `apiws.jar/es.java`、`mailsms.jar/mt/dq.java`、`webinst.jar/ez.java`、`wmsvr.jar/.../Comparison.java`，错误都是 `constant-specific enclosing identity`。
  - 新：`files=3100 syntax_failures=0 decompiler_refusals=0 empty_units=0 frontend_failures=0`，stub 两边都是 83 方法/74 文件。
  - 证据：`audit/subset-const5/run-old/verification.json`、`audit/subset-const5/run/verification.json`。退出码仍为 1（stub>0），不当作验收。
  - 再把 `webapp/WEB-INF/lib/wmsvr.jar` 作为 `wmsvr-webapp.jar` 加进同一子集（`run2`，4870 文件）：`syntax_failures=2`，两条正是 javac 已证明写不出的 `a/ct.java` 与 `com/tebie/cmapp/ajxvr/module/n.java`；`es`、`dq`、`ez`、两份 `Comparison` 均已通过。stub 135 方法/74+ 文件。证据 `audit/subset-const5/run2/verification.json`。
- 2026-10-23 11:2x 定向重放（不是全量）：5 条常量体身份已生成，提交 `3a31605`。混淆器把常量体改成无关类名并删掉 InnerClasses/EnclosingMethod 后，共享 marker 只留在构造器描述符的 `L<marker>;` 里，常量池没有对应 CONSTANT_Class。`eu.class`（266 字节）即此形状。无嵌套行时不再要求 marker 的 Class 项；有行时仍拒绝指向其它类的行。`TestRenamedEnumConstantBodyWithoutNestingRows` 在去掉这条放行时失败、恢复后通过（javac 重编译 + driver）。`audit/replay-const5-fix2`：Comparison 两份、es、dq、ez 均为 emitted，LE/LT 方向与字节码一致。jaxb 7 条仍拒绝。未跑新全量，台账勾选不改。
- 同日 javac `--release 8`（`audit/obscure3`）：`a/ct` 要同时写出 `cn.mailtech.utils.g.a` 和返回类型 `cn.mailtech.cm.preview.g`。同包类 `a.cn` 让这两条限定名都非法；两个单类型 import 报 `a type with the same simple name is already defined`。只 import `utils.g` 时，限定的 `preview.g` 是 `cannot find symbol`。`module/n` 的目标包根 `a` 被同包类 `module/a` 挡住，编译单元自己又声明了 `n`，不能再 import `a.n`。这两条保持拒绝，没有放宽证书。

证据根：`/mnt/data/scan-outputs/coremail-2b-v2-20261023/audit/`。原不安全工作区补丁及测试保存在 workspace `build/backup/coremail-2b-audit-20261023-0406/`，未删除历史证据。

## 全量运行记录

| run | files/checked | materialization errors | syntax failures | stub methods | exit | prepare s | verify s | process wall s | peak RSS KiB |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 2008 | 41765/41765 | 0 | 249 | 1634 | 1 | 1539.668 | 758.177 | 2695.2 | 见原 run |
| 030517 | 41765/41765 | 0 | 54 | 1634 | 1 | 1682.451 | 696.069 | 2667.653 | 9312860 |
| 051047 | 41765/41765 | 0 | 25 | 1635 | 1 | 1696.754 | 763.500 | 2746.081 | 9457064 |

- run：`/mnt/data/scan-outputs/coremail-2b-v2-20261023-030517`
- binary SHA256：`8f58bd0b662ac3a0ac3530a195f030e996ceef670ea631ef1e4fe8e09e75ea6d`
- yaklang：`c05686dcc470d39349ef673ab6fb83a9791595e1`；javajive：`0ba60eec4e1c920c6b8e1bda9c883dca75deea3b`。WIP 指纹与文件内容见 run 内 diff/untracked/manifest，不将 HEAD 当作完整源码指纹。
- 输入清单 SHA256：`874cbe3f514d6cca5d63ca6056d23871dae279e91621351dbf314f20f373de54`，2900 文件，180696930 字节。
- 命令：`/mnt/data/scan-outputs/coremail-2b-v2-20261023/materialize --debug /mnt/data/scan-outputs/coremail-2b-v2-20261023-030517 /home/wlz/Target/coremail`
- 环境：GOMAXPROCS=16 / GOGC=100 / GOMEMLIMIT=12GiB / YAK_SSA_SOURCE_CACHE=0；TMPDIR、YAKIT_HOME 均为 run 独立子目录。
- 峰值 RSS 仅为本进程；其他大进程和系统可用内存只记在 manifest.environment_baseline，不算本进程性能。
- CPU/heap/allocs/goroutine/mutex/block 与 goroutine-debug2 见 run/profiles；二进制构建信息见 binary-build-info.txt。

## 逐文件清单

勾选含义：原失败已在最近全量中消失且未被本轮审计重开；不是对整个项目宣告完成。未勾选行必须再次全量验证。所有路径均为 `/home/wlz/Target/coremail/` 下的虚拟源码路径。

| 结论 | 编号 | 文件 | 根因/问题编号 | 最新证据/状态 |
|---|---|---|---|---|
| [ ] | CM-001 | `CoremailSync.war/WEB-INF/lib/wmsvr.jar/a/dK.java` | J2B-017：static owner 的类型/方法命名空间冲突 | OPEN：030517 拒绝；source method a.dK.b(Ljava/io/File;Ljava/io/File;)Lcn/mailtech/cm/preview/h;: static owner cn.mailtech.utils.g.a(Ljava/lang/String;Ljava/lang/String;)Ljava/lang |
| [ ] | CM-002 | `CoremailSync.war/WEB-INF/lib/wmsvr.jar/a/dv.java` | J2B-017：static owner 的类型/方法命名空间冲突 | OPEN：030517 拒绝；source method a.dv.a(Ljava/lang/String;Ljava/lang/String;)Z: static owner cn.mailtech.utils.g.a(Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String; cannot b |
| [x] | CM-003 | `CoremailSync.war/WEB-INF/lib/wmsvr.jar/a/l.java` | J2B-002：同包类型被包首段遮蔽 | 030517 全量 java 通过；SHA256 8b1bfd3c02d2a05e |
| [ ] | CM-004 | `CoremailSync.war/WEB-INF/lib/wmsvr.jar/com/tebie/cmapp/ajxvr/lib/B.java` | J2B-017：static owner 的类型/方法命名空间冲突 | OPEN：030517 拒绝；source method com.tebie.cmapp.ajxvr.lib.B.a(Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;: static owner a.g.a(Ljava/lang/String;Ljava/lang/String;)Lja |
| [ ] | CM-005 | `CoremailSync.war/WEB-INF/lib/wmsvr.jar/com/tebie/cmapp/ajxvr/lib/i.java` | J2B-017：static owner 的类型/方法命名空间冲突 | OPEN：030517 拒绝；source method com.tebie.cmapp.ajxvr.lib.i.a(Lcom/tebie/cmx/mail/k;Ljava/lang/String;Z[Ljava/lang/String;I)La/cl;: static owner a.e.a(Lcom/tebie/cmx/Y;Ljava/lang |
| [ ] | CM-006 | `CoremailSync.war/WEB-INF/lib/wmsvr.jar/com/tebie/cmapp/ajxvr/module/D.java` | J2B-017：static owner 的类型/方法命名空间冲突 | OPEN：030517 拒绝；source method com.tebie.cmapp.ajxvr.module.D.b(Ljava/lang/String;)Ljava/lang/String;: static owner a.g.a(Ljava/lang/String;)Ljava/lang/String; cannot be bound:  |
| [x] | CM-007 | `CoremailSync.war/WEB-INF/lib/wmsvr.jar/com/tebie/commons/search/Comparison.java` | J2B-010/enclosing：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 a7ea0fe75ca44c39 |
| [x] | CM-008 | `apiws.war/WEB-INF/lib/apiws.jar/cB.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 8503bda9683894d2 |
| [x] | CM-009 | `apiws.war/WEB-INF/lib/apiws.jar/cE.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 ac6cefa7fdc03b06 |
| [x] | CM-010 | `apiws.war/WEB-INF/lib/apiws.jar/cG.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 442e06e930769673 |
| [x] | CM-011 | `apiws.war/WEB-INF/lib/apiws.jar/cN.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 3100a4caa621b153 |
| [x] | CM-012 | `apiws.war/WEB-INF/lib/apiws.jar/cT.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 f9a2ee6ba50a8ddc |
| [x] | CM-013 | `apiws.war/WEB-INF/lib/apiws.jar/cY.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 53cc2b92c36daa87 |
| [x] | CM-014 | `apiws.war/WEB-INF/lib/apiws.jar/ck.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 a5d47e690838c020 |
| [x] | CM-015 | `apiws.war/WEB-INF/lib/apiws.jar/cm.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 20d386c53f3c63ac |
| [x] | CM-016 | `apiws.war/WEB-INF/lib/apiws.jar/cn.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 800dd22e4d377e3f |
| [x] | CM-017 | `apiws.war/WEB-INF/lib/apiws.jar/cn/coremail/syncApiws/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 0734647a87b294c6 |
| [x] | CM-018 | `apiws.war/WEB-INF/lib/apiws.jar/cp.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 56e5cd2a25f0b450 |
| [x] | CM-019 | `apiws.war/WEB-INF/lib/apiws.jar/dd.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 dafb12da3645c593 |
| [x] | CM-020 | `apiws.war/WEB-INF/lib/apiws.jar/dg.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 3166eff129f1e8ff |
| [x] | CM-021 | `apiws.war/WEB-INF/lib/apiws.jar/do.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 2b158441d6d2d92c |
| [x] | CM-022 | `apiws.war/WEB-INF/lib/apiws.jar/ed.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 4129b37da662bada |
| [ ] | CM-023 | `apiws.war/WEB-INF/lib/apiws.jar/es.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | REOPEN：FrontendRaw 通过但 backing 赋值泄漏；定向修复待新全量 |
| [x] | CM-024 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/bindings/xformat/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 05103a44d99ccec2 |
| [x] | CM-025 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/bus/BusState.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 4fb49a77ca07812e |
| [x] | CM-026 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/configuration/security/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 09b46cd87cc0b7cd |
| [x] | CM-027 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/endpoint/ManagedEndpoint$State.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 4f9d9fb72dedbbe3 |
| [x] | CM-028 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/headers/Header$Direction.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 c77f3d45753af4ab |
| [x] | CM-029 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/interceptor/InterceptorChain$State.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 d5019db075e183f3 |
| [x] | CM-030 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/jaxb/JAXBUtils$IdentifierType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 6e655f8b1c3f37af |
| [x] | CM-031 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/jaxws/javaee/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 03514348a65b43ba |
| [x] | CM-032 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/management/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 ecd72d94ae643f9e |
| [x] | CM-033 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/message/FaultMode.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 36890e22a9186820 |
| [x] | CM-034 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/tools/common/model/JavaType$Style.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 8ebf1a8381e2d36b |
| [x] | CM-035 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/tools/java2wsdl/processor/FrontendFactory$Style.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 7625017694a5550a |
| [x] | CM-036 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/tools/plugin/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 d8400df6c645ade3 |
| [x] | CM-037 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/tools/wsdlto/frontend/jaxws/processor/internal/ServiceProcessor$BindingType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 4dd632fa6f16b8e3 |
| [x] | CM-038 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/transport/jms/DestinationStyleType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 c8c77f5b4cfa1d7b |
| [x] | CM-039 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/transport/jms/MessageFormatType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 57de295cf021de17 |
| [x] | CM-040 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/transport/jms/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 70e3e64a2f5673a7 |
| [x] | CM-041 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/transports/http/configuration/ClientCacheControlType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 a1a67f4135216e6d |
| [x] | CM-042 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/transports/http/configuration/ConnectionType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 a99e684f56f85315 |
| [x] | CM-043 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/transports/http/configuration/ProxyServerType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 abce9eb3607b2969 |
| [x] | CM-044 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/transports/http/configuration/ServerCacheControlType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 88942ddc8751a514 |
| [x] | CM-045 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/transports/http/configuration/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 543b8081813b9e7f |
| [x] | CM-046 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/transports/http_jetty/configuration/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 0ed6143cfc568ed7 |
| [x] | CM-047 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/workqueue/WorkQueueManager$ThreadingModel.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 b43450e2dd0844ee |
| [x] | CM-048 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/ws/addressing/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 11110196bee113fa |
| [x] | CM-049 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/ws/addressing/v200408/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 4703cc9a0bb1407f |
| [x] | CM-050 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/ws/addressing/wsdl/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 c3523dfe1eeac5fa |
| [x] | CM-051 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/ws/policy/v200409/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 ddb783e6daadabe2 |
| [x] | CM-052 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/ws/policy/v200607/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 2d912a05bbccc67e |
| [x] | CM-053 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/ws/rm/manager/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 a518139ce066b94c |
| [x] | CM-054 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/ws/rm/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 8b1bd88cd2aceadd |
| [x] | CM-055 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/ws/rm/policy/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 70e4835d66d87819 |
| [x] | CM-056 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/wsdl/WSDLConstants$WSDLVersion.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 790c3245e9f55f7d |
| [x] | CM-057 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/apache/cxf/wsdl/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 a881eb814ff64003 |
| [x] | CM-058 | `apiws.war/WEB-INF/lib/cxf-2.0.2-incubator.jar/org/xmlsoap/schemas/wsdl/http/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 39b10e1d90ea8524 |
| [x] | CM-059 | `apiws.war/WEB-INF/lib/geronimo-annotation_1.0_spec-1.1.jar/javax/annotation/Resource$AuthenticationType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 83b3e8c12557ee2d |
| [x] | CM-060 | `apiws.war/WEB-INF/lib/geronimo-ws-metadata_2.0_spec-1.1.1.jar/javax/jws/WebParam$Mode.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 6fac95c9cabc80d6 |
| [x] | CM-061 | `apiws.war/WEB-INF/lib/geronimo-ws-metadata_2.0_spec-1.1.1.jar/javax/jws/soap/SOAPBinding$ParameterStyle.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 9f818d714c023d3e |
| [x] | CM-062 | `apiws.war/WEB-INF/lib/geronimo-ws-metadata_2.0_spec-1.1.1.jar/javax/jws/soap/SOAPBinding$Style.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 9f818d714c023d3e |
| [x] | CM-063 | `apiws.war/WEB-INF/lib/geronimo-ws-metadata_2.0_spec-1.1.1.jar/javax/jws/soap/SOAPBinding$Use.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 9f818d714c023d3e |
| [x] | CM-064 | `apiws.war/WEB-INF/lib/jaxb-api-2.0.jar/javax/xml/bind/annotation/XmlAccessOrder.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 91218fddc1a6ded3 |
| [x] | CM-065 | `apiws.war/WEB-INF/lib/jaxb-api-2.0.jar/javax/xml/bind/annotation/XmlAccessType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 088172842589379a |
| [x] | CM-066 | `apiws.war/WEB-INF/lib/jaxb-api-2.0.jar/javax/xml/bind/annotation/XmlNsForm.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 af1090d01e6535c8 |
| [ ] | CM-067 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/Messages.java` | J2B-016：编译器 backing 数组被 legacy valueOf 读取 | OPEN：030517 拒绝；enum regeneration: original backing array is unproved: source-erased backing field escapes its generated factory |
| [ ] | CM-068 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/model/annotation/Messages.java` | J2B-016：编译器 backing 数组被 legacy valueOf 读取 | OPEN：030517 拒绝；enum regeneration: original backing array is unproved: source-erased backing field escapes its generated factory |
| [ ] | CM-069 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/model/core/ID.java` | J2B-016：编译器 backing 数组被 legacy valueOf 读取 | OPEN：030517 拒绝；enum regeneration: original backing array is unproved: source-erased backing field escapes its generated factory |
| [ ] | CM-070 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/model/core/PropertyKind.java` | J2B-016：编译器 backing 数组被 legacy valueOf 读取 | OPEN：030517 拒绝；enum regeneration: original backing array is unproved: source-erased backing field escapes its generated factory |
| [ ] | CM-071 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/model/core/WildcardMode.java` | J2B-016：编译器 backing 数组被 legacy valueOf 读取 | OPEN：030517 拒绝；enum regeneration: original backing array is unproved: source-erased backing field escapes its generated factory |
| [x] | CM-072 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/model/core/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 015b0e26966d117d |
| [ ] | CM-073 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/model/impl/ClassInfoImpl$PropertyGroup.java` | J2B-016：编译器 backing 数组被 legacy valueOf 读取 | OPEN：030517 拒绝；enum regeneration: original backing array is unproved: source-erased backing field escapes its generated factory |
| [ ] | CM-074 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/model/impl/ClassInfoImpl$SecondaryAnnotation.java` | J2B-016：编译器 backing 数组被 legacy valueOf 读取 | OPEN：030517 拒绝；enum regeneration: original backing array is unproved: source-erased backing field escapes its generated factory |
| [ ] | CM-075 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/model/impl/Messages.java` | J2B-016：编译器 backing 数组被 legacy valueOf 读取 | OPEN：030517 拒绝；enum regeneration: original backing array is unproved: source-erased backing field escapes its generated factory |
| [x] | CM-076 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/model/runtime/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 b878077ee8f1ad01 |
| [ ] | CM-077 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/runtime/Messages.java` | J2B-016：编译器 backing 数组被 legacy valueOf 读取 | OPEN：030517 拒绝；enum regeneration: original backing array is unproved: source-erased backing field escapes its generated factory |
| [ ] | CM-078 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/runtime/property/Messages.java` | J2B-016：编译器 backing 数组被 legacy valueOf 读取 | OPEN：030517 拒绝；enum regeneration: original backing array is unproved: source-erased backing field escapes its generated factory |
| [ ] | CM-079 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/runtime/reflect/Messages.java` | J2B-016：编译器 backing 数组被 legacy valueOf 读取 | OPEN：030517 拒绝；enum regeneration: original backing array is unproved: source-erased backing field escapes its generated factory |
| [ ] | CM-080 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/runtime/unmarshaller/Messages.java` | J2B-016：编译器 backing 数组被 legacy valueOf 读取 | OPEN：030517 拒绝；enum regeneration: original backing array is unproved: source-erased backing field escapes its generated factory |
| [ ] | CM-081 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/schemagen/Form.java` | J2B-016：编译器 backing 数组被 legacy valueOf 读取 | OPEN：030517 拒绝；enum regeneration: original backing array is unproved: source-erased backing field escapes its generated factory |
| [x] | CM-082 | `apiws.war/WEB-INF/lib/jaxb-impl-2.0.5.jar/com/sun/xml/bind/v2/schemagen/xmlschema/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 84a605e2f8280488 |
| [x] | CM-083 | `apiws.war/WEB-INF/lib/jaxws-api-2.0.jar/javax/xml/ws/Service$Mode.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 5dcb6f961d0b0042 |
| [x] | CM-084 | `apiws.war/WEB-INF/lib/jaxws-api-2.0.jar/javax/xml/ws/handler/MessageContext$Scope.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 1a2796afb8169d0d |
| [x] | CM-085 | `apiws.war/WEB-INF/lib/spring-beans-2.0.4.jar/org/springframework/beans/factory/annotation/Autowire.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 ae833a56a94e7cb4 |
| [ ] | CM-086 | `gtadmin.war/WEB-INF/lib/webadmin.jar/com/tebie/commons/collections/Comparators$1.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for com/tebie/commons/collections/Comparators$1 |
| [ ] | CM-087 | `icmagent.war/WEB-INF/lib/webadmin.jar/com/tebie/commons/collections/Comparators$1.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for com/tebie/commons/collections/Comparators$1 |
| [x] | CM-088 | `mailsms.war/WEB-INF/lib/mailsms.jar/mt/bR.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 9109f71307ca1167 |
| [x] | CM-089 | `mailsms.war/WEB-INF/lib/mailsms.jar/mt/bT.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 feb7ced8db50591a |
| [x] | CM-090 | `mailsms.war/WEB-INF/lib/mailsms.jar/mt/cB.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 0c0d43b08e7fddc4 |
| [x] | CM-091 | `mailsms.war/WEB-INF/lib/mailsms.jar/mt/ce.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 1fd7fae254a3ded9 |
| [x] | CM-092 | `mailsms.war/WEB-INF/lib/mailsms.jar/mt/cl.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 35842dafd2d861e7 |
| [x] | CM-093 | `mailsms.war/WEB-INF/lib/mailsms.jar/mt/co.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 a21adf7286f2d4c5 |
| [x] | CM-094 | `mailsms.war/WEB-INF/lib/mailsms.jar/mt/ct.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 778432d4dc865af7 |
| [x] | CM-095 | `mailsms.war/WEB-INF/lib/mailsms.jar/mt/dL.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 8c632cb8cfe904f9 |
| [x] | CM-096 | `mailsms.war/WEB-INF/lib/mailsms.jar/mt/db.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 7025cf8ca9e2b97e |
| [ ] | CM-097 | `mailsms.war/WEB-INF/lib/mailsms.jar/mt/dq.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | REOPEN：FrontendRaw 通过但 backing 赋值泄漏；定向修复待新全量 |
| [x] | CM-098 | `mailsms.war/WEB-INF/lib/mailsms.jar/mt/fQ.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 f632d104f1d8fa88 |
| [x] | CM-099 | `mailsms.war/WEB-INF/lib/mailsms.jar/mt/fR.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 f9f6b74ac4bf885d |
| [x] | CM-100 | `mailsms.war/WEB-INF/lib/mailsms.jar/mt/fu.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 e1ac84d0d2ca7f39 |
| [ ] | CM-101 | `webadmin.war/WEB-INF/lib/webadmin.jar/com/tebie/commons/collections/Comparators$1.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for com/tebie/commons/collections/Comparators$1 |
| [ ] | CM-102 | `webapp/WEB-INF/lib/wmsvr.jar/a/af.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for a/af |
| [ ] | CM-103 | `webapp/WEB-INF/lib/wmsvr.jar/a/ct.java` | J2B-017：static owner 的类型/方法命名空间冲突 | OPEN：030517 拒绝；source method a.ct.b(Ljava/io/File;Ljava/io/File;)Lcn/mailtech/cm/preview/g;: static owner cn.mailtech.utils.g.a(Ljava/lang/String;Ljava/lang/String;)Ljava/lang |
| [ ] | CM-104 | `webapp/WEB-INF/lib/wmsvr.jar/a/dP.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for a/dP |
| [ ] | CM-105 | `webapp/WEB-INF/lib/wmsvr.jar/a/dQ.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for a/dQ |
| [ ] | CM-106 | `webapp/WEB-INF/lib/wmsvr.jar/cn/mailtech/lib/cache/LinkedHashCache$1.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for cn/mailtech/lib/cache/LinkedHashCache$1 |
| [ ] | CM-107 | `webapp/WEB-INF/lib/wmsvr.jar/cn/mailtech/lib/cache/c.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for cn/mailtech/lib/cache/c |
| [ ] | CM-108 | `webapp/WEB-INF/lib/wmsvr.jar/cn/mailtech/lib/cache/d.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for cn/mailtech/lib/cache/d |
| [ ] | CM-109 | `webapp/WEB-INF/lib/wmsvr.jar/cn/mailtech/wmsvr/modules/a.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for cn/mailtech/wmsvr/modules/a |
| [ ] | CM-110 | `webapp/WEB-INF/lib/wmsvr.jar/cn/mailtech/wmsvr/modules/b.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for cn/mailtech/wmsvr/modules/b |
| [ ] | CM-111 | `webapp/WEB-INF/lib/wmsvr.jar/cn/mailtech/wmsvr/modules/c.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for cn/mailtech/wmsvr/modules/c |
| [ ] | CM-112 | `webapp/WEB-INF/lib/wmsvr.jar/cn/mailtech/wmsvr/modules/d.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for cn/mailtech/wmsvr/modules/d |
| [ ] | CM-113 | `webapp/WEB-INF/lib/wmsvr.jar/cn/mailtech/wmsvr/modules/e.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for cn/mailtech/wmsvr/modules/e |
| [ ] | CM-114 | `webapp/WEB-INF/lib/wmsvr.jar/com/tebie/cmapp/ajxvr/lib/i.java` | J2B-017：static owner 的类型/方法命名空间冲突 | OPEN：030517 拒绝；source method com.tebie.cmapp.ajxvr.lib.i.a(Lcom/tebie/cmx/mail/k;Ljava/lang/String;Z[Ljava/lang/String;I)La/bm;: static owner a.d.a(Lcom/tebie/cmx/E;Ljava/lang |
| [ ] | CM-115 | `webapp/WEB-INF/lib/wmsvr.jar/com/tebie/cmapp/ajxvr/lib/y.java` | J2B-017：static owner 的类型/方法命名空间冲突 | OPEN：030517 拒绝；source method com.tebie.cmapp.ajxvr.lib.y.a(Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;: static owner a.f.a(Ljava/lang/String;Ljava/lang/String;)Lja |
| [ ] | CM-116 | `webapp/WEB-INF/lib/wmsvr.jar/com/tebie/cmapp/ajxvr/module/n.java` | J2B-017：static owner 的类型/方法命名空间冲突 | OPEN：030517 拒绝；source method com.tebie.cmapp.ajxvr.module.n.a(Lcom/tebie/cmapp/ajxvr/f;Ljava/lang/String;Ljava/lang/String;[BZ)V: static owner a.n.a(Ljava/lang/String;Ljava/la |
| [ ] | CM-117 | `webapp/WEB-INF/lib/wmsvr.jar/com/tebie/cmapp/ajxvr/module/v.java` | J2B-017：static owner 的类型/方法命名空间冲突 | OPEN：030517 拒绝；source method com.tebie.cmapp.ajxvr.module.v.b(Ljava/lang/String;)Ljava/lang/String;: static owner a.f.a(Ljava/lang/String;)Ljava/lang/String; cannot be bound:  |
| [ ] | CM-118 | `webapp/WEB-INF/lib/wmsvr.jar/com/tebie/commons/collections/b.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for com/tebie/commons/collections/b |
| [ ] | CM-119 | `webapp/WEB-INF/lib/wmsvr.jar/com/tebie/commons/collections/c.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for com/tebie/commons/collections/c |
| [ ] | CM-120 | `webapp/WEB-INF/lib/wmsvr.jar/com/tebie/commons/collections/d.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for com/tebie/commons/collections/d |
| [ ] | CM-121 | `webapp/WEB-INF/lib/wmsvr.jar/com/tebie/commons/collections/e.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for com/tebie/commons/collections/e |
| [ ] | CM-122 | `webapp/WEB-INF/lib/wmsvr.jar/com/tebie/commons/collections/i.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for com/tebie/commons/collections/i |
| [ ] | CM-123 | `webapp/WEB-INF/lib/wmsvr.jar/com/tebie/commons/collections/n.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for com/tebie/commons/collections/n |
| [ ] | CM-124 | `webapp/WEB-INF/lib/wmsvr.jar/com/tebie/commons/collections/o.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for com/tebie/commons/collections/o |
| [ ] | CM-125 | `webapp/WEB-INF/lib/wmsvr.jar/com/tebie/commons/collections/p.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for com/tebie/commons/collections/p |
| [ ] | CM-126 | `webapp/WEB-INF/lib/wmsvr.jar/com/tebie/commons/collections/t.java` | J2B-015：EnclosingMethod/泛型词法作用域证据缺失 | OPEN：030517 拒绝；unproven original enclosing method type scope for com/tebie/commons/collections/t |
| [ ] | CM-127 | `webapp/WEB-INF/lib/wmsvr.jar/com/tebie/commons/search/Comparison.java` | J2B-010/enclosing：枚举常量隐藏名或改名常量体；详见关联 JSON | OPEN：030517 拒绝；enum regeneration: original constant initialization is unproved: constant-specific enclosing identity |
| [x] | CM-128 | `webinst.war/WEB-INF/lib/webinst.jar/cF.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 571cc6f7f504782c |
| [x] | CM-129 | `webinst.war/WEB-INF/lib/webinst.jar/cI.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 fc07c009a61e11ae |
| [x] | CM-130 | `webinst.war/WEB-INF/lib/webinst.jar/cK.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 4fb4d0e114a1e759 |
| [x] | CM-131 | `webinst.war/WEB-INF/lib/webinst.jar/cR.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 932fe57eefdacc1d |
| [x] | CM-132 | `webinst.war/WEB-INF/lib/webinst.jar/cX.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 d88301b0ab4a6804 |
| [x] | CM-133 | `webinst.war/WEB-INF/lib/webinst.jar/co.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 35a85453f550bf58 |
| [x] | CM-134 | `webinst.war/WEB-INF/lib/webinst.jar/com/tebie/cmapp/webinst/InstApp$State.java` | J2B-018：枚举 backing 声明/常量体所有权 | 030517 全量 java 通过；SHA256 978824b845073e34 |
| [x] | CM-135 | `webinst.war/WEB-INF/lib/webinst.jar/cq.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 37142d8ca9d1e797 |
| [x] | CM-136 | `webinst.war/WEB-INF/lib/webinst.jar/cr.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 2ba8609d875fd006 |
| [x] | CM-137 | `webinst.war/WEB-INF/lib/webinst.jar/ct.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 b16ac5b78557d602 |
| [x] | CM-138 | `webinst.war/WEB-INF/lib/webinst.jar/dc.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 75815f0763ac04d1 |
| [x] | CM-139 | `webinst.war/WEB-INF/lib/webinst.jar/dh.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 caabeb10ca9b5973 |
| [x] | CM-140 | `webinst.war/WEB-INF/lib/webinst.jar/dk.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 b1a13c6ded78bb24 |
| [x] | CM-141 | `webinst.war/WEB-INF/lib/webinst.jar/ds.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 2db42f4b9c3abe92 |
| [x] | CM-142 | `webinst.war/WEB-INF/lib/webinst.jar/ek.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 996295b16db2af2b |
| [ ] | CM-143 | `webinst.war/WEB-INF/lib/webinst.jar/ez.java` | J2B-010/name-ordinal：枚举常量隐藏名或改名常量体；详见关联 JSON | REOPEN：FrontendRaw 通过但 backing 赋值泄漏；定向修复待新全量 |
| [ ] | CM-144 | `weixin.war/WEB-INF/lib/groovy-all-2.0.1.jar/groovy/inspect/swingui/CompilePhaseAdapter.java` | J2B-019：枚举常量的动态构造来源 | OPEN：030517 拒绝；enum regeneration: original constant initialization is unproved: constant constructor origin |
| [x] | CM-145 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/annotations/CacheConcurrencyStrategy.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 19652300c0822b4f |
| [x] | CM-146 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/annotations/CacheModeType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 e2fa6bd4b872606f |
| [x] | CM-147 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/annotations/CascadeType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 d4517afdb36ae2f9 |
| [x] | CM-148 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/annotations/FetchMode.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 3f1b0668a789d68e |
| [x] | CM-149 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/annotations/FlushModeType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 a82e62bb5092fdaa |
| [x] | CM-150 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/annotations/GenerationTime.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 a5151ffbeb5f32ad |
| [x] | CM-151 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/annotations/LazyCollectionOption.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 44bdbc50acc3bb5e |
| [x] | CM-152 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/annotations/LazyToOneOption.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 0744814053dbf3aa |
| [x] | CM-153 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/annotations/NotFoundAction.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 54b541a703ffe5bd |
| [x] | CM-154 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/annotations/OnDeleteAction.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 54d32cee162024d0 |
| [x] | CM-155 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/annotations/OptimisticLockType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 70feecf4c87a1909 |
| [x] | CM-156 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/annotations/PolymorphismType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 91df6392f8319cde |
| [x] | CM-157 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/annotations/ResultCheckStyle.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 6e084b2001be3e25 |
| [x] | CM-158 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/annotations/SortType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 571cc305fc23828a |
| [x] | CM-159 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/annotations/SourceType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 82ebd692acb559a5 |
| [x] | CM-160 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/cfg/AccessType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 9494afd125291116 |
| [x] | CM-161 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/cfg/AnnotatedClassType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 033bf70ba526091a |
| [x] | CM-162 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/cfg/MetadataSourceType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 ec7bf1fc01cead48 |
| [x] | CM-163 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/cfg/annotations/Nullability.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 ffe60c5087d4bd32 |
| [x] | CM-164 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/cfg/annotations/reflection/JPAOverridenAnnotationReader$PropertyType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 aadb7ba7627395de |
| [x] | CM-165 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/cfg/beanvalidation/BeanValidationActivator$ValidationMode.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 fae7bb5315375ddb |
| [x] | CM-166 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/cfg/beanvalidation/GroupsPerOperation$Operation.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 27527d7c5716d340 |
| [x] | CM-167 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/id/enhanced/OptimizerFactory$StandardOptimizerDescriptor.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 50cd51b63270f6c5 |
| [x] | CM-168 | `weixin.war/WEB-INF/lib/hibernate-core-3.6.10.Final.jar/org/hibernate/mapping/MetadataSource.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 a2227ae0f21c74f9 |
| [x] | CM-169 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/AccessType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 21b215f81dbca091 |
| [x] | CM-170 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/CacheRetrieveMode.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 6a4feef9cf21a537 |
| [x] | CM-171 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/CacheStoreMode.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 1702b51b189886e2 |
| [x] | CM-172 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/CascadeType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 67f4c2b8547c1276 |
| [x] | CM-173 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/DiscriminatorType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 d48bb5ca28bd3f14 |
| [x] | CM-174 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/EnumType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 1da0e7b39d477f79 |
| [x] | CM-175 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/FetchType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 1f530ffd1fccfc1f |
| [x] | CM-176 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/FlushModeType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 e3630102b88aec76 |
| [x] | CM-177 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/GenerationType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 0aeddc71dfe117c7 |
| [x] | CM-178 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/InheritanceType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 4ed0aa945acb73a6 |
| [x] | CM-179 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/LockModeType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 265beff2f4afc7cb |
| [x] | CM-180 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/PersistenceContextType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 76264a14ed4821fa |
| [x] | CM-181 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/PessimisticLockScope.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 62edd8dae74b6cc4 |
| [x] | CM-182 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/SharedCacheMode.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 5f4fb82858e55b2a |
| [x] | CM-183 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/TemporalType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 20d961a6e5d4ebbd |
| [x] | CM-184 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/ValidationMode.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 09b4f949cc277606 |
| [x] | CM-185 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/criteria/CriteriaBuilder$Trimspec.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 62a9526c61b5be28 |
| [x] | CM-186 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/criteria/JoinType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 5f1c93f437cfc9cb |
| [x] | CM-187 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/criteria/Predicate$BooleanOperator.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 fda983b88e3c7acc |
| [x] | CM-188 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/metamodel/Attribute$PersistentAttributeType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 2ea24c285301cece |
| [x] | CM-189 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/metamodel/Bindable$BindableType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 e4093a5693fcdc4e |
| [x] | CM-190 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/metamodel/PluralAttribute$CollectionType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 82f29b4322a7cca9 |
| [x] | CM-191 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/metamodel/Type$PersistenceType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 8c82873522e58a41 |
| [x] | CM-192 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/spi/LoadState.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 abb53526397c5d8a |
| [x] | CM-193 | `weixin.war/WEB-INF/lib/hibernate-jpa-2.0-api-1.0.1.Final.jar/javax/persistence/spi/PersistenceUnitTransactionType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 730dbb87ca832756 |
| [x] | CM-194 | `weixin.war/WEB-INF/lib/hibernate-validator-4.3.0.Final.jar/org/hibernate/validator/internal/xml/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 a5515fb404faacee |
| [x] | CM-195 | `weixin.war/WEB-INF/lib/jackson-core-asl-1.7.1.jar/org/codehaus/jackson/annotate/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 b814fad63f50d5f8 |
| [x] | CM-196 | `weixin.war/WEB-INF/lib/jackson-core-asl-1.7.1.jar/org/codehaus/jackson/impl/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 beebc8cc4ef48be9 |
| [x] | CM-197 | `weixin.war/WEB-INF/lib/jackson-core-asl-1.7.1.jar/org/codehaus/jackson/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 0ea294385a137363 |
| [x] | CM-198 | `weixin.war/WEB-INF/lib/jackson-core-asl-1.7.1.jar/org/codehaus/jackson/sym/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 94c239fb5e5bbb68 |
| [x] | CM-199 | `weixin.war/WEB-INF/lib/jackson-core-asl-1.7.1.jar/org/codehaus/jackson/type/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 53ae65ccf27867fb |
| [x] | CM-200 | `weixin.war/WEB-INF/lib/jackson-core-asl-1.7.1.jar/org/codehaus/jackson/util/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 ab47091501879653 |
| [x] | CM-201 | `weixin.war/WEB-INF/lib/jackson-mapper-asl-1.7.1.jar/org/codehaus/jackson/map/annotate/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 67904b637fcf99fb |
| [x] | CM-202 | `weixin.war/WEB-INF/lib/jackson-mapper-asl-1.7.1.jar/org/codehaus/jackson/map/deser/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 a7e6717498f950e2 |
| [x] | CM-203 | `weixin.war/WEB-INF/lib/jackson-mapper-asl-1.7.1.jar/org/codehaus/jackson/map/ext/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 77dd0dc2bfac46f8 |
| [x] | CM-204 | `weixin.war/WEB-INF/lib/jackson-mapper-asl-1.7.1.jar/org/codehaus/jackson/map/introspect/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 af4999bf4684855e |
| [x] | CM-205 | `weixin.war/WEB-INF/lib/jackson-mapper-asl-1.7.1.jar/org/codehaus/jackson/map/jsontype/impl/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 41229c2970f20747 |
| [x] | CM-206 | `weixin.war/WEB-INF/lib/jackson-mapper-asl-1.7.1.jar/org/codehaus/jackson/map/jsontype/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 1a9228fba1434a72 |
| [x] | CM-207 | `weixin.war/WEB-INF/lib/jackson-mapper-asl-1.7.1.jar/org/codehaus/jackson/map/module/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 8189cf04995edf30 |
| [x] | CM-208 | `weixin.war/WEB-INF/lib/jackson-mapper-asl-1.7.1.jar/org/codehaus/jackson/map/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 d8d049ccd4ccbc80 |
| [x] | CM-209 | `weixin.war/WEB-INF/lib/jackson-mapper-asl-1.7.1.jar/org/codehaus/jackson/map/ser/impl/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 0d281b3f62a611a1 |
| [x] | CM-210 | `weixin.war/WEB-INF/lib/jackson-mapper-asl-1.7.1.jar/org/codehaus/jackson/map/ser/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 77dd433cd9146e3d |
| [x] | CM-211 | `weixin.war/WEB-INF/lib/jackson-mapper-asl-1.7.1.jar/org/codehaus/jackson/map/type/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 3b86a4f22d17fd0a |
| [x] | CM-212 | `weixin.war/WEB-INF/lib/jackson-mapper-asl-1.7.1.jar/org/codehaus/jackson/map/util/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 318d712432be48db |
| [x] | CM-213 | `weixin.war/WEB-INF/lib/jackson-mapper-asl-1.7.1.jar/org/codehaus/jackson/node/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 edd5c59b19acfa5e |
| [x] | CM-214 | `weixin.war/WEB-INF/lib/jackson-mapper-asl-1.7.1.jar/org/codehaus/jackson/schema/package-info.java` | J2B-001：package-info 注解/包声明丢失 | 030517 全量 descriptor 通过；SHA256 a791567b274ba371 |
| [x] | CM-215 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/AccessType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 21b215f81dbca091 |
| [x] | CM-216 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/CacheRetrieveMode.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 6a4feef9cf21a537 |
| [x] | CM-217 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/CacheStoreMode.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 1702b51b189886e2 |
| [x] | CM-218 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/CascadeType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 67f4c2b8547c1276 |
| [x] | CM-219 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/DiscriminatorType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 d48bb5ca28bd3f14 |
| [x] | CM-220 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/EnumType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 1da0e7b39d477f79 |
| [x] | CM-221 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/FetchType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 1f530ffd1fccfc1f |
| [x] | CM-222 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/FlushModeType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 e3630102b88aec76 |
| [x] | CM-223 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/GenerationType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 0aeddc71dfe117c7 |
| [x] | CM-224 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/InheritanceType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 4ed0aa945acb73a6 |
| [x] | CM-225 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/LockModeType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 265beff2f4afc7cb |
| [x] | CM-226 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/PersistenceContextType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 76264a14ed4821fa |
| [x] | CM-227 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/PessimisticLockScope.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 62edd8dae74b6cc4 |
| [x] | CM-228 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/SharedCacheMode.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 5f4fb82858e55b2a |
| [x] | CM-229 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/TemporalType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 20d961a6e5d4ebbd |
| [x] | CM-230 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/ValidationMode.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 09b4f949cc277606 |
| [x] | CM-231 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/criteria/CriteriaBuilder$Trimspec.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 62a9526c61b5be28 |
| [x] | CM-232 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/criteria/JoinType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 5f1c93f437cfc9cb |
| [x] | CM-233 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/criteria/Predicate$BooleanOperator.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 fda983b88e3c7acc |
| [x] | CM-234 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/metamodel/Attribute$PersistentAttributeType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 2ea24c285301cece |
| [x] | CM-235 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/metamodel/Bindable$BindableType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 e4093a5693fcdc4e |
| [x] | CM-236 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/metamodel/PluralAttribute$CollectionType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 82f29b4322a7cca9 |
| [x] | CM-237 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/metamodel/Type$PersistenceType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 8c82873522e58a41 |
| [x] | CM-238 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/spi/LoadState.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 abb53526397c5d8a |
| [x] | CM-239 | `weixin.war/WEB-INF/lib/jpa-api-2.0-cr-1.jar/javax/persistence/spi/PersistenceUnitTransactionType.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 730dbb87ca832756 |
| [ ] | CM-240 | `weixin.war/WEB-INF/lib/logback-classic-1.1.1.jar/ch/qos/logback/classic/gaffer/NestingType.java` | J2B-019：枚举常量的动态构造来源 | OPEN：030517 拒绝；enum regeneration: original constant initialization is unproved: constant constructor origin |
| [x] | CM-241 | `weixin.war/WEB-INF/lib/validation-api-1.0.0.GA.jar/javax/validation/constraints/Pattern$Flag.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 ownership_suppressed 通过；SHA256 61ab6eb000f466a0 |
| [x] | CM-242 | `weixin.war/WEB-INF/lib/validation-api-1.0.0.GA.jar/javax/validation/metadata/Scope.java` | J2B-011：旧编译器 final 枚举工厂 flags | 030517 全量 java 通过；SHA256 be643a827e818e9d |
| [ ] | CM-243 | `wmsvr.jar/a/dK.java` | J2B-017：static owner 的类型/方法命名空间冲突 | OPEN：030517 拒绝；source method a.dK.b(Ljava/io/File;Ljava/io/File;)Lcn/mailtech/cm/preview/h;: static owner cn.mailtech.utils.g.a(Ljava/lang/String;Ljava/lang/String;)Ljava/lang |
| [ ] | CM-244 | `wmsvr.jar/a/dv.java` | J2B-017：static owner 的类型/方法命名空间冲突 | OPEN：030517 拒绝；source method a.dv.a(Ljava/lang/String;Ljava/lang/String;)Z: static owner cn.mailtech.utils.g.a(Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String; cannot b |
| [x] | CM-245 | `wmsvr.jar/a/l.java` | J2B-002：同包类型被包首段遮蔽 | 030517 全量 java 通过；SHA256 8b1bfd3c02d2a05e |
| [ ] | CM-246 | `wmsvr.jar/com/tebie/cmapp/ajxvr/lib/B.java` | J2B-017：static owner 的类型/方法命名空间冲突 | OPEN：030517 拒绝；source method com.tebie.cmapp.ajxvr.lib.B.a(Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;: static owner a.g.a(Ljava/lang/String;Ljava/lang/String;)Lja |
| [ ] | CM-247 | `wmsvr.jar/com/tebie/cmapp/ajxvr/lib/i.java` | J2B-017：static owner 的类型/方法命名空间冲突 | OPEN：030517 拒绝；source method com.tebie.cmapp.ajxvr.lib.i.a(Lcom/tebie/cmx/mail/k;Ljava/lang/String;Z[Ljava/lang/String;I)La/cl;: static owner a.e.a(Lcom/tebie/cmx/Y;Ljava/lang |
| [ ] | CM-248 | `wmsvr.jar/com/tebie/cmapp/ajxvr/module/D.java` | J2B-017：static owner 的类型/方法命名空间冲突 | OPEN：030517 拒绝；source method com.tebie.cmapp.ajxvr.module.D.b(Ljava/lang/String;)Ljava/lang/String;: static owner a.g.a(Ljava/lang/String;)Ljava/lang/String; cannot be bound:  |
| [x] | CM-249 | `wmsvr.jar/com/tebie/commons/search/Comparison.java` | J2B-010/enclosing：枚举常量隐藏名或改名常量体；详见关联 JSON | 030517 全量 java 通过；SHA256 a7ea0fe75ca44c39 |

条目总数：249；本表已勾选 192，未勾选 57。

## 不变的验收门槛

- 真实全量 exit=0；materialization_errors=0；syntax_failures=0，所有生成 Java 均过 FrontendRaw。
- 不调阈值，不跳文件，不制造占位方法，不放宽协议。已证不可表达也必须有可审计、可复验的受控抑制证明，目前未新增。
- 1634 stub 仍计失败，不能因这 249 行解析变好而忽略。
- 不自动进入 coremail 后续阶段、ekp 或性能阶段；不 push、不建 PR。
