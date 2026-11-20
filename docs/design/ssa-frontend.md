# SSA 前端：统一管线与各语言实现

本文说明 yaklang 的 SSA 前端架构、语言注册、各语言语法来源、SSA 降级边界和测试入口。
重点覆盖新增的 Lua、Rust、Kotlin、Scala、C++ 五个前端，同时记录已有语言的接入方式。
前端统一使用仓库内的 ANTLR 4.13.2；TypeScript 前端是历史例外，见 §4.3。

## 1. 设计口径

- 一种语言只保留一套语法。
- 新语言使用分离式 `<Lang>Lexer.g4` / `<Lang>Parser.g4`；生成代码放在语言包的 `parser/`。
- 需要缩进或 NEWLINE 语义的语言，把逻辑放进手写基类，不把语法改成递归下降。
- Lua 复用已有 `common/yak/antlr4Lua`，不复制、不移动语法文件。
- Scala 只使用 Scala 3 语法，通过 `migration30()` 兼容 Scala 2 的常用写法。
- 首版不实现完整的 OOP 分派、模式匹配、泛型实例化、闭包捕获和协程语义。
- SSA 前端负责把语法树降到 `ssa.Instruction`；不做运行时执行，不做跨语言类型系统合并。

## 2. 从源文件到 SSA 的管线

```text
源码文件
  -> ssaapi.LanguageBuilderCreater[language]
  -> ssa.Builder.FilterFile / FilterParseAST
  -> Builder.ParseAST
  -> ANTLR lexer + parser（或语言自带 parser）
  -> Builder.BuildFromAST
  -> ssa.FunctionBuilder
  -> SSA IR
  -> 编译单元、持久化、SyntaxFlow
```

关键接口：

| 位置 | 作用 |
| --- | --- |
| `common/yak/ssaapi/ssaconfig/language.go` | 语言常量、扩展名、别名和 `ValidateLanguage`。 |
| `common/yak/ssaapi/ssa_language_builder_creater.go` | 语言到 `ssa.CreateBuilder` 的注册表。 |
| `common/yak/ssa/extra_file_analyzer.go` | `Builder`、`PreHandlerAnalyzer`、`FrontAST`、`AntlrCache` 接口。 |
| `common/yak/ssaapi/ssa_compile_project.go` | 项目扫描、编译单元划分和项目级编译。 |
| `common/yak/ssaapi/ssa_compile_fs.go` | 文件系统、预处理器和按单元执行。 |
| `common/yak/ssaapi/ssa_compile_utils.go` | 语言 builder 查找、并发和内存策略。 |
| `common/yak/ssa/package_loader.go` | 单文件解析、AST 构建和包级状态。 |

`ssa.Builder` 的关键方法：

- `FilterFile`：文件是否属于该语言。
- `FilterParseAST` / `FilterPreHandlerFile`：是否进入 AST 解析或预处理。
- `WrapWithPreprocessedFS`：需要宏、include、归档展开的语言在这里换文件系统。
- `GetAntlrCache`：共享 lexer/parser ATN 和 DFA 缓存；返回 `nil` 表示不复用。
- `ParseAST`：源码到 `FrontAST`。
- `BuildFromAST`：`FrontAST` 到 SSA IR。
- `PartitionCompileUnits` / `CompileUnitDependencies`：项目级编译单元划分和依赖。

解析默认走 `antlr4util.ParseASTWithSLLFirst`：先用 SLL 预测，失败后用同一 token 流重试 LL。
局部歧义不会直接放弃整个文件，因此 `GetAntlrCache` 的复用和回退策略要一起看。

## 3. 语言注册表

