# yakit-sg-jdbc-bridge

Yakit 对国家电网 SG-JDBC（SGI-NDS 200 第二代信息安全网络隔离装置）的 Java 桥接代理。

由于 SG-JDBC 只提供 Java 驱动，Yakit/yaklang 主程序用 Go 编写，因此通过本代理将 JDBC 操作暴露为本地 JSON-over-TCP 协议，Go 端再调用即可。

## 目录

```
├── pom.xml                              Maven 构建配置
├── lib/
│   └── sg-jdbc-driver.jar              国网 SG-JDBC 驱动（手动放入）
├── src/main/java/com/yaklang/sgnds/
│   ├── bridge/BridgeServer.java         TCP 服务入口
│   ├── codec/ProtocolHandler.java       JSON 协议 ↔ JDBC 映射
│   └── pool/ConnectionPool.java         connId 连接句柄管理
└── src/test/java/...                    JUnit 测试
```

## 环境要求

- JDK 1.8（SG-JDBC 4.3 v2 明确要求）
- Maven 3.6+

## 构建

```bash
mvn clean package
```

产物位于 `target/yakit-sg-jdbc-bridge-1.0.0-SNAPSHOT.jar`，已包含 gson 依赖。

## 运行

```bash
java -jar target/yakit-sg-jdbc-bridge-1.0.0-SNAPSHOT.jar
# 默认监听 127.0.0.1:19090
```

自定义监听地址和线程数：

```bash
java -jar target/yakit-sg-jdbc-bridge-1.0.0-SNAPSHOT.jar 127.0.0.1 19090 32
```

## 文本协议

每行一个请求，响应同样以换行结束。格式为 `key=value` 列表，字段之间用 `&` 分隔。

### ping

```
action=ping
```

响应：

```
ok=true&pong=true
```

### open

```
action=open&url=jdbc:nds://...&user=u&password=p
```

响应：

```
ok=true&connId=xxx
```

### query

```
action=query&connId=xxx&sql=select+...&params=1,2
```

响应：

```
ok=true&columns=ID,NAME&rows=1|foo,2|bar
```

### exec

```
action=exec&connId=xxx&sql=update+...&params=a,b
```

响应：

```
ok=true&affectedRows=1
```

### pingConnection

```
action=pingConnection&connId=xxx
```

### close

```
action=close&connId=xxx
```

特殊字符使用百分号编码：`&`→`%26`, `=`→`%3D`, `|`→`%7C`, `,`→`%2C`。

## 与 Yakit 集成

Go 端实现 `database/sql/driver` 接口，通过本地 socket 与本代理通信，详见 yaklang 仓库 `common/brute/probes/sgnds/`。

## 注意事项

- SG-JDBC 驱动要求 JDK 1.8，使用其他版本可能报 `NoClassDefFoundError: java/sql/SQLType`。
- 连接字符串、虚拟数据库名、应用名需由网安室/现场提供，URL 示例：
  `jdbc:nds://170.20.8.223:18600,170.20.8.224:18600/v_18600_tjuvmp?appname=uvmp`
