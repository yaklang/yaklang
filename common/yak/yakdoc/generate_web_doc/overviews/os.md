`os` 库提供操作系统交互能力，覆盖环境变量、进程信息、主机/网络信息、本地端口探测与文件系统基础操作，是采集本机环境与做系统级判断的入口。

典型使用场景：

- 主机与网络信息：`os.Hostname` / `os.GetMachineID` / `os.GetHomeDir`，`os.GetLocalIPv4Address` / `os.GetLocalAddress` / `os.LookupIP` / `os.LookupHost` 获取地址与解析。
- 端口探测：`os.GetRandomAvailableTCPPort` 取空闲端口，`os.IsTCPPortOpen` / `os.IsRemoteTCPPortOpen` / `os.IsTCPPortAvailable` 判断端口状态，`os.WaitConnect` 等待可连接。
- 进程与环境：`os.Getpid` / `os.Getppid` / `os.Getuid`，`os.Getenv` / `os.Setenv` / `os.Environ` / `os.ExpandEnv` 管理环境变量。
- 文件与监控：`os.Remove` / `os.RemoveAll` / `os.Rename` / `os.TempDir`，`os.NewProcessWatcher` / `os.NewConnectionsWatcher` 监控进程/连接。

与相邻库的关系：`os` 与 `env`（环境变量）、`exec`（执行命令）、`file`（文件操作）、`hids`（主机监控）协同，是系统层信息采集的基础。