| 语言 | `ssaconfig.Language` | 扩展名 | Builder | 说明 |
| --- | --- | --- | --- | --- |
| Yak | `Yak` | `.yak`, `.yaklang` | `yak2ssa` | Yak 自身脚本语言。 |
| JavaScript | `JS` | `.js` | `ts2ssa` | 与 TypeScript 共用 builder。 |
| TypeScript | `TS` | `.ts`, `.tsx`, `.js` | `ts2ssa` | 使用 TypeScript 前端，不是 ANTLR。 |
| PHP | `PHP` | `.php` | `php2ssa` | ANTLR PHP 语法。 |
| Java | `JAVA` | `.java` | `java2ssa` | 另有 JSP、Freemarker、SpEL 子前端。 |
| Go | `GO` | `.go`, `.mod` | `go2ssa` | 支持 Go module 和包级编译。 |
| C | `C` | `.c` | `c2ssa` | 带 C 预处理文件系统。 |
| C++ | `CPP` | `.cpp`, `.cc`, `.cxx` | `cpp2ssa` | 独立语法包，复用 `cfamily/preprocess`。 |
| Python | `PYTHON` | `.py` | `python2ssa` | ANTLR Python 语法。 |
| C# | `CSHARP` | `.cs` | `csharp2ssa` | 另有 ASP/ASPX 子前端。 |
| Lua | `LUA` | `.lua` | `lua2ssa` | 复用 `antlr4Lua`。 |
| Rust | `RUST` | `.rs` | `rust2ssa` | 新增。 |
| Kotlin | `KOTLIN` | `.kt`, `.kts` | `kotlin2ssa` | 新增；`.kts` 使用脚本入口。 |
| Scala | `SCALA` | `.scala`, `.sc` | `scala2ssa` | 新增；Scala 3 语法 + Scala 2 兼容。 |

`ssaapi.LanguageBuilderCreater` 是唯一运行时注册表。`irify_exclude` 构建只保留 Yak；
其他语言和 SyntaxFlow 由构建标签排除。新增语言只修改完整注册表；`irify_exclude`
版本保持仅 Yak，新增语言包不能经公共文件被无标签构建强制导入。

## 4. 各语言前端

### 4.1 Yak

- 语法：`common/yak/antlr4yak/YaklangLexer.g4`、`YaklangParser.g4`。
- 前端：`common/yak/yak2ssa`。
- 特点：Yak 是仓库自身语言，`yak2ssa` 同时服务脚本执行、插件和静态分析；语法改动的影响面最大。

### 4.2 PHP

- 语法：`common/yak/php/PHPLexer.g4`、`PHPParser.g4`；生成到 `common/yak/php/parser`。
- 前端：`common/yak/php/php2ssa`。
- 生成脚本：`common/yak/php/gen.sh`。
- 覆盖 PHP 7/8 的常见声明、表达式、闭包和命名空间；模板文件由语言策略单独处理。

### 4.3 JavaScript 与 TypeScript

- 注册表把 `JS` 和 `TS` 都映射到 `ts2ssa`。
- `common/yak/typescript/frontend` 是独立的 TypeScript 前端实现，不是 ANTLR 语法。
- `common/yak/JS2ssa` 保留 ANTLR JavaScript 语法的历史实现，但当前注册表不使用它。
- `ts2ssa` 负责 `export`、`import`、闭包、类、对象和常见 Web API 的 SSA 降级。

这是范围说明，不是新语言接入模板。新增语言仍按 §5 使用 ANTLR。

### 4.4 Java

- 语法：`common/yak/java/JavaLexer.g4`、`JavaParser.g4`。
- 子语法：`jsp/`、`freemarker/`、`spel/`。
- 前端：`common/yak/java/java2ssa`。
- 归档：`OpenArchiveCodeSource` 让 Java 前端打开 jar；这是 `ArchiveCodeSourceOpener` 的唯一实现。

### 4.5 Go

- 语法：`common/yak/antlr4go/GoLexer.g4`、`GoParser.g4`。
- 前端：`common/yak/go2ssa`。
- 项目编译按 Go 包和 module 划分编译单元；`go.mod` 进入预处理。
- 支持方法、接口、结构体、`defer`、goroutine 和 channel 的常见写法，复杂泛型仍是降级边界。

### 4.6 C

