# SSA 前端扩展：Lua、Rust、Kotlin、Scala

本文说明四个语言前端的语法来源、版本覆盖、SSA 降级边界和实测结果。
所有解析均使用 ANTLR 4.13.2，不使用 tree-sitter、手写词法分析器或递归下降解析器。

## 1. 约束与设计口径

- 每种语言只保留一套语法。
- Scala 使用 Scala 3 语法，并打开 Scala 2 兼容开关；不引入第二套 Scala 2 语法。
- Rust 与 Kotlin 使用分离的 `Lexer.g4` / `Parser.g4`。
- Scala 使用分离的 `ScalaLexer.g4` / `ScalaParser.g4`，命名与 `common/yak/java`、`common/yak/php` 一致。
- Lua 复用已有 `common/yak/antlr4Lua`，不复制、不移动、不重命名语法文件。
- 生成代码放在语言包内的 `parser/` 目录，生成文件不手工修改。
- 首版不实现 OOP 分派、模式匹配、泛型实例化、闭包捕获和协程。

## 2. 文件布局

| 语言 | 语法文件 | 生成代码 | 前端 | 测试 |
| --- | --- | --- | --- | --- |
| Lua | `common/yak/antlr4Lua/LuaLexer.g4`、`LuaParser.g4`（已有） | `common/yak/antlr4Lua/parser/`（已有） | `common/yak/lua/lua2ssa/` | `common/yak/lua/tests/` |
| Rust | `common/yak/rust/RustLexer.g4`、`RustParser.g4` | `common/yak/rust/parser/` | `common/yak/rust/rust2ssa/` | `common/yak/rust/tests/` |
| Kotlin | `common/yak/kotlin/KotlinLexer.g4`、`KotlinParser.g4`、`UnicodeClasses.g4` | `common/yak/kotlin/parser/` | `common/yak/kotlin/kotlin2ssa/` | `common/yak/kotlin/tests/` |
| Scala | `common/yak/scala/ScalaLexer.g4`、`ScalaParser.g4` | `common/yak/scala/parser/` | `common/yak/scala/scala2ssa/` | `common/yak/scala/tests/` |

生成脚本：

- `common/yak/rust/gen.sh`
- `common/yak/kotlin/gen.sh`
- `common/yak/scala/gen.sh`

三个脚本都调用 `../antlr4util/antlr4`，参数为 `-Dlanguage=Go -package <lang>parser ... -o parser -no-listener -visitor`。

## 3. 共用接线

以下文件把四个语言接入 2SSA 管线：

- `common/yak/ssaapi/ssaconfig/language.go`
  - 新增 `LUA`、`RUST`、`KOTLIN`、`SCALA` 常量。
  - 新增扩展名映射：`.lua`、`.rs`、`.kt`/`.kts`、`.scala`。
  - 新增别名：`lua`；`rust`/`rs`；`kotlin`/`kt`/`kts`；`scala`/`sc`。
- `common/yak/ssaapi/ssa_language_builder_creater.go`
  - 注册 `lua2ssa.CreateBuilder`、`rust2ssa.CreateBuilder`、`kotlin2ssa.CreateBuilder`、`scala2ssa.CreateBuilder`。
- `common/syntaxflow/sfpattern/language_files.go`
  - 增加 `.lua`、`.rs` 和对应别名。
- `common/yak/ssaapi/ssaconfig/scan_policies.yaml`
  - 增加 lua、rust、kotlin、scala 四组语言策略。
- `common/coreplugin/base-yak-plugin/SSA 项目探测.yak`
  - 增加四语言的扩展名、标记文件和清单文件。
- `scripts/ssa-test.sh`
  - 默认全量序列增加 `./common/yak/lua/...`、`./common/yak/rust/...`、`./common/yak/kotlin/...`、`./common/yak/scala/...`。

