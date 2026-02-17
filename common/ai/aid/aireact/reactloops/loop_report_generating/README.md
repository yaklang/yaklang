# loop_report_generating - 报告生成专注模式

## 概述

`loop_report_generating` 是一个专注于报告/文章生成的 ReAct 循环模式。它使 AI 能够一边查阅资料、一边撰写调查报告或分析文章，支持分批编写和修改。

## 工作模式

### 核心流程

```
┌─────────────────────────────────────────────────────────────┐
│                    报告生成工作流程                           │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐   │
│  │  调研准备   │ ──▶ │  撰写报告   │ ──▶ │  完善优化   │   │
│  └─────────────┘     └─────────────┘     └─────────────┘   │
│        │                   │                   │           │
│        ▼                   ▼                   ▼           │
│  • read_reference    • write_section     • modify_section   │
│  • grep_reference    • insert_section   • delete_section   │
│  • search_knowledge  • modify_section                      │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 第一阶段：调研准备

在写作前收集足够的参考资料：

| Action | 描述 | 使用场景 |
|--------|------|----------|
| `read_reference_file` | 读取参考文件内容 | 阅读文档、代码文件等 |
| `grep_reference` | 正则搜索参考资料 | 在大文件中搜索关键信息 |
| `search_knowledge` | 语义搜索知识库 | 从知识库获取相关知识 |

### 第二阶段：撰写报告

基于收集的资料创建和编辑报告：

| Action | 描述 | 使用场景 |
|--------|------|----------|
| `write_section` | 创建报告初稿 | 报告为空时使用 |
| `modify_section` | 修改指定行范围 | 更新/替换现有内容 |
| `insert_section` | 在指定行位置插入 | 添加新章节 |
| `delete_section` | 删除指定行范围 | 移除冗余内容 |
| `change_view_offset` | 切换视图偏移 | 导航大型报告 |

### 📖 大报告分页功能

当报告内容很大（超过约 30KB）时，系统会自动分页显示。使用 `change_view_offset` action 导航：

```json
{"@action": "change_view_offset", "offset_line": 51, "human_readable_thought": "查看第51行之后的内容"}
```

**参数说明：**
- `offset_line`: 从第几行开始展示（1-based，必填）
- `show_size`: 最大展示字符数（默认 30000，可选）

**导航操作：**
- 回到开头：`offset_line=1`
- 向下翻页：`offset_line = 当前结束行号 + 1`
- 跳转到指定行：`offset_line = 目标行号`

### 第三阶段：完善优化

检查和完善报告内容，直到满足质量要求后调用 `finish`。

## AITAG 输出规则

**⚠️ 关键规则：写作类 Action 必须使用 AITAG！**

当使用 `write_section`、`modify_section`、`insert_section` 时，必须在 JSON 后输出 AITAG 包裹的内容：

```
{"@action": "write_section", "human_readable_thought": "创建报告初稿"}

<|GEN_REPORT_xxx|>
# 报告标题

## 第一章
报告内容...
<|GEN_REPORT_END_xxx|>
```

**注意：**
- `xxx` 是系统提供的 Nonce
- 开始标签：`<|GEN_REPORT_xxx|>`
- 结束标签：`<|GEN_REPORT_END_xxx|>`
- 只输出 JSON 不输出标签，报告内容会为空！

## 文件结构

```
loop_report_generating/
├── README.md                    # 本文档
├── code.go                      # 模式注册和初始化
├── action_read_reference.go     # 读取参考文件
├── action_grep_reference.go     # 搜索参考资料
├── action_search_knowledge.go   # 知识库搜索
├── action_change_offset.go      # 切换视图偏移（分页导航）
├── init_task.go                 # 初始化任务（意图分析、文件准备）
│   # write_section, modify_section, insert_section, delete_section
│   # 由 loopinfra.SingleFileModificationSuiteFactory 统一提供
├── prompts/
│   ├── persistent_instruction.txt   # AI 角色定义
│   ├── reactive_data.txt            # 响应数据模板
│   └── reflection_output_example.txt # 输出示例
└── examples/
    ├── test_basic_report.yak          # 基础报告生成测试
    ├── test_grep_reference.yak        # grep 搜索测试
    ├── test_multi_file_analysis.yak   # 多文件分析测试
    ├── test_iterative_writing.yak     # 迭代写作测试
    ├── test_code_analysis_report.yak  # 代码分析报告测试
    ├── test_change_view_offset.yak    # 分页导航测试
    └── test_modify_existing_file.yak  # 修改现有文件测试
