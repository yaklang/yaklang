# Windows npipe 启动兼容性

本次验证针对 `feature/engine-distribution-optimization` 的 Windows 引擎启动链路。
TCP 仍是默认传输；显式传入 `--transport npipe --socket-path ...` 后，引擎和检查命令均使用命名管道。
IPC 必须配置有效的 `--local-password` 或 `--secret`，所有 gRPC 请求仍经过密码认证。

## 初轮本机验证结果（2026-09-10）

环境：Windows 11 Pro，10.0.26200，amd64；Go 1.22.12，CGO 开启。
源码基点：`9571a2e4e`。以下结果包含本次工作区修改。

| 场景 | 结果与范围 |
| --- | --- |
| 非管理员启动 | 普通桌面会话完成预检、真实引擎启动、带密码 Version/Echo 调用 |
| 更低的可选权限 | 测试子进程禁用管理员 SID、删除可选特权并设置 Medium 完整性级别；创建管道和启动真实引擎成功 |
| 管理员服务端 → 普通客户端 | 底层双向数据通信与真实带密码 gRPC 调用成功 |
| 普通服务端 → 管理员客户端 | 底层双向数据通信与真实带密码 gRPC 调用成功 |
| 存量数据权限切换 | 管理员创建隔离数据库 → 普通权限使用同一数据库重启 → 管理员再次重启，均成功 |
| 跨盘符 | 引擎位于 `Y:`，数据/工作目录及客户端位于 `C:`，中文、空格、`&` 路径均成功；`Y:` 是 SUBST 映射，不是第二块真实硬盘 |
| 旧端口被占用 | 占用 9011 时 npipe 预检和启动成功 |
| 代理环境 | HTTP_PROXY / HTTPS_PROXY 指向不可用的本地代理时，npipe 预检和通信成功 |
| 同名管道 | 返回 `ipc_bind_in_use`；原引擎继续接受请求 |
| 强制结束后重启 | 同名管道释放，使用相同数据和管道名重新启动成功 |
| 密码 | 正确密码成功；匿名及错误密码失败，混合权限场景同样有效 |
| Electron 使用的客户端库 | 使用 Yakit 锁定的 `@grpc/grpc-js 1.8.11`，20 次 Echo、1 MiB 消息、8 个并发请求成功；当时引擎 TCP LISTEN 数量为 0 |
| 竞态检查 | `go test -race ./common/utils/engineendpoint` 通过 |
| 其他平台构建 | endpoint 测试包的 Darwin arm64、Linux amd64 交叉编译通过，未在本机执行这些平台的测试 |

跨账号（使用其他管理员账号运行）、AppContainer/Low 完整性沙箱、第二块真实硬盘、Windows 10/Server/ARM64，未在本机完成实测。
“普通权限”指可读写自身数据目录的桌面进程，不表示能够绕过已有 NTFS 拒绝访问规则。
对于已有的、当前账号确实不可写的数据目录，应明确报告数据库权限错误；不自动重置 ACL、删除或更换用户数据库。

## Unix 更新后的 Windows 专项修复（2026-09-10）

在 `5c4ed4ebed5a351f0e27867fe7965b59e091758c` 上复测后，补充以下 Windows 修复。
不改变 macOS / Unix 的监听、锁文件、残留 socket 回收或输出行为：

- `PrepareListener` 只读识别被占用的管道；`grpc` 和服务器模式的
  `check-secret-local-grpc` 在数据库初始化前独占获取管道。真正的创建操作负责
  处理预检后的竞争，失败不会初始化数据库、连接或关闭已有服务。
  初始化中途失败时只释放本进程持有的 listener。
- Windows 的 `yak grpc ready` / `yak grpc failed` 使用启动时保存的原始 stdout，
  避免异步日志缓存尚未转发就退出而丢失控制事件。普通日志缓存保持原状，
  不通过延时等待补救，也不放宽管道 ACL 或密码校验。

两项问题在未修复的该提交及已发布 alpha 上均可复现，不是本次 Unix 更新引入。
新增回归在旧引擎失败、修复后通过：两个入口遇到管道占用均不创建三个数据库；
数据库初始化失败后管道名可复用；启用日志缓存后的 bind 失败恰好输出一次结构化事件。

修复后真实 Windows 引擎回归通过（约 37 秒），原有 CLI 启动验收通过（约 29 秒）。
另外完成提升权限测试：管理员服务端/普通客户端、反向连接，以及管理员创建隔离数据后
普通权限复用、管理员再次复用均通过（约 12 秒）；错误密码仍被拒绝。
跨盘符仍使用 SUBST，不代表第二块物理硬盘验证。