`--all-language` 不在当前分支。合并该选项的分支时，需要把四个语言加入该选项的语言清单；当前分支没有该清单，因此没有可修改的映射点。

## 4. Lua

### 4.1 语法来源

复用 `common/yak/antlr4Lua` 中的现有语法和生成代码。该语法基于 Lua 5.4 官方语法，并包含 Lua 5.1、5.2、5.3、5.4 与 Luau 的兼容形式。
`LuaParser.g4` 文件头记录了 Lua 5.2、5.3、5.4 测试套件和多个真实项目语料。

### 4.2 覆盖范围

- 支持 `local`、赋值、函数定义、表构造、`if`/`elseif`/`else`、`while`、`repeat`、数值 `for`、泛型 `for`、`break`、`return`。
- 支持 Lua 5.2 的 `goto` 和标签语法。
- 支持 Lua 5.3/5.4 的整除 `//`、按位运算、移位运算。
- 支持 Lua 5.4 的 `<const>` / `<close>` 属性语法。
- 词法层支持长字符串、长注释、十六进制数字、转义序列。

### 4.3 实测覆盖

| 样例 | 版本形态 | 结果 |
| --- | --- | --- |
| `common/yak/lua/tests/code/basic.lua` | 基础函数、表、循环 | PASS |
| `common/yak/lua/tests/code/lua51.lua` | Lua 5.1：`ipairs`、数值/泛型 `for` | PASS |
| `common/yak/lua/tests/code/lua52.lua` | Lua 5.2：`goto`、标签 | PASS |
| `common/yak/lua/tests/code/lua54.lua` | Lua 5.4：`<const>`、`//`、`&`、`<<` | PASS |

合计 4/4。测试入口：`TestLuaFrontendBasic`、`TestLuaFrontendVersionSamples`、`TestLuaProjectCompileBasic`。

### 4.4 SSA 降级边界

- `goto` 和标签可以解析，但当前不生成跳转 SSA，语句被跳过。
- 元表、`__index`、`__call` 等运行时语义不建模。
- 协程（`coroutine.*`）按普通外部函数调用处理。
- 可变参数 `...` 只生成占位值，不做参数展开。
- 面向对象式 `:` 方法调用会传递 `self` 参数，但不做动态分派。
- 泛型 `for` 的循环变量目前生成占位变量，不展开迭代器协议。

## 5. Rust

### 5.1 语法来源

- 上游：`antlr/grammars-v4`，目录 `rust/`，提交 `c306207ac870`。
- 文件：`RustLexer.g4`、`RustParser.g4`。
- 许可证：MIT，文件头保留原始版权声明。
- 本地修改：语义谓词中的 `this.` 改为 Go target 的 `p.`。

### 5.2 覆盖范围

- 支持 2015、2018、2021 edition 的常见语法：`mod`、`use`、`fn`、`let`、`const`、`static`、结构体、枚举、`impl`、`trait` 的语法解析。
- 支持表达式：字面量、路径、调用、方法调用、字段、索引、借用、解引用、分组、结构体字面量、宏调用。
- 支持控制流：`if`、`if let`、`match`、`loop`、`while`、`while let`、`for`、`break`、`continue`、`return`。
- 支持运算符：算术、比较、逻辑、赋值、复合赋值、位运算。

### 5.3 实测覆盖

语料目录：`/mnt/data/cache/grammars-v4/rust/examples`，包含 `quick/` 和 `long/`。

| 语料 | 文件数 | PASS | FAIL |
| --- | ---: | ---: | ---: |
| `rust/examples` | 16 | 16 | 0 |

版本样例：

| 样例 | 版本形态 | 结果 |
| --- | --- | --- |
| `common/yak/rust/tests/code/edition2015.rs` | Rust 2015 | PASS |
| `common/yak/rust/tests/code/edition2021.rs` | Rust 2021 | PASS |
| `common/yak/rust/tests/code/basic.rs` | 基础函数、循环、宏 | PASS |