```

## 使用示例

### 基础用法

```yak
err = aim.InvokeReAct(
    "请基于 README.md 生成项目介绍报告",
    aim.focus("report_generating"),
    aim.timeout(300),
    aim.maxIteration(15),
)
```

### 带回调的完整用法

```yak
var reportFilename = ""

err = aim.InvokeReAct(
    "请分析项目文档并生成技术报告",
    aim.focus("report_generating"),
    aim.timeout(300),
    aim.maxIteration(15),
    aim.onEvent(func(react, event) {
        // 注意：event.Type 需要转换为字符串进行比较
        eventType = string(event.Type)
        if eventType == "filesystem_pin_filename" {
            // event.Content 是 []byte 类型
            contentStr = string(event.Content)
            try {
                jsonData = json.loads(contentStr)
                if jsonData != nil && jsonData["path"] != nil {
                    reportFilename = jsonData["path"]
                    log.Info("Report file: %s", reportFilename)
                }
            } catch e {
                log.Warn("Parse event content failed: %v", e)
            }
        }
    }),
    aim.onFinished(func(react) {
        if reportFilename != "" {
            content, err = file.ReadFile(reportFilename)
            if err == nil {
                println("Report:\n", string(content))
            }
        }
    }),
)
```

## 测试脚本说明

### test_basic_report.yak
测试基础报告生成功能，只使用 `write_section` action。

### test_grep_reference.yak
测试 `grep_reference` 功能，先搜索再生成报告。

### test_multi_file_analysis.yak
测试多文件分析功能，阅读多个参考文件后生成综合报告。

### test_iterative_writing.yak
测试迭代写作流程，使用 `write_section` → `insert_section` → `modify_section` 的完整流程。

### test_code_analysis_report.yak
测试代码分析能力，分析 Go 代码文件并生成技术报告。

### test_change_view_offset.yak
测试大报告分页导航功能。生成一个较长的多章节报告，然后使用 `change_view_offset` 导航到不同位置，验证分页功能是否正常工作。

**注意：** 由于测试生成的报告通常不会超过 30KB，AI 可能会判断无需使用分页功能。这是合理的行为——分页功能主要为处理超大报告设计。

### test_modify_existing_file.yak
测试修改现有文件功能。这是一个重要的集成测试，验证 AI 能够：
1. 识别用户意图是"修改现有文件"而非"创建新文件"
2. 正确读取现有文件内容
3. 使用 `modify_section` 精确修改指定章节
4. 保留其他未修改的内容

**测试流程：**
1. 先创建一个预先存在的 Markdown 报告文件（包含占位符内容）
2. 让 AI 使用 `report_generating` 模式修改指定章节
3. 验证修改成功且保留了原有结构

## 运行测试

```bash
# 运行基础测试
cd /Users/v1ll4n/Projects/yaklang
go run common/yak/cmd/yak.go common/ai/aid/aireact/reactloops/loop_report_generating/examples/test_basic_report.yak

