# SCA 算法基准

## 环境与口径

本机 macOS / darwin-arm64，Go 1.22.12，GOMAXPROCS 默认 10。旧闭包必须启用 CGO（禁用时 PCRE2 编译失败）；新闭包禁用 CGO。这是旧新实际运行闭包的比较，不是相同动态库加载条件的纯函数比较。输入规模为 1,000 / 10,000 / 50,000，均是同名、不同精确版本的记录；旧新均断言保留 N 条结果。没有范围求解、网络、文件 IO 或依赖边，因此仅比较精确身份归并内核。

`BenchmarkExactIdentity` 包含输入 Package 构造开销，旧新使用同一测试函数。旧 SCA 从基线归档恢复到模块外隔离目录，其他 internal 包只用于让旧包编译；不把这个微基准说成完整旧扫描器验收。先编译测试二进制，再以 `/usr/bin/time -l` 计量执行阶段 RSS/CPU。当前单次进程重复 3 次操作，旧进程 1 次；更高重复负载仍未超过旧 RSS。

| N | 旧 ns/op | 新 ns/op 中位数 | 旧 B/op | 新 B/op 中位数 | 旧 RSS bytes | 新 RSS bytes |
|---:|---:|---:|---:|---:|---:|---:|
| 1000 | 32663666 | 653166 | 289264 | 476904 | 22478848 | 7634944 |
| 10000 | 3426150416 | 7462750 | 3033888 | 4443888 | 24821760 | 12042240 |
| 50000 | 41532943209 | 43191792 | 16338384 | 21799592 | 38830080 | 29966336 |

当前 50,000 条输入的时间和峰值 RSS 通过本微基准的 1.10 倍防回退检查。分配字节数高于旧实现，需要单独看待；峰值 RSS 和累计分配不是同一指标。第一次实现额外证据直接放在 Package 中曾超过 RSS 门槛，随后改为可选 PackageDetails，且用长度帧摘要代替反射 JSON 身份中间对象，降低了固定结构与临时分配。失败与改进过程原始日志保留在模块外证据目录。

## 规模与操作成本

旧同名组需按双层循环尝试配对，最多 N(N-1)/2 对比较；这是源码操作数，不冒充 profiler 实测。新实现每条输入一次身份摘要和 map lookup，随后稳定排序；显式边额外两次索引查找。哈希、字符串长度、边输出量仍计算在成本内。

`BenchmarkNormalize`、`BenchmarkPathIndex`、`BenchmarkParse` 另覆盖 1k/10k/50k 身份与 npm 路径规模，以及 go.mod 100/1k/10k/100k 指令规模。日志 `audit/benchmarks-final.txt` 保留时间、B/op、allocs/op；这些不是与旧实现语义等同的整扫描对比，不据此声称所有格式提速。

## 补充规模与稀疏发现

相同归并输入扩展到 100 与 100,000 条。100,000 条旧实现一次为 165,584,660,458 ns/op，新实现三次中位数为 142,131,625 ns/op；旧/新进程峰值 RSS 为 56,328,192 / 55,296,000 bytes。该进程同时运行 100 条子用例，RSS 是整个进程的峰值，不能单独分配给 100 条场景。原始记录为 `audit/rss-{baseline,current}-extra.log`。

`BenchmarkSparseDiscovery` 使用相同的只读 fs.FS → FileSystem 测试适配器，给旧新扫描器提供 100 / 1,000 / 10,000 / 100,000 个非候选文本文件和一个 go.mod。断言两边都输出唯一 `example.test/a 1.2.3`。计数包装器记录成功打开输入文件的次数及实际 Read/ReadAt 返回字节；Stat/ReadDir 元数据操作不计入 opens，不把它说成宿主 syscall 跟踪。

100,000 个非候选文件场景均重复 3 次：

| 指标 | 旧 | 新 |
|---|---:|---:|
| ns/op 中位数 | 208832417 | 154096541 |
| B/op 中位数 | 478795680 | 63308848 |
| 进程峰值 RSS bytes | 101171200 | 62472192 |
| 输入 opens/op | 100001 | 100002 |
| 输入 readbytes/op | 2200054 | 400058 |

旧发现流程的缓冲读取会读入完整短文件；新流程只取得候选判断所需的头部，再读取命中文件。新流程多一次候选打开，实际读取量和分配量降低。目录场景的时间和峰值 RSS 通过本场景 1.10 倍门槛。原始单次阶梯和重复 RSS 日志均在 `audit/discovery-*.log`。

当前复现命令：

```sh
GOWORK=off CGO_ENABLED=0 go test ./common/sca/analyzer -run '^$' -bench '^BenchmarkExactIdentity$' -benchtime=1x -count=3
GOWORK=off CGO_ENABLED=0 go test ./common/sca -run '^$' -bench '^BenchmarkSparseDiscovery$' -benchtime=1x -count=3
GOWORK=off CGO_ENABLED=0 go test -c -o /tmp/sca-scan.test ./common/sca
/usr/bin/time -l /tmp/sca-scan.test -test.run '^$' -test.bench '^BenchmarkSparseDiscovery/100000$' -test.benchtime=1x -test.count=3
```

旧基线在模块外从固定 Git 归档恢复，只加相同 benchmark 与只读输入适配器；使用其原 go.mod/go.sum 和 CGO_ENABLED=1。当前工作树没有旧运行实现或运行 fallback。

## 尚不能推出的结论

没有为 20 个格式逐一完成相同语义的端到端 wall/CPU/RSS/IO/临时文件前后矩阵。普通文本与内存材料不落盘由源码闭包、内存文件系统回归和禁止写入的隔离执行共同验证，尚未形成逐格式旧新临时文件计数表。不能把本微基准的通过外推为任务书全部性能门槛已通过。
