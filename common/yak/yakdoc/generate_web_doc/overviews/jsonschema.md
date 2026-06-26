`jsonschema` 库用于以声明式方式构建 JSON Schema 字符串，定义对象、字段类型、约束与枚举，主要服务于 AI 工具/函数调用的参数与输出结构定义。

典型使用场景：

- 构建对象：`jsonschema.Object` / `jsonschema.NewObjectSchema` / `jsonschema.ObjectArray` 定义对象/数组 Schema，`jsonschema.ActionObject` 定义带 action 的对象。
- 声明字段：`jsonschema.paramString` / `jsonschema.paramInt` / `jsonschema.paramBool` / `jsonschema.paramNumber` / `jsonschema.paramObject` / `jsonschema.paramStringArray` 等声明各类字段。
- 约束：`jsonschema.description` / `jsonschema.required` / `jsonschema.enum` / `jsonschema.min` / `jsonschema.max` / `jsonschema.minLength` / `jsonschema.example` 等修饰字段。

与相邻库的关系：`jsonschema` 为 `ai`（FunctionCall）、`aiagent`/`liteforge`（工具与结构化输出）提供参数/输出 Schema，是 AI 工具化的基础。
