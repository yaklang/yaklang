{{ .Background }}

<|USER_QUERY_{{ .Nonce }}|>
{{ .UserQuery }}
<|USER_QUERY_END_{{ .Nonce }}|>

{{/*----------------------------------------  额外能力（ExtraCapabilities - from intent recognition）------------------*/}}
{{ if .ExtraCapabilities }}<|EXTRA_CAPABILITIES_{{ .Nonce }}|>
{{ .ExtraCapabilities }}
<|EXTRA_CAPABILITIES_END_{{ .Nonce }}|>{{ end }}

{{/*----------------------------------------------------------------------------------------------------------------*/}}
{{ if .PersistentContext }}<|PERSISTENT_{{ .Nonce }}|>
{{ .PersistentContext }}
<|PERSISTENT_END_{{ .Nonce }}|>{{ end }}
{{/*----------------------------------------------------------------------------------------------------------------*/}}

{{/*----------------------------------------  Session Evidence（Config 级持久化观测）-----------------------------------*/}}
{{ if .SessionEvidence }}{{ .SessionEvidence }}{{ end }}

{{/*----------------------------------------  Session Reasoning（AI 思考内容缓存，5k）-----------------------------------*/}}
{{ if .SessionReasoning }}<|SESSION_REASONING_{{ .Nonce }}|>
# AI Reasoning Context
Recent AI reasoning content captured from previous model responses. Old content is trimmed when size exceeds 5k.
{{ .SessionReasoning }}
<|SESSION_REASONING_END_{{ .Nonce }}|>{{ end }}

{{/*----------------------------------------  Skills Context（按需加载的技能上下文）-------------------------------------*/}}
{{ if .SkillsContext }}{{ .SkillsContext }}{{ end }}

{{/*----------------------------------------  动态反应数据（ReactiveData Context）-------------------------------------*/}}
{{ if .ReactiveData }}<|REFLECTION_{{ .Nonce }}|>
{{ .ReactiveData }}
<|REFLECTION_END_{{ .Nonce }}|>{{ end }}

{{ if .InjectedMemory }}<|INJECTED_MEMORY_{{ .Nonce }}|>
# Memory Context
These are the memories automatically retrieved by the system that are most relevant to the current input.
{{ .InjectedMemory }}
<|INJECTED_MEMORY_END_{{ .Nonce }}|>{{ end }}

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
