# Antlr4 Yak

Generate Code

`antlr -Dlanguage=Go ./YaklangLexer.g4 ./YaklangParser.g4 -o parser -no-listener -visitor`

# TODO

statement
- [ ] go
- [ ] include
- [ ] defer
- [x] assert
- [x] return
- [ ] importmod
- [ ] range
- [ ] func
- [ ] recover
变量
- [ ] new/make/chan
- [ ] const
- [ ] type
- [ ] struct
- [ ] interface
操作符
- [ ] in
- [x] .获取成员
- [x] []获取元素
流程控制
- [ ] if/elif/else
- [ ] for
- [ ] switch
其它
- [ ] 语法警告提示
- [ ] 语法错误提示
- [x] 支持char类型

