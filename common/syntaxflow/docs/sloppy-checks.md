# 关键路径上的不严谨判断：模式与规则

对象：用「短常量/单字符/固定下标」代替语义判断，而结果决定协议、凭据、认证、
重定向、URL/主机比较等安全敏感行为。

种子：`ext/soap/php_sdl.c:304`（php-src）

```c
/* check for https://...:443/ */
if (l1 > 13 &&
    ctx->sdl->source[4] == 's' &&          /* 第 5 个字符是 's' 就算 https */
    ctx->sdl->source[l1-4] == ':' &&
    ctx->sdl->source[l1-3] == '4' && ...) {
    l1 -= 4;                                /* 归一化端口后再比主机，决定是否复用凭据 */
}
```

问题不是「用了字符比较」，而是**这个比较承担了它无法承担的语义**：scheme 大小写
不敏感（RFC 3986），但它只比小写 `s`；只看第 5 个字符，`ftps://` 也会命中。

## 1. 四个泛化维度

| 维度 | 取值 | 从 SSA 能拿到吗 |
| --- | --- | --- |
| 形态 | 固定下标单字符（`x[4]`）、指针解引用（`*p`）、定长前缀比较（`strncmp(x,"https",5)`）、短串全比较 | 是：`opcode: param_member`、`form=call` + `callee` + 常量长度 |
| 数据来源 | scheme/URL、host、port、认证信息、重定向目标、cookie、头部、路径 | 部分：表达式文本 + 函数名；没有类型标注 |
| 控制流去向 | 选传输（TLS/明文）、带/不带凭据、是否跟随重定向、主机白名单、认证放行、cookie 作用域、路径校验 | 部分：分支体里出现的调用与字段；没有「这是 sink」的标注 |
| 意图 | 「这段代码在判断是否 https」 | 否：只能靠函数名、文件、注释、被调 API 近似 |

前三维可以从 SSA 组合出来，第四维不能。这是这类规则的精度上限。

## 2. 能表达与不能表达

能表达（`sloppy-positional-check.sf`）：

```
*?{opcode: If || opcode: Loop} as $stmt
$stmt<cfgConditionValues> as $cond
$cond #> ?{opcode: param_member} as $indexed      // s[4]、p[0]、data[l-1]
$cond?{opcode: eq || ...} as $binDirect
$cond #> ?{opcode: eq || ...} as $binNested
$binDirect + $binNested as $bin
$bin #> ?{opcode: const} as $const
```

它回答：「这个条件的比较里有一侧是常量，并且同一条件里出现了按下标读取」。

不能表达：

- **「常量比较的那一侧就是按下标读出来的值」**。`$bin` 与 `$indexed` 是两组
  不同的值，SyntaxFlow 没有「某个操作数」的过滤算子，两组无法在规则里求交。
  现在由导出器检查 SSA 文本里有没有 `ParameterMember-…` 来完成这一步。
- **常量的语义类别**。字符字面量在 SSA 里是整数（`'s'` → `115`），长度、是否
  字母、是否只比小写，都得靠源文本还原（导出器做）。
- **数据来源**。没有「这个值是 URL 的 scheme」这种标注；只能看表达式文本与
  函数名。
- **sink/关键行为**。「这段分支会清空凭据」需要调用图或 sink 白名单。
- **意图**。没有任何标注说明这是一次安全判断。

## 3. 复核方法

规则只产生候选，置信度靠人工读源码。对每个候选看三件事：

1. 比较的值从哪来（参数、解析结果、还是常量拼接）；
2. 这一步判断的**失败方向**（少判→走更严的分支，还是多判→放行）；
3. 分支体里做了什么（清凭据、换传输、跟随跳转、放行认证）。

判定分三档：

- `same-class`：形态 + 关键行为都成立，读源码确认（例如种子）。
- `benign`：形态相同，但有长度/前置校验，或失败方向是保守的。
- 未复核：只按启发式排序，未读源码。

## 4. 在 php-src 上的实测

漏斗（规则 + 导出器，数据库 4,629,021 条指令）：

| 筛选 | 数量 |
| --- | ---: |
| 条件里与常量比较 | 39,512 |
| 常量是字符/字符串 | 12,185 |
| 比较涉及下标/解引用 | 1,867 |
| 固定下标且常量是字母 | 153 |
| 进入候选（形态 + 领域 + 分支行为） | 317 条 / 220 处 |

220 处里：`same-class` 4 处（同一函数的 2 个判断，各自对 source 与 uri 各写一遍）、
复核后排除 5 处、其余未复核。分布：ext 173、sapi 39、main 8；形态以固定下标
（112）为主。

结论：这类问题在 php-src 里**不是到处都是**。单字符判断很多（11,804 条），但绝大
多数是格式解析（转义、数字、日期），落在安全关键路径上的只有个位数。

## 5. 成本与收益

- 规则本身几分钟能跑完；瓶颈在复核。220 处候选要读源码，按每处 2 分钟算是 7 小时。
- 提高精度的办法都是项目定制：sink 白名单（`php_stream_xport_create`、
  `php_libxml_switch_context`、`Set-Cookie` 处理）、函数名白名单
  （`*scheme*`、`*auth*`、`*redirect*`）、按目录收窄。
- 收益判断：对**安全产品**（比如 SOAP/WSDL、URL 解析器、认证中间件）值得做成常规
  检查；对通用代码库，投入产出比低。

## 6. 复现

```sh
RUN=/mnt/data/scan-outputs/php-src-string-compare-20261120-220345
# 1. 两条条件规则跑出全量比较（见本目录 README.md）
# 2. 用导出器落 JSON
$RUN/oom-ab/sf_report-v7 -db $RUN/db/final-ssadb.db -program php-src-full-final \
  -rule common/syntaxflow/rules/condition-const-compare.sf \
  -call-rule common/syntaxflow/rules/condition-call-strcmp-literal.sf \
  -target /home/wlz/Target/php-src -out /tmp/report.json
# 3. 候选清单
python3 $RUN/audit/sloppy_candidates.py /tmp/report.json /tmp/cands.json /tmp/cands.md
```