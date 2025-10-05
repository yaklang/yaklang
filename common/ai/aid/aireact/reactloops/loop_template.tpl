{{ .Background }}

# User Query
<|USER_QUERY_NONCE_{{ .Nonce }}|>
{{ .UserQuery }}
<|USER_QUERY_NONCE_{{ .Nonce }}|>

响应格式输出JSON，请遵守如下Schema ：

```schema
{{ .Schema }}
```