测试入口：`TestRustFrontendBasic`、`TestRustFrontendEditionSamples`、`TestRustProjectCompileBasic`、`TestRustCorpusCoverage`。

### 5.4 SSA 降级边界

- `impl`、`trait` 只参与语法解析，不生成方法表或动态分派。
- `match` 只解析，不生成模式匹配分支的等价格式。
- 泛型参数、生命周期参数不参与类型推导。
- 闭包按匿名函数占位，不建模捕获变量。
- 异步块、`await` 按普通表达式占位。
- `for i in 0..3` 目前生成循环变量占位，不展开 Range 迭代协议。
- 宏调用按普通函数调用占位，不展开宏体。

## 6. Kotlin

### 6.1 语法来源与变体选择

- 上游：`antlr/grammars-v4`，目录 `kotlin/kotlin-formal/`，提交 `c306207ac870`。
- 文件：`KotlinLexer.g4`、`KotlinParser.g4`、`UnicodeClasses.g4`。
- 许可证：Apache-2.0，已随包放入 `common/yak/kotlin/LICENSE`。
- 只使用 `kotlin-formal` 一个变体。

选择理由：

- `kotlin-formal` 是纯 ANTLR 语法，不使用 `superClass`，不需要额外 Java 基类。
- 词法器和解析器都是独立文件，符合本仓库 Java/PHP 的组织方式。
- 语法来源是 Kotlin 官方语法规范，覆盖 Kotlin 1.x 与 2.x 的常见声明和表达式。

另一个变体 `kotlin/` 的参数对照：

| 变体 | Lexer 行数 | Parser 行数 | 需要基类 | 许可证 |
| --- | ---: | ---: | --- | --- |
| `kotlin/kotlin-formal`（已采用） | 450 | 893 | 否 | Apache-2.0 |
| `kotlin/kotlin`（未采用） | 516 | 839 | 否 | Apache-2.0 |

两个变体都是分离式 `Lexer.g4` / `Parser.g4`，都依赖 `UnicodeClasses.g4`。首版不引入第二套语法。

### 6.2 覆盖范围

- 支持 `package`、`import`、顶层 `fun`、`class`、`data class`、`object`、`interface`、`enum class`。
- 支持属性声明 `val` / `var`、多变量声明、类型标注、默认参数。
- 支持表达式：字面量、字符串、集合字面量、调用、成员访问、索引、二元/一元运算、`if`、`when`、`for`、`while`、`do while`、`return`、`break`、`continue`。
- 支持 `when` 的主体条件、逗号条件、`else` 分支；`is` 类型条件按真值占位。
- `.kts` 脚本使用独立入口 `ScriptFrontend`，复用同一套 `KotlinLexer.g4` / `KotlinParser.g4` 的 `script` 规则。

### 6.3 实测覆盖

语料目录：`/mnt/data/cache/grammars-v4/kotlin/kotlin-formal/examples`，包含 `fuzzer/`。

| 语料 | 文件数 | PASS | FAIL |
| --- | ---: | ---: | ---: |
| `kotlin-formal/examples` | 88 | 88 | 0 |

版本样例：

| 样例 | 版本形态 | 结果 |
| --- | --- | --- |
| `common/yak/kotlin/tests/code/classic1.kt` | Kotlin 1.x 风格：类、伴生对象、方法调用 | PASS |
| `common/yak/kotlin/tests/code/modern2.kt` | Kotlin 2.x 风格：`data class`、`sealed interface`、尾随逗号 | PASS |
| `common/yak/kotlin/tests/code/basic.kt` | 基础函数、循环、`when`、集合 | PASS |

测试入口：`TestKotlinFrontendBasic`、`TestKotlinFrontendVersionSamples`、`TestKotlinProjectCompileBasic`、`TestKotlinCorpusCoverage`、`TestAllSyntaxForKotlin_G4`。

