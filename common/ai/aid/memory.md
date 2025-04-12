# Memory 模板渲染指南

本文档总结了 Memory 结构体中所有可以通过 `{{ .Memory... }}` 在模板中渲染的字段和方法。

## 基本信息

- `{{ .Memory.Query }}` - 用户最初的输入查询
- `{{ .Memory.MetaInfo }}` - 元信息映射表
- `{{ .Memory.OS }}` - 当前操作系统
- `{{ .Memory.Arch }}` - 当前系统架构
- `{{ .Memory.Now }}` - 当前时间（格式：2006-01-02 15:04:05）
- `{{ .Memory.Schema }}` - 任务 JSON Schema

## 任务相关

- `{{ .Memory.CurrentTask }}` - 当前任务对象
- `{{ .Memory.RootTask }}` - 根任务对象
- `{{ .Memory.Progress }}` - 任务进度信息
- `{{ .Memory.CurrentTaskInfo }}` - 当前任务的详细信息（通过模板渲染）

## 工具相关

- `{{ .Memory.ToolsList }}` - 可用工具列表（通过模板渲染）
- `{{ .Memory.Tools }}` - 工具列表函数

## 工具调用结果

- `{{ .Memory.PromptForToolCallResultsForLast5 }}` - 最近5次工具调用结果
- `{{ .Memory.PromptForToolCallResultsForLast10 }}` - 最近10次工具调用结果
- `{{ .Memory.PromptForToolCallResultsForLast20 }}` - 最近20次工具调用结果

## 交互历史

- `{{ .Memory.InteractiveHistory }}` - 交互历史记录（OrderedMap类型）

## 计划历史

- `{{ .Memory.PlanHistory }}` - 计划执行历史记录 