- 语法：`common/yak/antlr4c/CLexer.g4`、`CParser.g4`。
- 前端：`common/yak/c2ssa`。
- `WrapWithPreprocessedFS` 接入 C 预处理文件系统，处理 include 和宏展开。
- 头文件和跨编译单元声明依赖 C 预处理结果。

### 4.7 Python

- 语法：`common/yak/python/PythonLexer.g4`、`PythonParser.g4`。
- 前端：`common/yak/python/python2ssa`。
- 支持缩进语法、函数、类、装饰器、生成式和常见标准库调用。

### 4.8 C#

- 语法：`common/yak/csharp/CSharpLexer.g4`、`CSharpParser.g4`。
- 子语法：`common/yak/csharp/asp`。
- 前端：`common/yak/csharp/csharp2ssa`。
- 使用 deferred file build；项目级状态包括全局 using、构造器和声明类型。

### 4.9 Lua（新增）

- 语法：复用 `common/yak/antlr4Lua/LuaLexer.g4`、`LuaParser.g4`。
- 前端：`common/yak/lua/lua2ssa`，入口 `Frontend` -> `parser.Chunk()`。
- `GetAntlrCache()` 返回 `nil`，因为复用已有生成代码，不额外维护 ATN 缓存。
- 覆盖 Lua 5.1、5.2、5.3、5.4 的常见语法：`goto`/标签、整除、按位运算、`<const>`/`<close>`、长字符串和长注释。
- SSA 降级边界：`goto` 只解析不生成跳转；元表、协程、可变参数展开和泛型 `for` 迭代器不建模；`:` 方法调用传递 `self`，但不做动态分派。

### 4.10 Rust（新增）

- 语法来源：`antlr/grammars-v4` 的 `rust/`，提交 `c306207ac870`。
- 文件：`common/yak/rust/RustLexer.g4`、`RustParser.g4`；生成到 `common/yak/rust/parser`。
- 生成脚本：`common/yak/rust/gen.sh`。
- 许可证：MIT，语法文件头保留原始版权声明。
- 入口：`rust2ssa.Frontend` -> `parser.Crate()`。
- 覆盖 2015/2018/2021 edition 的常见声明、表达式、控制流、运算符和宏调用语法。
- SSA 降级边界：`impl`/`trait` 不建立方法表；`match` 不展开模式匹配；泛型和生命周期不参与推导；闭包捕获、`async`/`await`、宏体、Range 迭代不展开。

### 4.11 Kotlin（新增）

- 语法来源：`antlr/grammars-v4` 的 `kotlin/kotlin-formal/`，提交 `c306207ac870`。
- 文件：`common/yak/kotlin/KotlinLexer.g4`、`KotlinParser.g4`；生成到 `common/yak/kotlin/parser`。
- 生成脚本：`common/yak/kotlin/gen.sh`。
- 词法层直接使用 ANTLR 的 Unicode 属性：`Letter` 为 `[\p{L}\p{Nl}]`，`UnicodeDigit` 为 `[\p{Nd}]`；不依赖单独的 `UnicodeClasses.g4`。
- 入口：
  - `.kt`：`kotlin2ssa.Frontend` -> `parser.KotlinFile()`。
  - `.kts`：`kotlin2ssa.ScriptFrontend` -> `parser.Script()`。
- 覆盖 `package`、`import`、`fun`、`class`、`data class`、`object`、`interface`、`enum class`、属性、默认参数、`when`、集合字面量和常见表达式。
- SSA 降级边界：类/接口只遍历成员，不建立继承表或虚方法分派；`when` 按条件链降级；`is` 不生成类型测试；泛型、委托属性、Lambda 捕获、协程和字符串模板插值不展开。

### 4.12 Scala（新增）