### 6.4 SSA 降级边界

- 类、接口、对象只遍历成员函数，不建立继承表或虚方法分派。
- `when` 按 `if` / `else if` 链降级；`is` 类型条件不生成类型测试。
- 泛型、类型参数、委托属性不参与类型推导。
- Lambda、匿名对象、可调用引用只做占位。
- 协程（`suspend`、`launch`、`async`）按普通函数调用处理。
- 字符串模板只保留原始文本，不拆分插值表达式。
- 扩展函数、中缀函数按普通调用处理。

## 7. Scala

### 7.1 版本选择测量

测量方法：用 ANTLR 4.13.2 分别生成 Scala 2 与 Scala 3 语法，解析同一组样例。
初始 51 文件样本结果：

| 语法 | PASS | FAIL |
| --- | ---: | ---: |
| Scala 3 | 48 | 3 |
| Scala 2 | 11 | 40 |

Scala 3 明显覆盖更多 Scala 2 代码，因此选择 Scala 3 语法作为唯一语法。
随后对 Scala 2 高频语法做本地兼容，不增加第二套语法。

### 7.2 语法来源与本地补丁

- 上游：`antlr/grammars-v4`，目录 `scala/scala3/`，提交 `c306207ac870`。
- 本地文件：`ScalaLexer.g4`、`ScalaParser.g4`。
- 上游文件内没有单独许可证头；许可证按上游仓库条款执行。
- Java 基类移植：
  - `Scala3LexerBase.java`（532 行）→ `common/yak/scala/parser/scala_lexer_base.go`。
  - `Scala3ParserBase.java`（17 行）→ `common/yak/scala/parser/scala_parser_base.go`。

本地补丁：

1. `ScalaLexer.g4` 把 `BlockComment`、`LineComment` 移到 `Op` 规则之前。
   - 原因：空行注释 `//` 与 `Op` 同为 2 字符，规则顺序会让 `//` 被识别成运算符。
   - 效果：空 `//`、`/* */` 注释不再触发语法错误。
2. `scala_parser_base.go` 的 `migration30()` 默认返回 `true`。
   - 对应上游 `--3.0-migration` 选项。
   - 启用 Scala 2 的 `._` 通配符导入和 `_` 通配符类型。
   - 不改变 Scala 3 的 `.*` 和 `?` 写法。

### 7.3 覆盖范围

- 支持 Scala 3 缩进语法和花括号语法。
- 支持 `package`、`import`、顶层 `def`、`val`、`var`、`class`、`object`、`trait`、`enum`、`case class`、`given` 的语法解析。
- 支持表达式：字面量、引用、调用、成员访问、二元/一元运算、`if`、`while`、`for`、`return`、`throw`、赋值。
- 支持 Scala 2 常见形式：`._` 通配符导入、`_` 通配符类型、`for (i <- 1 to 3)`。

### 7.4 实测覆盖

语料目录：`/mnt/data/cache/grammars-v4/scala`，包含 `scala2/examples` 与 `scala3/examples`。

| 语料 | 文件数 | PASS | FAIL |
| --- | ---: | ---: | ---: |
| `scala/scala2/examples` | 6 | 6 | 0 |
| `scala/scala3/examples` | 188 | 188 | 0 |
| 合计 | 194 | 194 | 0 |

版本样例：

| 样例 | 版本形态 | 结果 |
| --- | --- | --- |
| `common/yak/scala/tests/code/legacy2.scala` | Scala 2：`._` 导入、`for (i <- 1 to 3)`、类 | PASS |
| `common/yak/scala/tests/code/modern3.scala` | Scala 3：缩进语法、`enum`、`case class` | PASS |
| `common/yak/scala/tests/code/basic.scala` | 基础对象、函数、循环 | PASS |

测试入口：`TestScalaFrontendBasic`、`TestScalaFrontendVersionSamples`、`TestScalaProjectCompileBasic`、`TestScalaCorpusCoverage`、`TestAllSyntaxForScala_G4`。

