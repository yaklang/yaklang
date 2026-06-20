`mmdb` 库用于读取 MaxMind 的 MMDB 数据库（如 GeoLite2），按 IP 查询地理位置信息，常用于 IP 归属地标注与资产地理画像。

典型使用场景：

- 打开与查询：`mmdb.Open(file)` 打开 MMDB 文件得到 reader，`mmdb.QueryIPCity(reader, ip)` 按 IP 查询城市/地理信息。

与相邻库的关系：`mmdb` 是离线地理库读取工具，与 `db.QueryIPCity`（内置 GeoIP）、`amap`（在线地理服务）互补，用于 IP 地理信息富化。
