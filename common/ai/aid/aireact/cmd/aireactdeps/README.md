# AI ReAct CLI 工具

这是一个重构后的 AI ReAct 交互式命令行工具，使用 `urfave/cli` 框架构建。

## 文件结构

- **main.go** - 主入口文件，包含 CLI 框架配置和应用初始化
- **state.go** - 全局状态管理，包含所有共享状态和互斥锁
- **handlers.go** - 事件处理和用户输入处理逻辑
- **output.go** - 输出格式化和流处理功能
- **breakpoint.go** - 断点调试功能
- **setup.go** - 数据库和配置初始化

## 使用方法

```bash
# 构建
go build -o aireact

# 查看帮助
./aireact --help

# 基本使用
./aireact

# 调试模式
./aireact --debug

# 非交互模式
./aireact --no-interact

# 断点模式
./aireact --breakpoint

# 一次性查询
./aireact --query "你的问题"
```

## 重构改进

1. **使用 urfave/cli 框架** - 与项目其他部分保持一致的 CLI 框架
2. **模块化设计** - 将大文件拆分为功能专一的小模块
3. **状态管理** - 集中化的全局状态管理，使用互斥锁确保线程安全
4. **更好的维护性** - 代码结构清晰，易于理解和维护

## 主要功能

- 交互式 AI 对话
- 工具使用审核
- 断点调试
- 实时流输出
- 时间线和队列信息显示
- 多语言支持（中文/英文）

