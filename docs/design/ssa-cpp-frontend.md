# C++ 前端（cpp2ssa）

本文记录 C++ 语法来源、与 C 链路的关系、SSA 降级边界、标准库基线和测试口径。

## 1. 结论：C++ 独立成语言，不与 C 合并

C++ 使用独立的语法包 `common/yak/cpp` 和前端包 `common/yak/cpp/cpp2ssa`。
`CParser.g4`、`c2ssa` 的语法和访问器保持原样。

### 1.1 证据

| 维度 | 实测 | 含义 |
| --- | --- | --- |
| 语法规则 | C 语法 109 条规则；CPP14 193 条规则；同名规则 36 条 | 同名规则在两边定义不同（如 `declarator`、`initializer`），不能直接合并。 |
| 关键字 | CPP14 词法器 128 个 token；C 词法器 128 个 token；`class`、`namespace`、`template`、`new`、`virtual` 等 42 个 C++ 关键字 C 侧不存在 | 合并必须重写词法器和全部关键字分支。 |
| 现有前端耦合 | `c2ssa` 访问器 4,533 行，全部绑定 `antlr4c/parser` 生成的上下文类型 | 改 `CParser.g4` 会改变规则编号和上下文类型，等于重写 C 前端。 |
| 实测兼容性 | 用现有 C 解析器解析 57 个真实 C++ 翻译单元：PASS 4，FAIL 53（7% 通过） | 扩展 C 语法要达到可用，需要实质性重写，而不是增量补规则。 |
| 语义差异 | 模板消歧需要 `CPP14ParserBase` 语义谓词；C 侧没有该基类 | 合并会把 C++ 专属谓词塞进 C 语法，影响 C 的预测路径。 |

实测命令与产物：

- C 解析器跑 C++ 语料：`/tmp/cparse-probe/main.go`（一次性探针），结果 `total=57 pass=4 fail=53`。
- 语料、日志：`/mnt/data/scan-outputs/cpp-support-<时间>/`。

### 1.2 结论

- 选独立语法包和独立前端。
- `CParser.g4` 不动，C 的语法、AST 形状、测试全部保持。
- 共用部分只放在语言无关的包里，见 §3。

### 1.3 风险与成本

- 风险：两套前端长期并行，C 与 C++ 的宏、include、类型处理出现分叉。
  缓解：预处理和字面量解析共用同一份代码，C 回归测试纳入默认序列。
- 成本：新增约 1,100 行语法文件、约 1,000 行访问器、一份标准库声明表。
- 已避免的成本：不改 `c2ssa` 的 4,533 行访问器，不重排 C 语法规则编号。

## 2. 语法来源与生成

| 项 | 内容 |
| --- | --- |
| 上游 | `antlr/grammars-v4`，目录 `cpp/` |
| 提交 | `de38599628d221099c046e92962f70204d852b54`（2026-11-17） |
| 文件 | `CPP14Lexer.g4`、`CPP14Parser.g4`、`Go/cpp14_parser_base.go` |
| 许可 | MIT（Camilo Sanchez 2015，Martin Mirchev 2020），全文见 `common/yak/cpp/LICENSE` |
| 生成器 | 仓库自带 ANTLR 4.13.2（`common/yak/antlr4thirdparty/antlr-4.13.2-complete.jar`） |
| 生成脚本 | `common/yak/cpp/gen.sh` |
| 生成产物 | `common/yak/cpp/parser/` |

许可证说明：任务描述写的是 Apache-2.0。上游 `cpp/` 目录的两个语法文件头写明 MIT，
`grammars-v4` 仓库根目录没有 LICENSE 文件。本文和 `NOTICE.md` 按实际许可证记为 MIT。

### 2.1 本地补丁

1. 语义谓词改用 Go 目标接收者 `p.`（对应上游 `cpp/Go/transformGrammar.py`）。
2. `cpp14_parser_base.go` 改为 `package cppparser`，ANTLR 运行时改为
   `github.com/yaklang/antlr/v4`，与生成代码同包。
