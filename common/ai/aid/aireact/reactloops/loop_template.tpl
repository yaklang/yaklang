{{ .Background }}

<|USER_QUERY_{{ .Nonce }}|>
{{ .UserQuery }}
<|USER_QUERY_END_{{ .Nonce }}|>

{{/*----------------------------------------------------------------------------------------------------------------*/}}
{{ if .PersistentContext }}<|PERSISTENT_{{ .Nonce }}|>
{{ .PersistentContext }}
<|PERSISTENT_END_{{ .Nonce }}|>{{ end }}
{{/*----------------------------------------------------------------------------------------------------------------*/}}

{{/*----------------------------------------  动态反应数据（ReactiveData Context）-------------------------------------*/}}
{{ if .ReactiveData }}<|REFLECTION_{{ .Nonce }}|>
{{ .ReactiveData }}
<|REFLECTION_END_{{ .Nonce }}|>{{ end }}

{{/*----------------------------------------------------------------------------------------------------------------*/}}
响应格式输出JSON和<|TAG...{{.Nonce}}|>，请遵守如下Schema ：

<|SCHEMA_{{.Nonce}}|>
```jsonschema
{{ .Schema }}
```
<|SCHEMA_{{.Nonce}}|>

{{ if .OutputExample }}<|OUTPUT_EXAMPLE_{{.Nonce}}|>
{{ .OutputExample }}
<|OUTPUT_EXAMPLE_END_{{.Nonce}}|>{{ end }}