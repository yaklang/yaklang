# CDSL-Yaklang: Cybersecurity Domain-Specific Language

为了加速安全产品和安全工具的工程化研发，我们研发了一门新的语言（Yaklang）和并且同时实现了一个栈虚拟机（YakVM）。

In order to improve the development process of security products and hacking tools, we have created a new language (
Yaklang) and implemented a stack-based virtual machine (YakVM) for this language.

Yaklang 是一门图灵完备的过程式语言，其语法由上下文无关文法定义。它运行在 YakVM 上。

Yaklang is a Turing-complete procedural language defined by context-free grammar. It runs on YakVM.

## 为什么要做 DSL? (Why DSL?)

1. 提高生产力。DSL 设计简洁高效,专注于解决特定问题,可以大大提高开发效率和生产力。

1. 改善抽象能力。DSL 可以帮助开发者利用高层抽象构建解决方案,不需要处理底层细节,提高开发效率。

1. 可维护性好。DSL 语言简单明了,代码也更加清晰易读,这有利于代码的维护和扩展。

1. 可靠性高。DSL 专注一定领域,语言和语义都更加精确,这有助于编写出更加可靠的程序。

1. 易于嵌入。DSL可以很容易地嵌入到一门宿主语言中,实现起来非常方便。

### Translation:

Improved productivity. DSL is designed to be concise and efficient, focusing on solving specific problems, which can greatly improve development efficiency and productivity.

Improved abstraction. DSL can help developers build solutions using high-level abstractions without dealing with low-level details, improving development efficiency.

High maintainability. DSL languages are simple and clear, and the code is also more readable, which is beneficial for code maintenance and expansion.

High reliability. DSL focuses on a certain field, the language and semantics are more precise, which helps to write more reliable programs.

Easy to embed. DSL can be easily embedded in a host language, which is very convenient to implement.