3. `abstractDeclarator` 增加一条备选：
   `pointerOperator* LeftBracket constantExpression? RightBracket attributeSpecifierSeq?`。
   上游不接受无名数组类型，例如 `std::unique_ptr<char[]>`、`std::unique_ptr<const char*[]>`。
   cxxopts 等真实项目使用该写法。
4. C++17 结构（任务要求，见 §5.1）：折叠表达式、`noexcept(expr)` 运算符、
   `if constexpr`、结构化绑定（声明与范围 for）、嵌套命名空间定义 `namespace a::b {}`。
5. `literalOperatorId` 接受 `Operator UserDefinedLiteral`，
   使无空格的 `operator""_km` 可解析。
6. GNU `__attribute__((...))`：词法器新增 `AttributeGnu` 记号（放在 `Identifier` 前），
   语法器把双括号内的 `balancedTokenSeq` 当作属性。glibc 与 doctest 使用该写法。

补丁全部记录在 `common/yak/cpp/NOTICE.md` 和 `gen.sh` 注释里。

## 3. 与 C 共用和抽出的代码

| 部分 | 处理 | 说明 |
| --- | --- | --- |
| 宏、include、条件编译 | 直接复用 `common/yak/c2ssa/preprocess` | C++ 只提供配置，不复制实现。默认跳过系统头，项目内头文件内联。 |
| 字面量解析 | 抽出新包 `common/yak/cfamily/cliteral` | 整数、浮点、字符、字符串解码；C 和 C++ 共用。C 侧原函数改为一行转发。 |
| SSA 核心 | 复用 `common/yak/ssa` | `FunctionBuilder`、`Blueprint`、CFG 构建器、类型系统。 |
| 语法与 AST | 不共用 | 生成包不同，上下文类型不同。 |
| 访问器 | 不共用 | C++ 有类、命名空间、模板、`new`/`delete`。 |
| 类型映射与标准库 | C++ 专属 | `common/yak/cpp/cpp2ssa/type.go`、`stdlib.go`。 |

对共享代码的改动都保持 C 默认行为：

- `PreprocessConfig` 新增 3 个字段，零值等于原行为：
  - `ResolveProjectAngleIncludes`：C 为 false，C++ 为 true。
  - `HeaderExtensions`：为空时只索引 `.h`，C 保持原样；C++ 增加 `.hpp`、`.hxx`、`.hh` 等。
  - `IncludeResolver.candidatePaths` 仅在开关打开时增加 include 目录候选。
- 新包 `cliteral` 接受 C++14 数字分隔符和后缀；C 原有输入结果不变。

C 回归：`./scripts/ssa-test.sh -count=1 -timeout 5m ./common/yak/c2ssa/...` 全部通过
（`c2ssa`、`c2ssa/preprocess`、`c2ssa/test`）。

## 4. 管线接入

| 位置 | 内容 |
| --- | --- |
| `common/yak/ssaapi/ssaconfig/language.go` | `CPP = "cpp"`；主扩展名 `.cpp`；别名 `cpp`、`c++`、`cplusplus`、`cc`、`cxx`。 |
| `common/yak/ssaapi/ssa_language_builder_creater.go` | 注册 `cpp2ssa.CreateBuilder`。 |
| `common/yak/ssaapi/ssaconfig/scan_policies.yaml` | 语言策略组 `cpp` / `C++`。 |
| `common/syntaxflow/sfpattern/language_files.go` | 规则文件上限：`.cpp`、`.cc`、`.cxx`、`.hpp`、`.hxx`、`.hh`、`.h`；语言别名同上。 |
| `common/coreplugin/base-yak-plugin/SSA 项目探测.yak` | 扩展名、标记文件、清单文件、CLI 选项 `C++`。 |
| `scripts/ssa-test.sh` | 增加 `go test -timeout 5m ./common/yak/cpp/...`。 |
| `.github/workflows/syntax-g4-tests.yml` | matrix 增加 `./common/yak/cpp/tests`。 |