- 语法来源：`antlr/grammars-v4` 的 `scala/scala3/`，提交 `c306207ac870`；本地改名为 `ScalaLexer.g4` / `ScalaParser.g4`。
- 文件：`common/yak/scala/ScalaLexer.g4`、`ScalaParser.g4`；生成到 `common/yak/scala/parser`。
- 生成脚本：`common/yak/scala/gen.sh`。
- 选择依据：同一组 51 个样例上 Scala 3 语法 48 PASS、Scala 2 语法 11 PASS，因此只保留 Scala 3 语法。
- 基类：`Scala3LexerBase.java` 和 `Scala3ParserBase.java` 移植为 `scala_lexer_base.go`、`scala_parser_base.go`；NEWLINE、INDENT、DEDENT 由基类注入。
- 兼容：`migration30()` 默认开启，支持 Scala 2 的 `._` 通配符导入和 `_` 通配符类型；不引入第二套 Scala 2 语法。
- 覆盖 Scala 3 缩进和花括号语法、`package`、`import`、`def`、`val`/`var`、`class`/`object`/`trait`/`enum`/`case class`/`given`。
- SSA 降级边界：继承和虚方法分派、`match`、`given`/`extension`、泛型/隐式推导、Lambda 捕获、字符串插值和 `for` 生成器迭代不展开。

### 4.13 C++（新增）

- 语法：`common/yak/cpp/CPPLexer.g4`、`CPPParser.g4`，生成到 `common/yak/cpp/parser`；生成脚本 `common/yak/cpp/gen.sh`。
- 维护方式：语法在仓库内维护，不再跟随上游版本；本地补丁记录在 `gen.sh` 注释里。MIT 署名保留在 `.g4` 文件头和 `common/yak/cpp/LICENSE`。
- 与 C 分开：不改 `CParser.g4`；C 解析器实测只通过 7% 的真实 C++ 文件，扩展 C 语法等于重写 C 前端。
- 共用：`common/yak/cfamily/preprocess`（宏和 include）与 `common/yak/cfamily/cliteral`（字面量）；共享改动用配置开关保持 C 默认行为。
- 扩展名：`.cpp`、`.cc`、`.cxx` 是翻译单元；`.h` 归 C，`.hpp`、`.hxx`、`.hh` 归 C++。
- 头文件：只通过 `#include` 内联，不做独立翻译单元；项目内头文件全文内联，系统头不展开。纯头文件项目不产生翻译单元。
- 覆盖：命名空间、类与继承、成员函数、构造/析构与初始化列表、模板声明、引用/指针、`new`/`delete`、`using`/`typedef`、lambda、类型转换。
- 标准库：`stdlib.go` 一份声明表，只给类型和成员名，不给行为；`<algorithm>`、`<regex>`、`<chrono>` 等未收录类型按占位蓝图处理。
- SSA 降级边界：不实例化模板、重载只按参数个数选择、不做虚分派和 RTTI、`sizeof`/`alignof` 返回 0、`goto` 不建 CFG 边、C++20 concepts/requires/modules/coroutines 不支持。
- 测试：`common/yak/cpp/cpp2ssa/test`（`TestAllSyntaxForCpp_G4`、`TestCpp*`、`CPP_CORPUS` 语料模式）。

## 5. 新增语言的接入清单

按以下顺序接入，任何一步遗漏都会让语言在某个入口不可见：

1. `common/yak/ssaapi/ssaconfig/language.go`
   - 增加语言常量。
   - 增加 `GetFileExt` 扩展名。
   - 增加 `ValidateLanguage` 别名。
   - 加入 `GetAllSupportedLanguages`。
2. `common/yak/ssaapi/ssa_language_builder_creater.go`
   - 注册 `CreateBuilder`。
   - 确认 `irify_exclude` 版本仍只保留 Yak。
3. 语言目录
   - `<Lang>Lexer.g4`、`<Lang>Parser.g4`、`gen.sh`、`parser/`。
   - `<lang>2ssa/`：`builder.go`、`frontend.go`、语句/表达式访问器。
4. Builder 配置
   - `FilterFile`、`FilterParseAST`、`FilterPreHandlerFile`。
   - `WrapWithPreprocessedFS`。
   - `GetAntlrCache`。
   - 需要项目语义时实现 `PreHandlerProject` / `PartitionCompileUnits`。
