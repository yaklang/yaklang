## fuzztag 可用标签一览

|标签名|标签别名|标签描述|
|:-------|:-------|:-------|
|`array`|`list`|设置一个数组，使用 `&#124;` 分割，例如：`{{array(1&#124;2&#124;3)}}`，结果为：[1,2,3]，|
|`base64dec`|`base64decode, base64d, b64d`|进行 base64 解码，{{base64dec(YWJj)}} => abc|
|`base64enc`|`base64encode, base64e, base64, b64`|进行 base64 编码，{{base64enc(abc)}} => YWJj|
|`base64tohex`|`b642h, base642hex`|把 Base64 字符串转换为 HEX 编码，{{base64tohex(YWJj)}} => 616263|
|`bmp`|  |生成一个 bmp 文件头，例如 {{bmp}}|
|`char`|`c, ch`|生成一个字符，例如：`{{char(a-z)}}`, 结果为 [a b c ... x y z]|
|`codec`|  |调用 Yakit Codec 插件|
|`codec:line`|  |调用 Yakit Codec 插件，把结果解析成行|
|`date`|  |生成一个时间，格式为YYYY-MM-dd，如果指定了格式，将按照指定的格式生成时间|
|`datetime`|`time`|生成一个时间，格式为YYYY-MM-dd HH:mm:ss，如果指定了格式，将按照指定的格式生成时间|
|`doubleurldec`|`doubleurldecode, durldec, durldecode`|双重URL解码，{{doubleurldec(%2561%2562%2563)}} => abc|
|`doubleurlenc`|`doubleurlencode, durlenc, durl`|双重URL编码，{{doubleurlenc(abc)}} => %2561%2562%2563|
|`file`|  |读取文件内容，可以支持多个文件，用竖线分割，`{{file(/tmp/1.txt)}}` 或 `{{file(/tmp/1.txt&#124;/tmp/test.txt)}}`|
|`file:dir`|`filedir`|解析文件夹，把文件夹中文件的内容读取出来，读取成数组返回，定义为 `{{file:dir(/tmp/test)}}` 或 `{{file:dir(/tmp/test&#124;/tmp/1)}}`|
|`file:line`|`fileline, file:lines`|解析文件名（可以用 `&#124;` 分割），把文件中的内容按行反回成数组，定义为 `{{file:line(/tmp/test.txt)}}` 或 `{{file:line(/tmp/test.txt&#124;/tmp/1.txt)}}`|
|`fuzz:password`|`fuzz:pass`|根据所输入的操作随机生成可能的密码（默认为 root/admin 生成）|
|`fuzz:username`|`fuzz:user`|根据所输入的操作随机生成可能的用户名（默认为 root/admin 生成）|
|`gif`|  |生成 gif 文件头|
|`headerauth`|  ||
|`hexdec`|`hexd, hexdec, hexdecode`|HEX 解码，{{hexdec(616263)}} => abc|
|`hexenc`|`hex, hexencode`|HEX 编码，{{hexenc(abc)}} => 616263|
|`hextobase64`|`h2b64, hex2base64`|把 HEX 字符串转换为 base64 编码，{{hextobase64(616263)}} => YWJj|
|`htmldec`|`htmldecode, htmlunescape`|HTML 解码，{{htmldec(&#97;&#98;&#99;)}} => abc|
|`htmlenc`|`htmlencode, html, htmle, htmlescape`|HTML 实体编码，{{htmlenc(abc)}} => &#97;&#98;&#99;|
|`htmlhexenc`|`htmlhex, htmlhexencode, htmlhexescape`|HTML 十六进制实体编码，{{htmlhexenc(abc)}} => &#x61;&#x62;&#x63;|
|`ico`|  |生成一个 ico 文件头，例如 `{{ico}}`|
|`int`|`port, ports, integer, i, p`|生成一个整数以及范围，例如 {{int(1,2,3,4,5)}} 生成 1,2,3,4,5 中的一个整数，也可以使用 {{int(1-5)}} 生成 1-5 的整数，也可以使用 `{{int(1-5&#124;4)}}` 生成 1-5 的整数，但是每个整数都是 4 位数，例如 0001, 0002, 0003, 0004, 0005|
|`jpg`|`jpeg`|生成 jpeg / jpg 文件头|
|`lower`|  |把传入的内容都设置成小写 {{lower(Abc)}} => abc|
|`md5`|  |进行 md5 编码，{{md5(abc)}} => 900150983cd24fb0d6963f7d28e17f72|
|`network`|`host, hosts, cidr, ip, net`|生成一个网络地址，例如 `{{network(192.168.1.1/24)}}` 对应 cidr 192.168.1.1/24 所有地址，可以逗号分隔，例如 `{{network(8.8.8.8,192.168.1.1/25,example.com)}}`|
|`null`|`nullbyte`|生成一个空字节，如果指定了数量，将生成指定数量的空字节 {{null(5)}} 表示生成 5 个空字节|
|`padding:null`|`nullpadding, np`|使用 \x00 来填充补偿字符串长度不足的问题，{{nullpadding(abc&#124;5)}} 表示将 abc 填充到长度为 5 的字符串（\x00\x00abc），{{nullpadding(abc&#124;-5)}} 表示将 abc 填充到长度为 5 的字符串，并且在右边填充 (abc\x00\x00)|
|`padding:zero`|`zeropadding, zp`|使用0来填充补偿字符串长度不足的问题，{{zeropadding(abc&#124;5)}} 表示将 abc 填充到长度为 5 的字符串（00abc），{{zeropadding(abc&#124;-5)}} 表示将 abc 填充到长度为 5 的字符串，并且在右边填充 (abc00)|
|`payload`|`x`|从数据库加载 Payload, `{{payload(pass_top25)}}`|
|`png`|  |生成 PNG 文件头|
|`punctuation`|`punc`|生成所有标点符号|
|`quote`|  |strconv.Quote 转化|
|`randint`|`ri, rand:int, randi`|随机生成整数，定义为 {{randint(10)}} 生成0-10中任意一个随机数，{{randint(1,50)}} 生成 1-50 任意一个随机数，{{randint(1,50,10)}} 生成 1-50 任意一个随机数，重复 10 次|
|`randomupper`|`random:upper, random:lower`|随机大小写，{{randomupper(abc)}} => aBc|
|`randstr`|`rand:str, rs, rands`|随机生成个字符串，定义为 {{randstr(10)}} 生成长度为 10 的随机字符串，{{randstr(1,30)}} 生成长度为 1-30 为随机字符串，{{randstr(1,30,10)}} 生成 10 个随机字符串，长度为 1-30|
|`rangechar`|`range:char, range`|按顺序生成一个 range 字符集，例如 `{{rangechar(20,7e)}}` 生成 0x20 - 0x7e 的字符集|
|`regen`|`re`|使用正则生成所有可能的字符|
|`repeat`|  |重复一个字符串，例如：`{{repeat(abc&#124;3)}}`，结果为：abcabcabc|
|`repeat:range`|  |重复一个字符串，并把重复步骤全都输出出来，例如：`{{repeat(abc&#124;3)}}`，结果为：['' abc abcabc abcabcabc]|
|`repeatstr`|`repeat:str`|重复字符串，`{{repeatstr(abc&#124;3)}}` => abcabcabc|
|`sha1`|  |进行 sha1 编码，{{sha1(abc)}} => a9993e364706816aba3e25717850c26c9cd0d89d|
|`sha224`|  ||
|`sha256`|  |进行 sha256 编码，{{sha256(abc)}} => ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad|
|`sha384`|  ||
|`sha512`|  |进行 sha512 编码，{{sha512(abc)}} => ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f|
|`sm3`|  |计算 sm3 哈希值，{{sm3(abc)}} => 66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a3f0b8ddb27d8a7eb3|
|`tiff`|  |生成一个 tiff 文件头，例如 `{{tiff}}`|
|`timestamp`|  |生成一个时间戳，默认单位为秒，可指定单位：s, ms, ns: {{timestamp(s)}}|
|`trim`|  |去除字符串两边的空格，一般配合其他 tag 使用，如：{{trim({{x(dict)}})}}|
|`unquote`|  |把内容进行 strconv.Unquote 转化|
|`upper`|  |把传入的内容变成大写 {{upper(abc)}} => ABC|
|`urldec`|`urldecode, urld`|URL 强制解码，{{urldec(%61%62%63)}} => abc|
|`urlenc`|`urlencode, url`|URL 强制编码，{{urlenc(abc)}} => %61%62%63|
|`urlescape`|`urlesc`|url 编码(只编码特殊字符)，{{urlescape(abc=)}} => abc%3d|
|`uuid`|  |生成一个随机的uuid，如果指定了数量，将生成指定数量的uuid|
|`yso:bodyexec`|  |尽力使用 class body exec 的方式生成多个链|
|`yso:dnslog`|  ||
|`yso:exec`|  ||
|`yso:find_gadget_by_bomb`|  ||
|`yso:find_gadget_by_dns`|  ||
|`yso:headerecho`|  |尽力使用 header echo 生成多个链|
|`yso:urldns`|  ||
