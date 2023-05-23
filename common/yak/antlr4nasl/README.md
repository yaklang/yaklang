# NASL 的一些语法规则

## 变量类型
INT、STRING、DATA、ARRAY、UNDEF

| 类型 | 说明 | 映射到的 Go 类型       |
| --- | --- |------------------|
| INT | 整型 | int64            |
| STRING | 字符串 | string           |
| DATA | 二进制数据 | []byte           |
| ARRAY | 数组 | struct NaslArray |
| UNDEF | 未定义 | nil              |

array类型是一个特殊类型，和传统的array不同，它既是map又是list，后端存在形式是hash_index和num_index