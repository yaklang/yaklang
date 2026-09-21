`sca` 库分析调用方提供的文件系统材料，识别组件、版本声明与依赖证据。

- `sca.ScanLocalFilesystem` / `sca.ScanFilesystem` 扫描已经准备好的目录或虚拟文件系统。
- `sca.analyzers` / `sca.customAnalyzer` 选择内置分析器或注入受信任回调；`sca.scanMode` / `sca.concurrent` 配置扫描。
- SCA 不再获取 Docker 镜像、容器或 Git 历史。调用方自行准备材料；不同提交应分别扫描，分别保留快照标识。

识别结果可交给 `cve`/`cwe` 做关联。声明版本、校验记录与实际安装证据应分别解释。
