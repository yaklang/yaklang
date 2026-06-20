`sca` 库是软件成分分析（Software Composition Analysis）工具，扫描文件系统、Git 仓库或容器镜像，识别其中的第三方组件与版本，为供应链安全与已知漏洞关联提供基础。

典型使用场景：

- 扫描来源：`sca.ScanLocalFilesystem` / `sca.ScanFilesystem` 扫描目录，`sca.ScanGitRepo` 扫描仓库，`sca.ScanImageFromFile` / `sca.ScanImageFromContext` / `sca.ScanContainerFromContext` 扫描镜像/容器，返回组件包列表。
- 定制：`sca.analyzers` / `sca.customAnalyzer` 定制分析器，`sca.scanMode` / `sca.concurrent` 控制扫描方式与并发，`sca.NewAnalyzerResult` 构造自定义包结果。

与相邻库的关系：`sca` 识别出的组件与版本常交给 `cve`/`cwe` 关联已知漏洞，与 `git`（仓库）、`filesys`（文件系统）、`diff`（版本比对）配合用于供应链审计。