### 4.1 扩展名与 `.h` 归属

规则如下，按优先级执行：

1. 扩展名注册表把 `.h` 归给 C，把 `.hpp`、`.hxx`、`.hh` 归给 C++。
2. C++ builder 只把 `.cpp`、`.cc`、`.cxx` 当翻译单元。
3. 头文件不作为翻译单元编译。C++ 翻译单元通过 `#include` 内联看到头文件声明，
   因此 `.h` 归 C 不会丢失 C++ 代码里的声明。
4. SyntaxFlow 的 C++ 规则上限包含 `.h`：规则允许读 C 风格头文件，避免漏报。
5. `.h` 的内容判断只在 include 内联阶段发生：
   - 项目内尖括号 include（`#include <name.hpp>`）先在项目注册表解析，
     包括当前文件目录，然后才按系统头跳过；
   - 系统头（`<vector>`、`<iostream>`）不展开，由标准库占位类型兜底。

未覆盖：纯头文件项目（只有 `.hpp`、没有 `.cpp`）不会产生翻译单元。
绕过方式：加一个空 `.cpp`，或把入口头文件改为 `.cpp`。

## 5. 能力边界

### 5.1 已覆盖

- 命名空间、嵌套命名空间、namespace 别名、匿名命名空间内容。
- 类、结构体、联合体；单继承与多继承的基类登记；嵌套类。
- 成员函数（类内定义和类外 `Class::method` 定义）、静态成员、成员字段。
- 构造函数、析构函数，含成员初始化列表（`Foo() : x_(0) {}`）。
- 函数模板、类模板、模板参数的声明解析；显式实例化/特化访问内层声明。
- 引用与指针：声明类型、`&x`、`*p`、`->` 成员访问。
- `new` / `new[]` / `delete`：生成类型化的占位值；已知构造器时发射构造调用。
- `using namespace`、`using std::name`、`typedef`、`using Alias = T`。
- 语句：复合语句、`if`/`else`、`switch`/`case`/`default`、`for`、范围 `for`、
  `while`、`do`、`break`、`continue`、`return`、`try`/`catch`。
- 表达式：字面量、标识符、限定名、成员访问、下标、调用、一元/二元运算、
  复合赋值、三目运算、类型转换（C 风格与 `static_cast` 等）、lambda、花括号初始化。
- C++11/14 常用写法：`auto`、`nullptr`、`enum class`、`override`/`final`、
  `constexpr`、尾置返回类型、`noexcept` 说明符与 `noexcept(expr)` 运算符、
  属性 `[[...]]` 与 GNU `__attribute__((...))`。
- C++17 常用写法：`if constexpr`、结构化绑定、折叠表达式、
  嵌套命名空间定义、`inline namespace`、类模板参数推导的语法外壳。
- 模板声明层：类模板、成员函数模板、全特化与偏特化、模板参数默认值、
  可变参数模板、别名模板、`typename`/`template` 消歧、嵌套 `>>`。
- 运算符重载（含转换函数、`operator()`、`operator[]`）与用户定义字面量。

### 5.2 未覆盖

- 模板实例化语义：不按模板参数生成专用函数/类；只建一份声明占位。
- 重载决议与运算符重载语义：同名方法后注册的覆盖先注册的。
- 虚函数分派：只登记基类关系，不做动态派发求解。
- 多重继承的对象布局、虚基类、RTTI（`dynamic_cast`/`typeid` 只透传操作数）。
- 编译期求值：`constexpr` 函数、`sizeof`/`alignof` 求值（返回常量 0）。
- C++20 语法未纳入：`concept`/`requires`、`<=>`、指定初始化器
  （`S s{.x = 1}`）、modules、coroutines。遇到即解析失败，不做降级。