5. 语言周边
   - `common/syntaxflow/sfpattern/language_files.go` 的扩展名和别名。
   - `common/yak/ssaapi/ssaconfig/scan_policies.yaml` 的语言策略。
   - `common/coreplugin/base-yak-plugin/SSA 项目探测.yak` 的扩展名、标记文件和清单文件。
   - `scripts/ssa-test.sh` 的本地测试序列。
6. 测试
   - 语法夹具：`common/yak/<lang>/<lang>2ssa/test/code/`，配 `README.md` 记录来源和许可证。
   - AST 解析：`syntax_<lang>_test.go` 只保留 `TestAllSyntaxFor<Lang>_G4`。
   - SSA 降级：`common/yak/<lang>/<lang>2ssa/test/` 的 `cfg_test.go`、`func_test.go`、`expression_test.go`、`oop_test.go`、`import_test.go`、`multi_file_test.go`，使用 `ssatest` 助手断言 Phi、调用操作数和符号。
7. CI
   - `.github/workflows/syntax-g4-tests.yml` matrix 加入语言测试包。
   - 普通 SSA 测试交给 `scripts/ci/configs/ssa.json` 和 Essential Tests，不放进语法工作流。

## 6. 测试与 CI 分层

| 层级 | 位置 | 运行方式 |
| --- | --- | --- |
| 语法/AST 解析 | `common/yak/<lang>/<lang>2ssa/test/syntax_<lang>_test.go` | `syntax-g4-tests.yml`，`-run '^TestAllSyntax.*_G4$'` |
| 语法夹具 | `common/yak/<lang>/<lang>2ssa/test/code/` | 由 `TestAllSyntax...` 嵌入并逐个解析 |
| SSA 降级与 CFG | `common/yak/<lang>/<lang>2ssa/test/` | `scripts/ssa-test.sh` 跑语言测试包；Phi 用 `checkPhi`，值用 `CheckPrintf` |
| 跨语言项目编译 | `common/yak/ssaapi/test/ssatest/frontend_languages_test.go` | Essential Tests 的 SSA 组 |
| 本地全量 | `scripts/ssa-test.sh` | 无参数时按语言顺序跑完整 SSA 套件 |

`syntax_<lang>_test.go` 的职责只有一个：解析夹具并检查单文件预算。
默认预算 10 秒，可用 `YAK_<LANG>_FIXTURE_PARSE_BUDGET_SEC` 覆盖。
失败信息必须包含文件名、耗时和解析错误。
Kotlin 的 `.kts` 使用 `ScriptFrontend`，`.kt` 使用 `Frontend`。

新增四语言的语法驱动：

```text
common/yak/lua/lua2ssa/test/syntax_lua_test.go       TestAllSyntaxForLua_G4
common/yak/rust/rust2ssa/test/syntax_rust_test.go     TestAllSyntaxForRust_G4
common/yak/kotlin/kotlin2ssa/test/syntax_kotlin_test.go TestAllSyntaxForKotlin_G4
common/yak/scala/scala2ssa/test/syntax_scala_test.go   TestAllSyntaxForScala_G4
```

新增四语言的 SSA 降级测试在各自语言的 `test/` 目录：

```text
common/yak/lua/lua2ssa/test/       cfg/func/expression/oop/import/multi_file
common/yak/rust/rust2ssa/test/     cfg/func/expression/oop/import/multi_file
common/yak/kotlin/kotlin2ssa/test/ cfg/func/expression/oop/import/multi_file
common/yak/scala/scala2ssa/test/   cfg/func/expression/oop/import/multi_file
```

`TestFrontendLuaRustKotlinScalaCompile` 只检查项目编译和符号存在性，不重复语法解析预算；这样语法失败和 SSA 失败能分开定位。

### 6.1 真实项目语料

