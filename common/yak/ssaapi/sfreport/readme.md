# CodeScan 
通过命令: `yak code-scan`运行。 

参数：

| 参数名 | 参数分组 | 作用 |
| --- | --- | --- |
| -t, --target  | 扫描目标 | 设置需要分析的目标路径， 支持：本地目录、本地zip包、 |
| -p, --program | 扫描目标 | 设置已经编译过的项目名，可以在IRify客户端中 项目管理功能内看到 |
| -l, --language  | 扫描目标 | 设置扫描目标的语言，设置路径或项目名将会自动确定语言，但可以选择手动强制设置。 |
| -kw, -rw, --rule-keyword  | 扫描规则 | 设置需要扫描的规则，只需要提供关键字将会进行检索，默认将会使用项目的语言对应的所有规则进行扫描。 |
| --format | 扫描报告 | 设置扫描报告的格式， 可以设置为：`irify`, `irify-full`, `sarif` |
| -o, --output  | 扫描报告 | 设置扫描结果保存的位置，如果没有设置将输出到stdout中，输出格式目前默认为**sarif格式**。 |


## 使用样例
比如对于一个项目，可以直接使用: 

```plain
yak code-scan -t ${项目路径} [可选: -l ${编程语言} --rule-keyword ${规则关键字} --format ${报告格式}] -o ${报告文件位置}
```

程序执行流程如下：

1. 自动解析` -t ${项目路径}`内的代码
    1. 如果没有设置 `-l ${编程语言}`将会根据项目内的文件自动确定语言并进行解析。
2. 使用规则进行扫描，
    1. 如果设置了 `--rule-keyword ${规则关键字}`则进行规则筛选并扫描
    2. 如果没有设置则直接执行项目同语言的所有规则
3. 结果生成报告保存到` -o ${报告文件位置}`中。
    1. 如果设置 ` --format ${报告格式}` 则会按照格式生成，默认为`sarif`格式，可以设置为`irify``irify-full`

# IRify格式报告 
IRify格式有两种类型：

+ `irify` 该格式保存数据，但对于文件内容只保留前100字符。需要搭配相关代码仓库进行数据渲染。
+ `irify-full`该格式保存完整的文件内容信息，报告本身即可进行完整的数据渲染。

## 一、JsonSchema格式定义
见文件：`irify_report_schema.json`

## 二、整体架构设计
```plain
{
  "report_type": "irify/irify-full",          // 支持Irify/Irify-full类型
  "engine_version": "x.y.z",                  // 引擎版本
  "report_time": "ISO8601时间戳",           
  "program_name": "被分析程序标识",  
  "Rules": [],                                // 当前扫描相关规则
  "Risks": {},                                // 风险指纹字典（哈希索引）
  "File": []                                  // 当前扫描相关文件
}
```

## 三、核心数据结构说明
### 1. 风险本体模型 (Risk)
```plain
{
  "id": 1001,                                 // 全局唯一递增ID
  "hash": "9a8b7c...",                        // 全局唯一Hash 
  "severity": "high",                         // 漏洞与风险等级 (info/low/middle/critical/high)
  "title": "Check Java Path Traversal Vulnerability",                    
  "title_verbose": "检测Java路径穿越漏洞",
  "description": "漏洞描述信息",
  "risk_type": "path-traversal",              // 漏洞与风险分类
  "cve": "CVE-xx-xx",                         // 关联漏洞库编号
  "code_range": "{\"url\":\"...\",\"start_line\":24,\"start_column\":62,\"end_line\":24,\"end_column\":77}", // 精确代码范围定位
  "code_source_url": "FileUploader.java",     // 源码路径
  "solution": "修复建议",                      // 上下文感知修复方案
  "details": "Java代码中发现路径穿越漏洞，并且数据流中间没有进行任何过滤。", // 动态分析上下文
  "time": "2025-08-08T11:17:50.3410823+08:00", // 精确到毫秒的时间戳
  "language": "java",                         // 编程语言
  "line": 24,                                 // 风险所在行号
  "rule_name": "检查Java路径穿越漏洞",           // 触发风险的规则名称
  "program_name": "7a8f7821-0d17-4501-9f10-5b141c0e74a6", // 项目名称
  "latest_disposal_status": "not_set",        // 最新处置状态
  "data_flow_paths": [                        // 数据流路径信息（新增）
    {
      "path_id": "path_1323",
      "description": "Data flow path for path-traversal vulnerability",
      "nodes": [...],                         // 数据流节点
      "edges": [...],                         // 数据流边
      "dot_graph": "strict digraph {...}"     // DOT格式图描述
    }
  ]
}
```

