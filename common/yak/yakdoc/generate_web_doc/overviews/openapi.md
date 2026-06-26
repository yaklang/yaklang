`openapi` 库用于处理 OpenAPI/Swagger 文档：在 JSON/YAML 间转换、从站点提取 OpenAPI 描述、并据此生成可发送的 HTTP 请求流，常用于 API 测试用例的自动生成与接口资产梳理。

典型使用场景：

- 格式转换：`openapi.ConvertJsonToYaml` / `openapi.ConvertYamlToJson` 互转文档格式。
- 提取与生成：`openapi.ExtractOpenAPI3Scheme(domain)` 从目标提取 OpenAPI 3 描述，`openapi.GenerateHTTPFlows(doc, opts...)` 据文档生成 HTTP 请求（配 `openapi.domain` / `openapi.https` / `openapi.flowHandler` 处理每条请求）。

与相邻库的关系：`openapi` 生成的请求常交给 `fuzz`/`poc` 做接口测试，是"从 API 文档到测试请求"的桥梁。