- MSVC 扩展 `__declspec(...)` 未纳入；GNU `__attribute__((...))` 已纳入。
- `goto` 不建立 CFG 边，只记录调用式占位；`label:` 直接访问内部语句。
- 宏展开的剩余限制：`__VA_OPT__`、`#include` 计算式路径、
  `#define` 内嵌 `#if` 的分支收集、`#` 字符串化保留原始宏实参
  （当前对实参先做宏展开，ISO C 规定 `#`/`##` 操作数不展开）。
- 纯头文件项目不产生翻译单元，见 §4.1。

## 6. 标准库基线

目标：真实项目不因缺少标准类型而直接失败。实现方式是**声明占位**，不是行为模拟。

### 6.1 覆盖范围

| 头文件 | 类型 | 占位成员（示例） |
| --- | --- | --- |
| `<string>`、`<string_view>` | `string`、`wstring`、`basic_string`、`string_view` | `size`、`length`、`substr`、`c_str`、`find`、`append`、`operator[]` |
| `<vector>`、`<array>`、`<deque>`、`<list>`、`<forward_list>`、`<initializer_list>` | 同名容器 | `push_back`、`pop_back`、`emplace_back`、`insert`、`erase`、`at`、`begin`、`end` |
| `<map>`、`<unordered_map>` | `map`、`multimap`、`unordered_map` | `insert`、`find`、`count`、`at`、`operator[]` |
| `<set>`、`<unordered_set>` | `set`、`multiset`、`unordered_set` | `insert`、`find`、`count`、`contains` |
| `<utility>`、`<tuple>` | `pair`、`tuple` | 字段 `first`、`second` |
| `<memory>` | `unique_ptr`、`shared_ptr`、`weak_ptr` | `get`、`reset`、`release`、`use_count` |
| `<optional>` | `optional` | `has_value`、`value`、`value_or` |
| `<stack>`、`<queue>` | `stack`、`queue`、`priority_queue` | `push`、`pop`、`top`、`front`、`size` |
| `<iostream>`、`<sstream>`、`<fstream>` | `istream`、`ostream`、`iostream`、`stringstream`、`ifstream`、`ofstream` | `read`、`write`、`getline`、`open`、`close`、`str` |
| `<exception>`、`<stdexcept>`、`<new>`、`<typeinfo>` | `exception`、`runtime_error`、`logic_error`、`bad_alloc` 等 | `what`、`code`、`message` |
| `<thread>`、`<mutex>`、`<condition_variable>`、`<atomic>`、`<future>` | `thread`、`mutex`、`atomic`、`future`、`promise` | `lock`、`unlock`、`join`、`wait`、`load`、`store` |
| `<functional>` | `function`、`bind`、`reference_wrapper` | `operator()`、`target` |
| `<iterator>` | `iterator`、`const_iterator` | `begin`、`end`、`next` |
| `<cstddef>`、`<cstdint>` | `size_t`、`ptrdiff_t`、`int*_t` 等 | 映射为数值类型，不建 blueprint |

容器类型直接映射到 SSA 类型：

- `vector`/`array`/`deque`/`list` → 切片类型。
- `map`/`unordered_map` → 映射类型。
- `set`/`unordered_set` → 键到布尔的映射类型。
- `unique_ptr`/`shared_ptr`/`optional` → 元素类型本身（不区分所有权包装）。
- `string` → 字符串类型，同时建 `std::string` blueprint 提供成员名。

导出接口：`cpp2ssa.StdlibHeaders()`、`cpp2ssa.StdlibTypeNames()`，供文档与测试守护。

### 6.2 未覆盖

- 没有任何行为：占位方法体为空，返回 undefined。数据流不穿过库函数。
- 未覆盖的头文件：`<algorithm>`、`<numeric>`、`<regex>`、`<chrono>`、`<filesystem>`、
  `<random>`、`<cmath>` 等。它们的类型按未知类型建占位 blueprint，调用不报错，
  但也不登记成员。