**关键字段**

+ `description`,`details`：风险信息详情
+ `severity`：风险评估优先级的核心依据，支持 `info`/`low`/`middle`/`critical`/`high` 五个等级
+ `code_range`：精准定位风险代码位置，包含完整的代码范围信息
+ `data_flow_paths`：数据流分析路径，展示从源到汇的完整数据流
+ `language`：编程语言标识
+ `rule_name`：触发风险的规则名称
+ `latest_disposal_status`：风险处置状态

### 2. 数据流路径模型 (DataFlowPath)
```plain
{
  "path_id": "path_1323",                     // 路径唯一标识
  "description": "Data flow path for path-traversal vulnerability", // 路径描述
  "nodes": [                                  // 数据流节点列表
    {
      "node_id": "n1",                        // 节点唯一标识
      "ir_code": "Parameter-fileName",        // 中间表示代码
      "source_code": "String fileName",       // 源代码片段
      "source_code_start": 0,                 // 源代码起始位置
      "code_range": {                         // 代码范围信息
        "url": "/FileUploader.java",
        "start_line": 24,
        "start_column": 62,
        "end_line": 24,
        "end_column": 77,
        "source_code_line": 20
      }
    }
  ],
  "edges": [                                  // 数据流边列表
    {
      "edge_id": "e0",                        // 边唯一标识
      "from_node_id": "n1",                   // 起始节点
      "to_node_id": "n2",                     // 目标节点
      "edge_type": "depend_on",               // 边类型：depend_on/call/search-exact等
      "description": "The dependency edge in the dataflow" // 边描述
    }
  ],
  "dot_graph": "strict digraph {...}"         // DOT格式图描述，用于可视化
}
```

**关键字段**

+ `nodes`：数据流中的节点，包含源代码位置和中间表示
+ `edges`：节点间的连接关系，描述数据如何流动
+ `dot_graph`：DOT格式的图描述，可用于生成可视化图表

### 3. 节点信息模型 (NodeInfo)
```plain
{
  "node_id": "n1",                           // 节点唯一标识
  "ir_code": "Parameter-fileName",           // 中间表示代码
  "source_code": "String fileName",          // 源代码片段
  "source_code_start": 0,                    // 源代码起始位置
  "code_range": {                            // 代码范围信息
    "url": "/FileUploader.java",
    "start_line": 24,
    "start_column": 62,
    "end_line": 24,
    "end_column": 77,
    "source_code_line": 20
  }
}
```

### 4. 边信息模型 (EdgeInfo)
```plain
{
  "edge_id": "e0",                           // 边唯一标识
  "from_node_id": "n1",                      // 起始节点ID
  "to_node_id": "n2",                        // 目标节点ID
  "edge_type": "depend_on",                  // 边类型
  "description": "The dependency edge in the dataflow" // 边描述
}
```

## 四、核心数据关联深度解析
### 1. 文件关联 (File)
```plain
{
  "path": "/src/main.py",                    // 规范化路径格式
  "length": 4096,                            // 带字节单位校验
  "hash": {                                  // 防篡改校验集
    "md5": "a1b2c3...", 
    "sha256": "d4e5f6...",
    "blake3": "e7f8g9..."
  },
  "content": "print('test')",                // irify-full模式包含完整内容
  "risks": ["risk_hash_1", ...]                   // 风险索引
}
```

**关键字段**

+ `path`：漏洞所在的代码文件路径，用于快速定位
+ `hash`：多算法文件指纹，用于完整性校验和唯一性比对
+ `content`：文件内容，当报告格式为`irify-full`的时候将会填写完整内容
+ `risks`：该文件触发的所有风险哈希，建立文件与风险的关联

### 2. 规则关联 (Rule)
```plain
{
  "rule_name": "检查Java路径穿越漏洞",         // 规则名
  "language": "java",                        // 该规则所支持的语言
  "description": "检测Java路径穿越漏洞",        
  "solution": "修复建议",                     // 开发修复指南
  "content": "规则实现代码",                  // 合规条款映射
  "risks": ["risk_hash_1", "risk_hash_2"],  // 风险索引
}
```

