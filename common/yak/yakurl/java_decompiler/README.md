# Java 反编译 YakURL 使用说明

本文说明 `javadec://` 的调用方式、响应字段、源码视图规则，以及
`codeSource` 为什么同时持有两套文件系统。

## 1. 入口与整体流程

Yakit 的 Java 反编译页面只通过三个步骤使用 YakURL：

```text
文件树   GET javadec:///jar-aifix?jar=<jar-or-dir>&dir=<dir>
打开文件 GET javadec:///class-aifix?class=<...java>&jar=<root>&innerClasses=<a.java,b.java>
导出 ZIP GET javadec:///export?jar=<root>
```

`/jar`、`/class` 是同一套逻辑的普通版本；Yakit 页面实际使用
`/jar-aifix` 和 `/class-aifix`，两者共享同一个源码视图。

```text
jar/dir
  │
  ├─ /jar-aifix ──> listSourceEntries ──> .class 显示为 .java
  │                                      └─ Extra.originalPath = 真实 .class
  │
  ├─ /class-aifix ─> sourceClassPath(.java -> .class)
  │                  └─ readClassFile ─> JarFS.ReadFile ─> 反编译源码
  │
  └─ /export ──────> writeExportTree ──> 只写 .java
```

## 2. 路由与参数

| 路由 | 参数 | 说明 |
| --- | --- | --- |
| `GET /jar` | `jar` 必填，`dir` 可选 | 列出目录；class 条目显示为 `.java` |
| `GET /jar-aifix` | `jar`、`dir` 必填 | Yakit 文件树使用的目录；带 inner class 隐藏信息 |
| `GET /class` | `jar`、`class` 必填 | 读取源码；`class` 可为 `.java` 或 `.class` |
| `GET /class-aifix` | `jar`、`class` 必填，`innerClasses` 可选 | Yakit 打开文件使用；合并内部类源码 |
| `GET /export` | `jar` 必填 | 导出 ZIP，class 条目为 `.java` |

`jar` 可以是：

- `.jar` / `.war` / `.ear` / `.zip` 文件；
- 一个目录，目录内可以放 class 文件和嵌套归档。

`dir` 是归档内路径或目录根下的相对路径。根目录写 `.`。
嵌套归档按普通目录导航，例如 `WEB-INF/lib/dep.jar`。

`innerClasses` 是逗号分隔的显示路径列表。Yakit 从
`Extra.innerClass` 原样取出后拼到 URL 上。

## 3. 响应约定

### 3.1 目录列表

每个 `Resources` 项的主要字段：

| 字段 | class 条目 | 目录或归档 | 普通资源 |
| --- | --- | --- | --- |
| `ResourceName` | `Main.java` | `com` | `MANIFEST.MF` |
| `Path` | `com/Main.java` | `com` | `META-INF/MANIFEST.MF` |
| `VerboseName` | `Main.java`（`/jar` 带大小后缀） | `com` | `MANIFEST.MF` |
| `ResourceType` | `file` | `dir` | `file` |
| `VerboseType` | `java-source` | `java-directory` | `java-file` |
| `HaveChildrenNodes` | 无 | 是否有子项 | 无 |
| `Url.Path` | `/class` 或 `/class-aifix` | `/jar` 或 `/jar-aifix` | `/jar` 或 `/jar-aifix` |

class 条目额外带：

- `Extra.originalPath`：真实 `.class` 路径。
- `Extra.innerClass`：外部类才有的内部类列表。
- `Extra.outerClass`、`Extra.hide=true`：内部类条目，前端据此隐藏。

同名 `.class` 与 `.java` 只列一次，字节码条目优先。

### 3.2 读取源码

`/class` 与 `/class-aifix` 返回一个 `ResourceType=class` 的资源，
源码放在 `Extra.content`，值为 hex 编码的 UTF-8：

```text
Extra: [{Key: "content", Value: "7061636b616765 ..."}]
```

`/class-aifix` 会把外部类和 `innerClasses` 的源码依次拼接，并增加
`Extra.innerClassesIncluded`。普通 `/class` 的 `VerboseType` 为
`decompiled-java-class`，`/class-aifix` 为 `ai-enhanced-java-class`。

### 3.3 导出

`/export` 返回一个 ZIP 资源，`Path` 和 `Extra.path` 都是 ZIP 的绝对
路径。ZIP 内的 class 条目全部写为 `.java`；非代码资源保持原名。

## 4. 为什么有两个文件系统

`codeSource` 持有两个字段：

```go
expandedFS fi.FileSystem              // 目录树、嵌套归档、非 class 资源
jarFS      *javaclassparser.JarFS     // class 反编译、嵌套归档内的 class
```

这是 javajive 的职责划分，不是重复解包。

### 4.1 `expandedFS` 负责“看见”

`ExpandedZipFS` 提供虚拟目录树：

- `ReadDir` 列出归档目录；
- `Stat` 把 `.jar` 文件当作可展开目录；
- `GetJarFS` 打开嵌套归档；
- 对目录根，`ReadFile` 会解析本地 `.class`。

但归档根的 `ExpandedZipFS.ReadFile` 走
`readSelectedJarEntry`（`classparser/jar_release_selection.go`），
返回物理条目原始字节。这是读取嵌套 jar 内容所需要的语义，不能拿
它当反编译入口。

### 4.2 `jarFS` 负责“解析”

`JarFS.ReadFile` 走 `ReadClassOutcome` → `decompileClassOutcome`，
返回 Java 源码；嵌套路径由 `getNestedJarFS` 解析：