真实项目源码不入库，语料结果只放 `/mnt/data/scan-outputs/feature-ssa-lua-rust-kotlin-scala/realworld/`。
本轮测量使用临时本地脚本，脚本不进入仓库；仓库只保留可复现的夹具和 CI 驱动。
测量固定单文件预算 10 秒，结果如下（历史记录，供后续对比）：

| 语言 | 项目 | 文件 | 通过 | 失败 | 超时 | p50 | p95 | 总耗时 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Lua | plenary.nvim | 113 | 112 | 1 | 0 | 3.49ms | 40.2ms | 1.18s |
| Lua | kong | 1310 | 1306 | 4 | 0 | 0.85ms | 34.1ms | 11.3s |
| Rust | serde | 208 | 206 | 2 | 0 | 4.88ms | 1.55s | 1m02s |
| Rust | ripgrep | 102 | 94 | 8 | 0 | 244ms | 2.66s | 1m21s |
| Kotlin | okhttp | 620 | 620 | 0 | 0 | 66.9ms | 1.09s | 2m38s |
| Kotlin | kotlinx.coroutines | 1082 | 1082 | 0 | 0 | 42.0ms | 534ms | 2m32s |
| Scala | scalatra | 235 | 226 | 8 | 1 | 19.1ms | 480ms | 44.5s |
| Scala | zio | 976 | 763 | 210 | 3 | 22.5ms | 911ms | 5m27s |

Lua 的 `spec/fixtures/invalid-module.lua` 是故意非法样本，统计时按文件名排除，不修改夹具。
Kotlin 两个语料全部通过，包含 `.kts` Gradle 脚本。
Scala 剩余失败集中在 XML 字面量、宏和缩进边界；Rust 剩余失败集中在 `let` 链、宏 token 树和宏调用；Lua 剩余失败包含长注释和 `goto` 边界。
解析预算没有放宽。

### 6.2 性能口径

总耗时包含 SLL 优先解析失败后的 LL 回退。Lua 和 Kotlin 在覆盖率提升后进入更完整的解析路径，
总耗时不能直接和修复前的失败短路路径比较。
Scala 的 `simpleExpr` 改为 primary + suffix 循环后，`ServiceReloader.scala` 从约 23 秒降到约 3.3 秒；
zio 总耗时从 6m29s 降到 5m27s。Scalatra 仍有一个缩进边界样本超过 10 秒。

## 7. 已知边界与后续工作

- Scala：XML 字面量、宏展开、缩进边界；`forms/package.scala` 等大文件仍可能超过预算。
- Rust：macro token tree、let chain、`union` 上下文关键字。
- Lua：`--["...` 形式的行注释、长注释和 `goto` 边界。
- Kotlin：语法覆盖已清空语料失败；后续如需 Gradle DSL 语义，再单独设计脚本 SSA。
- 现有语言：TypeScript 仍是独立 parser；Java 的 jar 展开和 Go/C 的项目级依赖分别由各自 builder 处理，不应复制到新语言。

## 8. 本地验证

```sh
export GOCACHE=/mnt/data/cache/go-build
export GOMODCACHE=/mnt/data/cache/go-mod
export GOTMPDIR=/mnt/data/cache/go-tmp
export TMPDIR="$GOTMPDIR"
export YAKIT_HOME=/mnt/data/scan-outputs/feature-ssa-lua-rust-kotlin-scala/<run-id>/yakit-home
mkdir -p "$GOTMPDIR" "$YAKIT_HOME"

scripts/ssa-test.sh -count=1 -timeout 5m \
  ./common/yak/lua/... ./common/yak/rust/... \
  ./common/yak/kotlin/... ./common/yak/scala/...

scripts/ssa-test.sh -count=1 -timeout 2m \
  -run TestFrontendLuaRustKotlinScalaCompile \
  ./common/yak/ssaapi/test/ssatest
```

构建、日志、性能和语料结果全部位于 `/mnt/data/scan-outputs/feature-ssa-lua-rust-kotlin-scala/`，不写入 worktree 的 `build/`、`.db/` 或 home。