### 7.5 Scala 2 后续路径

当前不引入 Scala 2 语法。若后续需要更完整的 Scala 2 覆盖，可选两条路径：

1. 继续在 Scala 3 语法上打补丁。
   - 适合少量高频差异。
   - 风险是两套语义在同一语法中累积条件分支，维护成本上升。
2. 引入独立 Scala 2 语法。
   - 需要新增 `Scala2Lexer.g4` / `Scala2Parser.g4` 和独立 parser 包。
   - 需要在 `scala2ssa` 外增加版本选择逻辑。
   - 不推荐在首版使用。

Scala 3 基类已经移植完成。若改用 Scala 2 语法，不需要再移植 Scala 3 基类；但需要为 Scala 2 语法补自己的基类（若有）。

### 7.6 SSA 降级边界

- 类、对象、trait 只遍历成员定义，不建立继承表或虚方法分派。
- `enum`、`given`、`extension` 只解析，不生成等价 SSA。
- 模式匹配、`match` 类型、联合类型、类型类不降级。
- 泛型、上下文参数、隐式参数不参与类型推导。
- Lambda、`?=>`、`inline` 按普通表达式占位。
- 字符串插值只保留原始文本。
- `for` 推导式的生成器变量使用占位值，不展开迭代协议。

## 8. 覆盖率证据

覆盖率测试通过环境变量启用，默认测试跳过语料扫描：

| 语言 | 环境变量 | 结果文件 |
| --- | --- | --- |
| Rust | `RUST_CORPUS` | `/mnt/data/scan-outputs/feature-ssa-lua-rust-kotlin-scala/rust/coverage.tsv` |
| Kotlin | `KOTLIN_CORPUS` | `/mnt/data/scan-outputs/feature-ssa-lua-rust-kotlin-scala/kotlin/coverage.tsv` |
| Scala | `SCALA_CORPUS` | `/mnt/data/scan-outputs/feature-ssa-lua-rust-kotlin-scala/scala/coverage.tsv` |

运行方式：

```sh
export GOCACHE=/mnt/data/cache/go-build
export GOMODCACHE=/mnt/data/cache/go-mod
export GOTMPDIR=/mnt/data/cache/go-tmp
export TMPDIR="$GOTMPDIR"
export YAKIT_HOME=/mnt/data/scan-outputs/feature-ssa-lua-rust-kotlin-scala/<lang>/yakit-home
RUST_CORPUS=/mnt/data/cache/grammars-v4/rust/examples \
  scripts/ssa-test.sh -count=1 -timeout 10m -run TestRustCorpusCoverage -v ./common/yak/rust/tests
```

每个 `coverage.tsv` 的格式为 `RESULT<TAB>PASS|FAIL<TAB>文件路径<TAB>错误摘要`。

## 9. 首版未支持构造汇总

| 语言 | 未支持 |
| --- | --- |
| Lua | `goto` 跳转、元表分派、协程语义、可变参数展开、泛型 `for` 迭代器展开 |
| Rust | `impl`/`trait` 分派、模式匹配降级、泛型/生命周期推导、闭包捕获、`async`/`await`、宏展开、Range 迭代展开 |
| Kotlin | 继承/虚方法分派、`is` 类型测试、泛型推导、Lambda 捕获、协程、字符串模板插值、扩展函数分派 |
| Scala | 继承/虚方法分派、模式匹配、`given`/`extension`、泛型/隐式推导、Lambda 捕获、字符串插值、`for` 推导式生成器展开 |

## 10. 验证命令

