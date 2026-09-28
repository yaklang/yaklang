# SG-JDBC（SGI-NDS 200）兼容适配

本目录提供 yaklang 对国家电网第二代信息安全网络隔离装置 SGI-NDS 200 的 SG-JDBC 驱动兼容支持。

由于 SG-JDBC 只提供 Java 驱动，yaklang 通过本地 Java 代理进程桥接 JDBC 操作。

## 目录结构

```
common/brute/probes/sgnds/       # Go 端 probe 实现
common/third_party/sgnds-bridge/ # Java 代理源码、构建脚本、驱动 jar
```

## 构建 Java 代理

```bash
cd common/third_party/sgnds-bridge
mvn clean package
# 或手动编译：
javac -encoding UTF-8 -d target/classes -source 1.8 -target 1.8 -cp lib/sg-jdbc-driver.jar $(find src/main/java -name '*.java')
```

产物：`target/sgnds-bridge.jar`

## 运行 Java 代理

```bash
cd common/third_party/sgnds-bridge
java -cp target/sgnds-bridge.jar;lib/sg-jdbc-driver.jar com.yaklang.sgnds.bridge.BridgeServer
```

默认监听 `127.0.0.1:19090`。

## Go 端使用

在 yaklang 中调用注册：

```go
import "github.com/yaklang/yaklang/common/brute/probes/sgnds"

sgnds.Register()
```

## 协议

Go 端与 Java 代理之间使用简单文本协议（每行一个请求，key=value 用 `&` 分隔）：

- `action=ping`
- `action=open&url=...&user=...&password=...`
- `action=query&connId=...&sql=...`
- `action=exec&connId=...&sql=...`
- `action=close&connId=...`

详见 `common/third_party/sgnds-bridge/README.md`。

## 测试

### 本地 mock 测试

```bash
go test ./common/brute/probes/sgnds/ -v -timeout 30s
```

### 真实 Java 代理集成测试

```bash
cd common/third_party/sgnds-bridge
# 先构建 jar
javac -encoding UTF-8 -d target/classes -source 1.8 -target 1.8 -cp lib/sg-jdbc-driver.jar $(find src/main/java -name '*.java')
printf 'Manifest-Version: 1.0\nMain-Class: com.yaklang.sgnds.bridge.BridgeServer\nClass-Path: lib/sg-jdbc-driver.jar\n' > target/MANIFEST.MF
jar cfm target/sgnds-bridge.jar target/MANIFEST.MF -C target/classes .

cd ../../..
go test ./common/brute/probes/sgnds/ -tags integration -v -run TestWithRealBridge -timeout 60s
```

## 注意事项

- SG-JDBC 4.3 v2 驱动文档要求 JDK 1.8，但 JDK 17 下也能加载。建议现场仍使用 JDK 1.8。
- 驱动 jar 不含 `META-INF/services/java.sql.Driver`，需要手动注册，代理已处理。
- 目标不可达时驱动会抛 `NullPointerException`（内部 bug），代理已包装为友好错误。