# 运行所有测试
for f in common/ai/aid/aireact/reactloops/loop_report_generating/examples/*.yak; do
    echo "Running: $f"
    go run common/yak/cmd/yak.go "$f"
done
```

## 测试效果展示

### 测试结果汇总（2026-01-03）

| 测试脚本 | 状态 | 使用的 Actions | 耗时 |
|---------|------|---------------|------|
| test_basic_report.yak | ✅ PASSED | `write_section` | ~15s |
| test_grep_reference.yak | ✅ PASSED | `grep_reference`, `write_section` | ~20s |
| test_multi_file_analysis.yak | ✅ PASSED | `read_reference_file`(多次), `write_section` | ~25s |
| test_iterative_writing.yak | ✅ PASSED | `write_section`, `insert_section` | ~30s |
| test_code_analysis_report.yak | ✅ PASSED | `read_reference_file`, `grep_reference`, `write_section` | ~20s |
| test_change_view_offset.yak | ✅ PASSED | `write_section`, `insert_section` | ~60s |
| test_modify_existing_file.yak | ✅ PASSED | `modify_section`(多次) | ~30s |

### 生成报告示例

#### test_basic_report.yak - 基础报告生成

**输入提示：**
```
请帮我基于 /Users/v1ll4n/Projects/yaklang/README.md 文件生成一份简要的 Yaklang 项目介绍报告。
报告要求：
1. 简要介绍项目的用途和定位
2. 列出主要功能特点
3. 总结项目的技术架构
报告长度控制在 200 字左右即可。
```

**生成报告：**
```markdown
# Yaklang项目介绍报告

## 用途和定位
Yaklang是面向网络安全领域的专用编程语言（CDSL），旨在解决安全场景中自动化任务开发效率低、
工具链碎片化问题，提供统一编程范式，支持从协议分析到攻击模拟的全流程开发。

## 主要功能特点
1. 深度融合100+网络协议解析器和协议状态机抽象
2. 原生集成漏洞检测框架、密码学算法库及Fuzztag模糊测试引擎
3. 支持编译为原生可执行文件，执行效率较传统解释型语言提升40%以上
4. 提供Yakit配套IDE，支持可视化开发和第三方工具链集成

## 技术架构
采用"语言+虚拟机+工具链"三层架构：核心层为YakVM虚拟机和SSA静态分析框架；
能力层包含协议解析、漏洞检测等安全能力库；应用层通过Yakit IDE和CLI提供开发界面，
形成完整的"语言-工具"开发闭环。
```

#### test_grep_reference.yak - SSA 功能分析

**输入提示：**
```
请帮我分析 Yaklang 项目中的 SSA（静态单赋值）相关功能。
使用 grep_reference 搜索 "SSA" 或 "静态分析" 相关内容，
基于搜索结果撰写一份简要的 SSA 功能分析报告。
```

**生成报告：**
```markdown
# Yaklang SSA功能分析报告

## SSA的作用
YAK SSA（静态单赋值形式）是Yaklang核心组件之一，作为专为静态分析优化的中间表示形式，
为代码审计和静态分析提供基础支撑，是实现程序安全分析的关键技术层。

## 应用场景
SSA与SyntaxFlow共同构成代码审计和静态分析的基础层，支撑IRify等Yaklang审计平台
实现多语言分析与报告生成，在漏洞检测、代码安全审计等场景中发挥重要作用。

## 技术亮点
SSA作为Yaklang语言核心技术架构的关键组成部分，通过与YakVM虚拟机、SyntaxFlow
语法模式匹配等技术深度协同，构建了面向网络安全领域的专用静态分析能力。
```

#### test_iterative_writing.yak - 迭代写作（技术白皮书）

**输入提示：**
```
请帮我撰写一份 Yaklang 项目的技术白皮书（简化版）。
分步骤执行：
1. 使用 write_section 创建初始结构
2. 使用 insert_section 添加新章节
3. 使用 modify_section 完善内容
```

**生成报告：**
```markdown
# Yaklang 技术白皮书（简化版）

## 第一章：项目背景
当前网络安全领域面临自动化任务开发效率低、工具链碎片化等挑战。Yaklang作为面向网络安全领域
的专用编程语言（CDSL），于2023年完全开源，由电子科技大学网络空间安全学院学术指导，
Yaklang.io研发团队历经多年迭代而成。其设计目标是通过统一编程范式，解决安全场景中的技术痛点，
支持从协议分析到攻击模拟的全流程开发。

## 第二章：核心技术
Yaklang的核心技术包括：CDSL Yaklang专用编程语言，融合100+网络协议解析器和协议状态机抽象；
YakVM虚拟机和SSA静态分析框架，为代码审计提供基础支撑；SyntaxFlow语法模式匹配和漏洞签名
建模DSL；LSP/DSP语言服务协议支持。技术架构采用"语言+虚拟机+工具链"三层设计。

## 第三章：应用场景
Yaklang主要应用于安全自动化与工具开发、漏洞检测与渗透测试、网络流量分析与监控、
安全教学与研究等场景。安全从业人员可利用其开发漏洞扫描器、编写渗透测试脚本；
企业安全团队可构建自动化安全检测与响应流程。
```

## 质量标准

生成的报告应满足：

1. **准确性** - 基于参考资料，不虚构信息
2. **完整性** - 覆盖用户要求的所有方面
3. **可读性** - 语言流畅，结构清晰
4. **专业性** - 使用专业术语，逻辑严谨

## 输出格式

- 使用 Markdown 格式
- 包含清晰的标题层级（#, ##, ###）
- 适当使用列表、表格等元素
- 引用来源时标注出处

