---
name: fuzztag
metadata:
  display_name_zh-CN: FuzzTag 文本生成
description: 使用 Yaklang/Yakit FuzzTag 模板批量生成文本、测试数据、URL 和编码变体；用于编写、渲染或调试 FuzzTag。
---

# FuzzTag 文本生成

FuzzTag 是 Yaklang 的多结果模板引擎：固定文本保留，标签生成或加工字符串。适合组合文案、编号、字典和测试样本；先写内容，再展开。

## 语法与组合

使用 `{{标签(参数)}}`，无参可写 `{{uuid}}`。参数是文本，各标签自行解析分隔符。

- `{{list(红|蓝)}}`（别名 array）枚举；`{{int(1-3)}}` 闭区间；`{{int(1-3|3)}}` 补零为 001–003。
- `{{randstr(8,8,3)}}` 生成三个八位随机串；`{{regen([ab]{2})}}` 枚举 aa/ab/ba/bb。
- `{{repeatstr(ha|3)}}` 得到 hahaha；`upper/lower/trim` 处理大小写与空白。
- `{{base64({{list(hello|world)}})}}` 内层先展开，外层逐项编码。支持 hex、md5、sha256、urlenc；urlenc 会编码全部字符，urlescape 只编码特殊字符。

并列标签默认取笛卡尔积。`{{list(A|B)}}-{{int(1-2)}}` 产生四种组合。同层标签用相同 `::row` 同步配对：`{{list::row(A|B)}}={{int::row(1-2)}}` 得到 A=1、B=2；配对列表保持等长。字面标签用 raw 包裹：`{{={{int(1-2)}}=}}` 输出原文。

## 执行与交付

优先调用内置 AI 工具 `exec_fuzztag`，传 `template`；默认将结果直接输出到 stdout。传 `output-file` 保存文件，`stdout:true` 可同时输出，`force:true` 允许覆盖。`format` 为 lines（每项后加换行）或 json（字符串数组，保留多行边界）。例如：

```json
{"template":"通知：{{list(小王|小李)}}，任务{{int(1-3|3)}}已完成。","output-file":"/tmp/notices.txt","limit":20}
```

值传 `params`，用 `{{params(name)}}`。多行用 AITAG 的 `TOOL_PARAM_template` 写真实换行；字面 `\n` 不会解码，JSON 参数只转义一次。CLI：`yak exec_fuzztag.yak --template 'item{{int(1-3)}}'`。

工具保留重复结果；`limit` 默认 1000，范围 1–100000，在引擎生成时限制。检查 RESULT 的 success、count、truncated、output_file、bytes 和 preview；截断时 count 只是已输出数。交付前验证样本；`fuzz.Strings` 会去重。

文件词表需 `enable-file-tags:true`；payload/codecflow 依赖本地数据库，codec/yak 需插件或热加载入口，本工具未启用。未知标签可能变空串，语法错误可能保留原文，仍须检查样本和 JSON/CSV 转义。

## 查看全集

需要更多标签、别名和参数时，用 `loading_skills` 加载 `skill_name:"fuzztag-reference"`。该技能涵盖当前仓库标签全集与运行条件；组合后仍用 exec_fuzztag 验证。以本机源码和渲染为准。