```text
JarFS.ReadFile("lib/dep.jar/com/A.class")
  └─ getNestedJarFS("lib/dep.jar") -> nested JarFS
       └─ ReadFile("com/A.class") -> source
```

`JarFS` 不提供“把嵌套归档当目录展开”的接口，所以不能只保留它。

### 4.3 两者共享索引

归档根构造方式：

```go
jarFS := javaclassparser.NewJarFSFromLocal(rootPath)
expandedFS := javaclassparser.NewExpandedZipFS(jarFS.ZipFS, jarFS.ZipFS)
```

两个视图使用同一个根 `ZipFS`，根归档只解析一次。`expandedFS` 负责
列表，`jarFS` 负责根 class 反编译。

目录根使用 `ExpandedLocalFileSystem`；目录根读取嵌套归档时通过
`expandedFS.GetJarFS` 取同一份嵌套索引。

## 5. 历史：为什么以前显示 `.class`，打开却是原始字节

### 5.1 正常时期

`86f1475681` 之前，`codeSource` 持有 `jar.JarParser`：

```go
if strings.HasSuffix(relativePath, ".class") {
    return cs.jarParser.DecompileClass(relativePath)
}
```

`/class` 直接调用 `DecompileClass`，前端看到的是解析后的 Java。

### 5.2 回归

`86f1475681` 把 `jar.JarParser` 换成了
`ExpandedArchiveFileSystemFromLocal`，同时删掉了 `/class` 里的
`DecompileClass` 分支。新的读取路径变成：

```go
return cs.expandedFS.ReadFile(cs.resolveFSPath(relativePath))
```

归档根命中 `ExpandedZipFS` 的 `zipFS != nil` 分支，调用
`readSelectedJarEntry`，返回原始 class 字节。`/class` 再把它 hex
编码给前端，打开文件就是 `CAFEBABE`。

修复前实测：tools.jar 4710 个 class 中 4710 个返回原始字节；
jd-gui 1607 个 class 中 1607 个返回原始字节。

### 5.3 `.class` 名字的来源

`86f1475681` 后 UnifiedFS 把 class 条目映射成 `.java`。提交
`975514fe3f` 为了保持文件树导航，主动把显示名恢复成 `.class`。
所以旧页面看到的是：

```text
com/example/Main.class     <- 列表显示
打开后 CAFEBABE 二进制      <- /class 返回原始字节
```

### 5.4 当前行为

- 列表显示 `com/example/Main.java`，目录结构不变。
- 打开走 `/class-aifix`，`sourceClassPath` 把 `.java` 映射回
  `.class`，再走 `JarFS.ReadFile` 反编译。
- 旧客户端继续传 `.class` 也能读取同一份源码。
- 导出与列表使用同一个 `listSourceEntries`，不会出现列表 `.java`、
  导出 `.class` 的不一致。

## 6. 前端兼容性

核对对象：Yakit `f2c6ef0b4d`，目录
`app/renderer/src/main/src/pages/yakJavaDecompiler`。

- 文件树：`utils.ts` 使用 `/jar-aifix`，取 `VerboseName` 作为节点名。
- 打开：`YakJavaDecompiler.tsx` 用 `resource.Path` 拼
  `/class-aifix`，并从 `Extra.innerClass` 收集内部类。
- 下载：`RunnerTabs.tsx` 只把 `.class` 改名为 `.java`；现在名字
  已经是 `.java`，行为不变。
- 导出：`RunnerFileTree.tsx` 使用 `/export`，读取 `Path` 或
  `Extra.path`。
- 隐藏内部类：`utils.ts` 过滤 `Extra.hide=true`，依赖字段未变。

前端不依赖 `.class` 后缀。`Extra.originalPath` 保留原始 class 路径。

## 7. 测试

快速用例不依赖外部 jar：

```sh
scripts/ssa-test.sh -count=1 ./common/yak/yakurl/java_decompiler
```

真实 jar 抽样：

```sh
JAVA_DECOMPILER_TEST_JARS=/path/tools.jar,/path/jd-gui.jar \
  scripts/ssa-test.sh -run TestJavaDecompilerClassRouteRealJars -count=1 -v \
  ./common/yak/yakurl/java_decompiler
```

全量读取并导出：

```sh
JAVA_DECOMPILER_TEST_JARS=... JAVA_DECOMPILER_FULL=1 \
  scripts/ssa-test.sh -run TestJavaDecompilerClassRouteRealJars -count=1 -v -timeout 50m \
  ./common/yak/yakurl/java_decompiler
```

相关用例：

| 用例 | 覆盖 |
| --- | --- |
| `TestJavaDecompilerClassRouteReturnsSourceFromSmallJar` | 小 jar 列表、读取、导出 |
| `TestJavaDecompilerAifixFrontendFlow` | 前端三个 URL、树结构、innerClass、去重、嵌套归档 |
| `TestJavaDecompilerSourceViewExportAndDedup` | 同名 class/java、嵌套导出 |
| `TestJavaDecompilerDirectorySourceView` | 目录根松散 class 与嵌套 jar |
| `TestJavaDecompilerClassRouteRealJars` | tools.jar / jd-gui 抽样与全量 |

## 8. 已知限制

- 同名 `.class` 与 `.java` 优先字节码，真实 `.java` 源码被遮蔽。
- 少数合成内部类只有 javajive 的注释占位，没有 `package`/`class`。
- `/export` 仍包含非代码资源，保持旧导出行为。
- 目录根导出名取目录 basename；临时目录会得到类似
  `001-decompiled.zip` 的名字。