- 不处理模板默认参数、特化、别名模板、`allocator` 参数。
- 不区分 `std::` 版本命名空间（C++17 起无 `std::experimental` 处理）。

### 6.3 取舍：一份声明集合

标准库只维护 `stdlib.go` 里的一份表，不按 C++11/14/17/20 拆多套。
理由：

- 前端目标是解析和基础类型，不做版本相关的语义。
- 同一个头文件在不同标准下只有新增重载/成员，占位表取并集即可。
- 多套表会引入「按语言版本选表」的配置面，收益低于维护成本。

## 7. 取舍：一套语法覆盖全部特性

结论：只用一份 `CPP14Lexer.g4` + `CPP14Parser.g4`，不按 C++11/14/17/20 拆多套。

理由：

- 同一种语言只保留一套语法，是 `docs/design/ssa-frontend.md` §1 的既有口径。
- 上游 CPP14 语法本身接受大量后续标准的写法（`auto`、`enum class`、`nullptr`
  等早在 C++11 就有）；拆分版本会重复 1,500 行语法。
- 版本差异主要是新增关键字和库设施，不是语法结构重写。新增写法的缺口按实际
  语料补丁修复，并记录在 `NOTICE.md`，例如无名数组模板实参。
- 代价：C++20 的 concepts/requires/modules/coroutines 只按 CPP14 的接受度处理，
  不支持时表现为该文件解析失败，而不是切换到另一套语法。

## 8. 测试

| 层级 | 位置 | 命令 |
| --- | --- | --- |
| 语法 | `common/yak/cpp/tests/syntax_cpp_test.go` | `go test ./common/yak/cpp/tests -run '^TestAllSyntaxForCpp_G4$'` |
| 前端注册 | `common/yak/cpp/cpp2ssa/builder_test.go` | `go test ./common/yak/cpp/cpp2ssa` |
| 端到端 SSA | `common/yak/cpp/tests/builder_test.go` | `go test ./common/yak/cpp/tests` |
| 公共管线 | `common/yak/ssaapi/test/ssatest/frontend_languages_test.go` | `go test ./common/yak/ssaapi/test/ssatest` |
| 真实语料 | `common/yak/cpp/tests/coverage_test.go` | `CPP_CORPUS=<dir> go test ./common/yak/cpp/tests -run TestCppCorpusCoverage -v` |
| C 回归 | 既有包 | `./scripts/ssa-test.sh -count=1 ./common/yak/c2ssa/...` |

语料模式：

- 默认：只测语法（`cpp2ssa.Frontend`）。
- `CPP_CORPUS_PREPROCESS=1`：先做 C++ 预处理，再解析，接近生产管线。

### 8.1 真实语料实测（2026-11-18）

语料：`cxxopts`（MIT）、`argparse`（MIT），共 57 个 `.cpp` 翻译单元。
语料不进入 git，存放于 `/mnt/data/scan-outputs/cpp-support-<时间>/corpus/`。

基线提交 `4b69840d81`（宏健壮性修复前）与当前提交用同一语料、同一统计脚本对比。
每个顶层目录按独立项目预处理：两个项目各自带一份测试框架头文件，
宏表跨项目合并会互相覆盖 `TEST_CASE`，那是检出布局的产物，不是前端行为。

| 测法 | 修复前 PASS/57 | 修复后 PASS/57 | p50（前→后） | p95（前→后） | 最大（前→后） |
| --- | --- | --- | --- | --- | --- |
| 现有 C 解析器（对照） | 4 | 4 | — | — | — |
| C++ 语法，纯解析 | 22 | 22 | 8.7→6.4 ms | 172.0→203.5 ms | 246.3→257.7 ms |
| C++ 语法 + 预处理 | 23 | **57** | 18.3→14.3 ms | 191.2→1201.4 ms | 3,366.8→4,306.5 ms |

修复前的失败分布：34 个失败全部来自 doctest/Catch2 测试文件，
根因是共享宏展开器的 5 个缺陷（见 §8.2），库源码文件全部通过。
修复后这 34 个文件全部通过；纯解析仍是 22，因为纯解析不做 include 与宏展开。

