# 流解析工具

rule文件以yaml格式编写，支持解析多种格式的数据流，二进制文件、链路层、网络层、应用层协议等。

## 1. rule组成
rule的根节点为Package，其下包含多个子节点，用来描述数据的结构、属性。
如
```yaml
Package:
  Rule1: xxx
  Rule2: xxx
```
## 2. rule节点
每个节点由属性和子节点组成，属性用小写字母开头的key表示，子节点用大写字母开头的key表示。
如一个Package节点
```yaml
Package:
  endian: big
  isList: true
  Rule1: xxx
```
其中endian、isList为Package的属性，Rule1为Package的子节点。
## 3. 属性
默认解析器内置了一些属性，包括：
- endian: 字节序，big或little，默认为big
- isList: 是否为列表，true或false，默认为false
- length: 长度，用于描述固定长度的数据，如uint32、ipv4等
## 4. operator
operator是一个特殊的属性，可以编写yak代码，控制解析流程
如
```yaml
Package:
  endian: big
  isList: true
  length: 10
  operator: |
    d = this.Rule1.Process()
    if d == 1 {
      this.Rule2.Process()
    }
  Rule1: raw,10
  Rule2: raw,10
```
## 5. Context
operator可以使用context传输上下文数据，如
```yaml
Package:
  endian: big
  isList: true
  length: 10
  operator: |
    this.SetCtx("rule1-length", 1)
  Rule1: 
    operator: |
      this.GetCtx("rule1-length")
```
## 6. 默认解析器
可以通过属性、上下文控制默认解析器的解析行为，上面提到了属性，下面介绍上下文
- stopArray: 停止数组解析，true或false，默认为false