`prepare-yak` 后新增的 Windows / macOS / Linux 并行任务仅测试当前源码的 IPC 层，
不重新编译完整引擎、不加载数据库；参见 [快速 IPC 冒烟](unix-ipc-startup-compatibility.md#fast-three-platform-ipc-smoke-test)。
Windows 冒烟覆盖 16 个竞争启动者仅一个成功、双向大于管道缓冲区的数据、强杀服务端后
保留旧客户端句柄再启动和通信、普通令牌和私有 ACL。本机连续 20 轮通过，冷缓存含编译
约 34 秒。macOS / Linux 仅完成交叉编译，实际运行需对应 CI runner 验证。

### Windows CI 的 SID 别名回归

首轮 Windows Server 2022 CI 的通信测试通过，但 ACL 断言失败：系统将 runner 的
内置管理员 SID 格式化为 `LA`，旧测试错误地要求 SDDL 中包含完整数字 SID。
这属于测试断言的账号兼容性问题，不通过修改生产 ACL 或跳过权限测试解决。

测试现在使用 `GetAce` 和二进制 SID 比较，要求受保护的 DACL 恰有当前用户和
SYSTEM 的两个显式完整授权，拒绝其他账号、额外条目、拒绝型条目、继承和不完整权限。
新增 22 个正反例覆盖 `LA`/数字 SID 等价、条目顺序、SYSTEM 进程及不安全或不可用的 ACL，
同时纳入快速 IPC 冒烟；Medium 完整性标签的检查保持不变。
SID 别名定义参见 [Microsoft SDDL SID 说明](https://learn.microsoft.com/en-us/windows/win32/secauthz/sid-strings)。

## 本次修改

1. Windows 在独占创建同名管道时可能返回 `ERROR_ACCESS_DENIED (5)`。仅在失败后核实该名字确实存在，将其归为地址占用；核实过程不连接或替换原管道。
2. 区分 Windows 管道的 Win32 错误与 TCP 的 Winsock 错误，避免把真实权限错误归入未知错误。
3. 按 UTF-16 单元数校验 Windows 管道名称长度，允许合法的长中文名称和大小写等价的本地管道前缀。
4. 显式设置管道的 Medium 完整性标签。ACL 仍只授权当前用户 SID 和 SYSTEM，不要求管理员权限。
5. IPC 检查成功/失败时展示实际管道名；连接失败提示管道、进程和权限，不再给出无关的 TCP 端口/防火墙建议。

对照测试中，仅移除新增完整性标签的版本在这台 Windows 11 上也通过了同账号混合权限测试。
因此，显式标签属于明确兼容性策略，并不是“旧代码在所有 Windows 上都无法跨提升等级连接”的证据。
Windows 完整性控制独立于 DACL；无标签对象按 Medium 处理，显式标签让预期行为可验证。
参见 [Microsoft 完整性控制说明](https://learn.microsoft.com/en-us/windows/win32/secauthz/mandatory-integrity-control) 和
[命名管道访问控制说明](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights)。

测试使用 Windows 的 [受限令牌 API](https://learn.microsoft.com/en-us/windows/win32/secauthz/restricted-tokens)
从当前管理员进程派生普通权限子进程；不创建系统账号，不修改本机权限策略。

## 复现

从 Windows 仓库目录执行。完整引擎构建需要可用的 Go 和 C 编译器。
如果源码路径包含 `&`，Go 对 `${SRCDIR}` 展开的 C 编译参数校验可能拒绝该路径；可使用一个空闲盘符的 SUBST 映射构建。
这是源码构建路径限制，本次从包含 `&` 的可执行文件和数据路径启动引擎已验证成功。

```powershell
go build -o .\yak-npipe.exe common/yak/cmd/yak.go
go test -c -o .\endpoint-tests.exe ./common/utils/engineendpoint
$env:YAK_STARTUP_TEST_BINARY = (Resolve-Path .\yak-npipe.exe).Path
& .\endpoint-tests.exe '-test.v' '-test.timeout=5m'
```

普通终端会跳过需要管理员令牌的测试；在管理员终端运行相同测试二进制与环境变量，执行混合权限矩阵：

```powershell
& .\endpoint-tests.exe '-test.run=TestNamedPipeMixedElevation|TestWindowsEngineMixedElevation|TestNamedPipeACL' '-test.v' '-test.timeout=5m'
```

跨盘符测试需给出与测试二进制、引擎二进制不同盘符上的可写目录；可用真实数据盘进一步验收：

```powershell
$env:YAK_NPIPE_TEST_SECOND_ROOT = 'D:\npipe-test-output'
& .\endpoint-tests.exe '-test.run=TestNamedPipeDifferentDrive|TestWindowsEngineDifferentDrive' '-test.v' '-test.timeout=5m'
```

这些测试仅使用自己创建的临时数据库及目录，结束时清理子进程和临时文件。
原有 `common/yak/cmd` 接受测试仍由 `YAK_STARTUP_TEST_BINARY` 开关控制；运行该包时需额外设置隔离的 `YAKIT_HOME` 和三个数据库环境变量，避免包初始化访问用户数据。

## Yakit 接入约定

初轮验证时检出的 `yakit/master` 只传 TCP 端口，不能把引擎测试通过等同于完整 GUI 已经使用 npipe。
前端需要同时传递传输方式、端点和会话密码，并以 `yak grpc ready` 中的实际 `address` 为准。
旧 TCP 启动与预检 JSON 协议保持不变。

预检与启动示例（密码应采用预检返回的每次会话随机值）：

```powershell
& .\yak-npipe.exe check-secret-local-grpc --transport npipe --socket-path '\\.\pipe\yakit-session-unique-id'
& .\yak-npipe.exe grpc --transport npipe --socket-path '\\.\pipe\yakit-session-unique-id' --local-password $sessionSecret
```

`grpc-js 1.8.11` 使用其 `unix:` resolver 把原始 Windows 管道路径交给 Node 的 `net.connect`：

```javascript
const target = 'unix:' + ready.address
const client = new Yak(target, grpc.credentials.createInsecure())
const metadata = new grpc.Metadata()
metadata.set('authorization', 'bearer ' + sessionSecret)
```

显式 npipe 模式不同时传 `--host` / `--port` / TLS 参数。会话选择独立管道名；若名字已存在，应报告或选择新名字，不抢占旧引擎。
密码只应出现在必要的认证/机器协议中，不写入普通日志。