**关键字段**

+ `rule_name`：规则名 用于定位规则
+ `solution`：标准修复方案，可直接传递给开发团队
+ `content`：规则的具体实现（正则模式或静态分析逻辑）
+ `risks`：该规则触发的所有风险哈希，建立文件与风险的关联

### 3. 风险关联矩阵
| 关系类型 | 实现方式 | 安全价值 |
| :--- | :--- | :--- |
| 规则→风险 | Rule.risks[] 风险哈希数组 | 风险类型聚合分析 |
| 文件→风险 | File.risks[] 风险指纹列表 | 影响范围分析 |
| 风险←→代码位置 | Risk.code_range | 精准修复定位 |
| 风险←→数据流 | Risk.data_flow_paths | 数据流分析追踪 |

## 五、报告样例
```plain
{
  "report_type": "irify",
  "engine_version": "dev",
  "report_time": "2025-08-08T11:17:50.344591+08:00",
  "program_name": "7a8f7821-0d17-4501-9f10-5b141c0e74a6",
  "RiskNums": 1,
  "Rules": [
    {
      "rule_name": "检查Java路径穿越漏洞",
      "language": "java",
      "description": "检测Java路径穿越漏洞",
      "solution": "修复建议",
      "content": "规则实现代码",
      "risks": [
        "c5153eae0789a64ae99b99a1ac7b4fec3a12bc17"
      ]
    }
  ], 
  "Risks": {
    "c5153eae0789a64ae99b99a1ac7b4fec3a12bc17": {
      "id": 1323,
      "hash": "c5153eae0789a64ae99b99a1ac7b4fec3a12bc17",
      "title": "Check Java Path Traversal Vulnerability",
      "title_verbose": "检测Java路径穿越漏洞",
      "description": "漏洞描述信息",
      "solution": "修复建议",
      "severity": "high",
      "risk_type": "path-traversal",
      "details": "Java代码中发现路径穿越漏洞，并且数据流中间没有进行任何过滤。",
      "cve": "",
      "time": "2025-08-08T11:17:50.3410823+08:00",
      "language": "java",
      "code_source_url": "FileUploader.java",
      "line": 24,
      "code_range": "{\"url\":\"/7a8f7821-0d17-4501-9f10-5b141c0e74a6/FileUploader.java\",\"start_line\":24,\"start_column\":62,\"end_line\":24,\"end_column\":77,\"source_code_line\":20}",
      "rule_name": "检查Java路径穿越漏洞",
      "program_name": "7a8f7821-0d17-4501-9f10-5b141c0e74a6",
      "latest_disposal_status": "not_set",
      "data_flow_paths": [
        {
          "path_id": "path_1323",
          "description": "Data flow path for path-traversal vulnerability in 7a8f7821-0d17-4501-9f10-5b141c0e74a6",
          "nodes": [
            {
              "node_id": "n1",
              "ir_code": "Parameter-fileName",
              "source_code": "String fileName",
              "source_code_start": 0,
              "code_range": {
                "url": "/7a8f7821-0d17-4501-9f10-5b141c0e74a6/FileUploader.java",
                "start_line": 24,
                "start_column": 62,
                "end_line": 24,
                "end_column": 77,
                "source_code_line": 20
              }
            }
          ],
          "edges": [
            {
              "edge_id": "e0",
              "from_node_id": "n1",
              "to_node_id": "n2",
              "edge_type": "depend_on",
              "description": "The dependency edge in the dataflow"
            }
          ],
          "dot_graph": "strict digraph {...}"
        }
      ]
    }
  }, 
  "File": [
    {
      "path": "FileUploader.java",
      "length": 3129,
      "hash": {
        "md5": "af73de5509ea017768b9875b62ac10af",
        "sha1": "d67de95394aa7fe980974a82c3f10b1061e1bf3e",
        "sha256": "514232b3a104b0e844053ba1ece2c2231dd494bf30758de2394b69cd2873e496"
      },
      "content": "import java.io.File;\nimport java.io.IOException;\n...",
      "risks": [
        "c5153eae0789a64ae99b99a1ac7b4fec3a12bc17"
      ]
    }
  ]
}
```