预处理耗时上升集中在测试文件：`options.cpp` 4.3 s、`test_scan.cpp` 1.8 s、
`test_repr.cpp` 1.2 s。这些文件以前快速失败（宏不展开），现在真正展开出
数百个 `do { ... } while(false)` 断言块再解析。库源码文件仍在 10 ms 量级。

### 8.2 语法特性夹具与宏修复清单

夹具位置：`common/yak/cpp/tests/code/`。`TestAllSyntaxForCpp_G4` 逐个解析，
预算 10 s。宏夹具在子目录 `code/macros/`，由 `macro_test.go` 先预处理再解析。

| 特性组 | 夹具 | 覆盖点 |
| --- | --- | --- |
| 模板 | `templates.cpp`、`templates_advanced.cpp` | 类模板、成员函数模板、全特化、偏特化、默认参数、可变参数、别名模板、`typename`/`template` 消歧、`>>`、`extern template` |
| 宏 | `code/macros/{framework.hpp,macros.cpp}` | 对象宏、函数宏、`__VA_ARGS__`、`#`、`##`、多行宏体、多行调用、`#if/#elif/#else`、宏体行注释、`#x##suffix` |
| 运算符 | `operators.cpp` | 算术/比较/下标/调用/转换运算符、成员与非成员重载、用户定义字面量 |
| lambda 与基础 | `lambdas.cpp` | 捕获列表、`mutable`、泛型 lambda、尾置返回、`noexcept`、`[[nodiscard]]`、范围 for、`auto`、`decltype` |
| C++17 | `cxx17.cpp` | 结构化绑定、`if constexpr`、折叠表达式、嵌套命名空间、`inline namespace` |

本轮修复的共享宏展开缺陷（`common/yak/c2ssa/preprocess`，C 与 C++ 共用）：

| 缺陷 | 影响 | 修复 |
| --- | --- | --- |
| 多行宏调用按行展开 | `TEST_CASE("..." * tag("x"))` 跨行时不展开 | 括号未配平时缓存逻辑行，配平后整段展开 |
| `#elif` 设置了 `elseSeen` | `#if 0 / #elif 0 / #else` 的 `#else` 被跳过，`DOCTEST_UNUSED` 缺失 | `elif` 不再设置 `elseSeen` |
| 指令名取整个字段 | `#endif// x` 被当作未知指令，条件栈失衡，`SECTION` 定义落在“非活动”分支 | 指令名只取前导标识符串 |
| `#` 字符串化不折叠空白、不转义 | 多行实参产生含换行的字符串，导致词法错误 | 空白折叠为一个空格，转义 `\` 与 `"` |
| `#x##suffix` 不粘贴 | `#matcher##_catch_sr` 残留 `##` | 字符串化结果参与同一轮 `##` 折叠 |
| 宏体行注释保留 | `} while(false) // NOLINT` 吞掉调用方的 `;` | 宏体注释按翻译阶段 3 变为一个空格 |
| `"a" "b"` 相邻字面量不合并 | 宏展开产生相邻字符串时解析失败 | 新增翻译阶段 6 合并（raw string 除外） |

## 9. 剩余风险

- 宏展开仍不是完整预处理器：`__VA_OPT__`、`#`/`##` 操作数先展开、
  `#define` 内嵌 `#if` 的分支收集都有偏差；语料未覆盖的宏写法可能仍失败。
- 预处理后解析耗时上升：测试框架宏真正展开后，单文件最大 4.3 s（原 3.4 s 快速失败）。
- 模板只做声明解析，模板实例化后的成员访问可能落到占位类型。
- 头文件只通过 include 内联，纯头文件项目不产生翻译单元。
- 标准库占位没有行为，跨库数据流不成立。
- 共享预处理新增 3 个开关，C 默认值不变；C 回归测试必须继续在默认序列里跑。