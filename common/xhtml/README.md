# compare比较规则记录

## 属性
比较内容：数量，key，value
是否继续比较：false
特殊属性:style，
## text
比较内容：data
是否继续比较：false
## tag
比较内容：data
是否继续比较：false
## 注释
比较内容：data
是否继续比较：false

## 问题
script 标签内检测，例：example7.php
example8.php特殊情况
example9.php自己x自己，没意义

NewXssFuzz方法提取get和post参数
多个参数如何fuzz：GenFuzzParams

script处检查不严谨

## 所有回显位置
属性、text、comment、path

## 所有绕过方式
大小写、双写、单引号，双引号，无引号、a标签伪协议、/代替属性的空格、
<img/src=x onerror=alert(1)>，<video src=x onerror=alert(1)>，<audio src=x onerror=alert(1)>

## 构造payload
属性（单引号、双引号、没引号），文本（script标签）

## 后端行为猜测
直接插入文本（闭合标签内文本、闭合属性）

## 过滤
php后端htmlspecialchars

特殊位置绕过方式
属性可以html实体编码，

参考文献
https://www.freebuf.com/vuls/256239.html
https://www.ddosi.org/xss-bypass/#%E6%B2%A1%E6%9C%89%E8%BF%87%E6%BB%A4%E5%99%A8%E8%A7%84%E9%81%BF%E7%9A%84%E5%9F%BA%E6%9C%AC_XSS_%E6%B5%8B%E8%AF%95
https://github.com/payloadbox/xss-payload-list
https://github.com/ethicalhackersrepo/Xss-payloads
