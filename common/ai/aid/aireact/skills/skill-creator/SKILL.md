---
name: skill-creator
description: >
  Skill 创建与更新技能。用于在 Yaklang AI ReAct 技能体系中设计、初始化、校验和迭代 Skill，
  适用于新增或修改 common/ai/aid/aireact/skills 下的技能目录、编写或重构 SKILL.md、
  规划 scripts/references/assets 资源、把重复流程沉淀为 Yak 脚本，或在交付前执行
  Skill 结构校验。当用户要求创建 skill、更新 skill、设计 skill 目录结构、编写 skill 脚本、
  补充 skill 参考资料、校验 skill 元数据时，应优先使用此技能。
---

# Skill Creator

用于创建和更新 Yaklang AI ReAct Skill。这个技能面向另一个 AI Agent，而不是面向最终用户文档。
目标是让 Agent 用最少的上下文读到最关键的程序化知识，并在需要时再按需加载资源文件。

---

## 1. 先理解 Skill 在 Aireact 里的结构

在当前仓库里，Skill 的入口文件是 `SKILL.md`。Skill Loader 会先读取 frontmatter 中的 `name` 和 `description`，
再决定要不要把正文加载进上下文。其余资源文件由 Agent 通过 `load_skill_resources` 按需加载。

推荐目录结构：

```text
common/ai/aid/aireact/skills/<skill-name>/
├── SKILL.md
├── scripts/        # 可执行脚本，优先使用 .yak
├── references/     # 按需加载的说明文档
└── assets/         # 最终产物会直接用到的模板或素材
```

核心原则：

1. 保持 `SKILL.md` 精炼，让 `description` 同时描述“做什么”和“什么时候触发”。
2. 把重复且易错的操作沉淀到 `scripts/*.yak`，不要每次临时重写。
3. 把长文档放到 `references/`，让 Agent 通过 `load_skill_resources` 按需读取。
4. 只有当最终输出确实要复制或消费某个文件时，才把它放进 `assets/`。

---

## 2. 创建或更新 Skill 的工作流

### Step 1: 收集具体用例

先明确这个 Skill 需要覆盖哪些真实请求，例如：

- 用户会说什么，才应该触发这个 Skill
- 需要操作哪些文件格式、目录或工具
- 哪些步骤每次都会重复，值得沉淀成脚本

### Step 2: 规划可复用资源

根据用例拆分资源：

- `scripts/`：稳定、重复、需要确定性的流程，优先写成 Yak 脚本
- `references/`：数据库结构、接口说明、领域规则、较长工作流说明
- `assets/`：模板、样板工程、图标、字体、示例输入输出

### Step 3: 初始化目录

优先加载并执行 `@skill-creator/scripts/init_skill.yak`。

先理解：`init_skill.yak` 只负责生成脚手架，不会自动把 Skill 写完整。
执行成功后，必须继续修改生成出来的 `SKILL.md`、`scripts/`、`references/`、`assets/`，直到该 Skill 可以被实际使用。

如果你不确定脚本参数，先看帮助：

```bash
yak /abs/path/to/init_skill.yak -h
```

核心参数：

- `--name`: skill 名称，会被规范化为小写连字符格式
- `--path`: skill 父目录。当前仓库一般应使用 `common/ai/aid/aireact/skills`
- `--resources`: 需要预创建的资源目录，逗号分隔，可选值为 `scripts,references,assets`
- `--examples`: 是否在已创建的资源目录中放入示例文件

常见命令示例：

```bash
yak /abs/path/to/init_skill.yak --name my-skill --path common/ai/aid/aireact/skills
yak /abs/path/to/init_skill.yak --name my-skill --path common/ai/aid/aireact/skills --resources scripts,references
yak /abs/path/to/init_skill.yak --name my-skill --path common/ai/aid/aireact/skills --resources scripts,references,assets --examples
```

如果你已经拿到了脚本绝对路径，直接运行：

```bash
yak /abs/path/to/init_skill.yak --name my-skill --path common/ai/aid/aireact/skills --resources scripts,references
```

如果还没有脚本路径，先使用 `load_skill_resources`：

- `resource_path`: `@skill-creator/scripts/init_skill.yak`
- `resource_type`: `script`

### Step 4: 编写 SKILL.md 和资源文件

重要：运行 `init_skill.yak` 后不能结束任务。初始化只是生成占位模板，后续至少要完成下面这些动作：

1. 打开新建目录里的 `SKILL.md`
2. 替换 frontmatter 里的占位 `description`
3. 补完正文流程，而不是保留 TODO
4. 根据需求实现或删除 `scripts/`、`references/`、`assets/` 中的示例文件
5. 如果没有某类资源需求，就不要保留空目录或无意义示例

更新 `SKILL.md` 时重点关注：

1. frontmatter 的 `description` 必须覆盖触发条件
2. 正文只保留 Agent 真正需要的流程知识
3. 使用命令示例、路径示例、决策顺序，而不是空泛描述
4. 引导 Agent 在需要时加载 `references/` 和 `scripts/`

### Step 5: 校验

在提交前运行 `quick_validate.yak`：

```bash
yak /abs/path/to/quick_validate.yak --skill-dir /abs/path/to/skill-folder
```

这个脚本会检查 `SKILL.md` frontmatter、命名规则和描述字段的基本合法性。

### Step 6: 迭代

在真实任务中使用 Skill，观察：

- 触发描述是否太宽或太窄
- `SKILL.md` 是否太长，是否应该拆到 `references/`
- 是否有步骤被重复重写，应该提取到 Yak 脚本

---

## 3. Aireact 里的 Skill 编写约束

当前 Skill Loader 重点识别这些 frontmatter 字段：

- `name`
- `description`
- `license`
- `compatibility`
- `metadata`
- `disable-model-invocation`

其中最关键的是：

- `name`：唯一标识，建议使用小写字母、数字和连字符
- `description`：主触发条件，必须写清“做什么”和“何时使用”

不要把“什么时候触发”只写在正文里，因为正文只有在 Skill 被选中后才会被加载。

---

## 4. 资源加载约定

这个运行时支持按需加载 Skill 子文件，所以应主动利用 `load_skill_resources`：

- 获取脚本绝对路径：`@skill-creator/scripts/quick_validate.yak`
- 执行脚本：`yak /absolute/path/to/script.yak ...`

当脚本是 Yak 脚本时，始终使用 `yak` 来执行，而不是把脚本内容复制到回答里。

---

## 5. 附带脚本说明

本 Skill 自带三类脚本：

1. `init_skill.yak`
   用于创建 Skill 目录、生成 `SKILL.md` 模板和可选资源目录。它只生成脚手架，不会完成 skill 内容本身。
2. `quick_validate.yak`
   用于快速校验 `SKILL.md` 的 frontmatter 和命名约束。

---

## 6. 何时优先写 Yak 脚本

优先把下面这类工作写成 Yak：

- Skill 初始化脚手架
- 重复的目录生成和模板写入
- frontmatter / YAML / 文件结构校验
- 大量文本拼装、报告生成、规则转换

如果逻辑只是一次性说明，不必强行脚本化；但只要同类代码会被多次重写，就优先落到 Yak。