```sh
export GOCACHE=/mnt/data/cache/go-build
export GOMODCACHE=/mnt/data/cache/go-mod
export GOTMPDIR=/mnt/data/cache/go-tmp
export TMPDIR="$GOTMPDIR"
export YAKIT_HOME=/mnt/data/scan-outputs/feature-ssa-lua-rust-kotlin-scala/final/yakit-home
mkdir -p "$YAKIT_HOME"

scripts/ssa-test.sh -count=1 -timeout 5m ./common/yak/lua/... ./common/yak/rust/... ./common/yak/kotlin/... ./common/yak/scala/...

go build -o /mnt/data/scan-outputs/feature-ssa-lua-rust-kotlin-scala/final/yak ./common/yak/cmd/yak.go
```

构建二进制、日志和覆盖率结果全部位于 `/mnt/data/scan-outputs/feature-ssa-lua-rust-kotlin-scala/`。

默认全量 `scripts/ssa-test.sh` 在本次环境中的结果：

- 新语言四个测试步骤全部通过。
- `./common/yak/ssa/...` 和 `./common/yak/ssa/ssadb` 在 20 秒包超时处失败。
- 失败包不属于本次修改范围，失败日志见 `final/full-suite.log`。

## 11. 真实项目语料与 CI

### 11.1 CI 语料入口

四个语言都在 `tests/` 提供统一入口：

```text
common/yak/<lang>/tests/syntax_<lang>_test.go
```

`TestAllSyntaxFor<Lang>_G4` 遍历 `tests/code/` 中对应扩展名的夹具，逐个调用语言
`Frontend`，默认单文件预算 10 秒；预算可通过
`YAK_<LANG>_FIXTURE_PARSE_BUDGET_SEC` 覆盖。失败消息包含文件名、耗时和解析错误。
Kotlin 的 `.kts` 夹具使用 `kotlin2ssa.ScriptFrontend`，`.kt` 使用
`kotlin2ssa.Frontend`。

`.github/workflows/syntax-g4-tests.yml` 的 matrix 已加入：

```text
./common/yak/lua/tests
./common/yak/rust/tests
./common/yak/kotlin/tests
./common/yak/scala/tests
```

触发路径沿用现有的 `common/yak/**/*.g4`、`common/yak/**/parser/**`、
`common/yak/**/code/**` 和 `common/yak/**/syntax_*.go`，不需要为新语言增加单独规则。

### 11.2 真实项目结果

扫描使用 `scripts/ssa-lang-corpus`，固定单文件预算 10 秒。项目源码不入库，结果在：

```text
/mnt/data/scan-outputs/feature-ssa-lua-rust-kotlin-scala/realworld/
```

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

Lua kong 的 `spec/fixtures/invalid-module.lua` 是故意非法样本，已用
`-exclude invalid-module.lua` 从统计中排除。Kotlin 的两个语料全部通过，包含 `.kts` Gradle 脚本。Scala 的剩余失败集中在
XML 字面量、宏展开和少数缩进边界；Rust 的剩余失败集中在 `let` 链、宏 token
树和宏调用；Lua 的剩余失败包含 `goto`/长注释边界样本。解析预算没有放宽。

### 11.3 性能口径

真实项目总耗时包含 SLL 优先解析失败后的 LL 回退。Lua 和 Kotlin 的语料覆盖率
提升后，部分文件开始进入完整解析路径，因此总耗时不能直接和修复前的失败短路
路径比较。Scala `simpleExpr` 改成 primary + suffix 循环后，ServiceReloader
从约 23 秒降到约 3.3 秒，zio 总耗时从 6m29s 降到 5m27s；Scalatra 仍有一个
缩进边界样本超过 10 秒，是本轮剩余性能风险。

## 12. 后续工作

- Scala 继续减少 SLL→LL 回退，优先处理 `for`/`if` 缩进和 XML 字面量。
- Rust 处理 `let` 链、宏 token 树和 `union` 上下文关键字。
- Lua 处理剩余长注释和 `goto` 边界样本。
- Kotlin 已覆盖 `.kts`；后续如需解析 Gradle DSL 语义，再单独设计脚本 SSA 降级。
