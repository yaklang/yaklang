# SSH具体方法身份与未知布局：cc9d检查点

cc9d只按精确算法名选择12种已经实现完整初始布局的方法，精确移除4个协商能力标志。
私有算法名包含DH、GEX、curve等字样时，保留真实名称及SSH公共封装，数据体保持opaque；
只有完整解析的已选reply才生成其实际主机密钥blob的指纹。指纹不代表签名验证或对端认证。
first_kex_packet_follows按排除精确能力标志后的第一个真实算法处理。

## 独立源字节与运行差别

12个人工wire控制共148个物理帧，包含完整TCP SYN字节、server-first、反向地址、同tuple
新SYN代、猜测包、未知私名和NEWKEYS后的不透明哨兵；不是新抓的真实SSH连接。
同一个事先冻结的字段预期，旧版60/76个payload通过，cc9d为76/76；16处改善分布在
6个私名控制。842个API叶子观测、275次私有字段观测、12项字段描述符均通过。
608个源字段观测与API/别名计数具有不同粒度，不能相加为独立字节覆盖。

76个payload另有11条FIN触发的握手未完成诊断，合计87个SDK回调。CLI仍是148个物理行，
11条诊断嵌在相应FIN行的published_pdus里，FIN不被伪造成原始字节贡献者。
初版checker漏计这11条和已有连接caption；后续合同分别严格核全部诊断DTO及连接注释后，
再调用字节未变的原76条预期。原失败、自测负控与版本化修正全部保留。

## 与Wireshark逐包对照

TShark4.6.9使用隔离配置，自然heuristic与显式decode-as各运行PDML/字段导出。
每种模式762个primitive、48个空字符串和24个不含CRLF的identification内容相符；
16个aggregate观测对应7个owner，另有13个未映射观测及3个未渲染alias，均不冒称字段通过。
空字符串的零宽PDML位置不是高亮范围证明。比较器初版误读缺省value属性、closure漏计
HASSH角色标签的失败与修正版本均保存，源字节和值没有改写。

参考对未知私名会回退标准DH布局；9个帧出现标准方法字段，12个帧出现标准方法caption，
两类重叠，不能相加。私名reply中的任意首string不能因此成为主机密钥。
参考也会解析应忽略的猜测包。server-first正控自然模式6个payload方向与已观察SYN角色
相反，显式decode-as修正方向和4个HASSH角色标签；实际标识字符串及摘要值没有变化。
相关实现见[Wireshark4.6.9 SSH dissector](https://github.com/wireshark/wireshark/blob/v4.6.9/epan/dissectors/packet-ssh.c)。
参考结果是逐项对照材料，不能替代RFC和捕获字节的约束。

## Windows实际页面与完整回放

Windows实际控制台frame69保留Ethernet/IPv4/TCP/SSH层级，显示私有方法真名与
KEY_EXCHANGE_31 (layout unconfirmed)，不生成标准reply或主机密钥标签。完整物理源86B、
完整PDU32B与所选opaque值18B逐字节核对；当前SSH API没有叶子偏移，页面明确使用
Extracted field value / frame offsets unavailable，不用字节搜索伪造位置。
验证包含CP936恢复、鼠标/标签/resize及默认20000包缓存。图像由真实控制台cells/attributes
重建，不冒称Windows Terminal像素截图。最初两次过度滚动属于导航失败，已保留；默认位置通过。

新二进制重放37组外部来源，307819个完整native对象、384518条完整CLI记录与两份完整
flow文件对上一版逐字节一致，无ID映射或字段归一化。CLI含383936个物理行及582条
非物理记录；不能把总记录数当物理包数。这证明连续性，不能证明所有旧解析都正确。
396项SSH定向测试和cc9d四项CI通过；没有把旧全量结果写成cc9d的新全量测试。

## 范围与剩余工作

本检查点没有证明完整SSH消息顺序、对端认证、所有算法、SFTP或TOP30完备。
P384/P521/Curve448和其他私名保留opaque，后续端点角色约束单独审查。
仓库只保存23个精确源文件及聚合摘要；PCAP/keylog/raw blobs、完整预期、SDK/CLI/PDML、
图像、日志、二进制、完整RFC和Wireshark C仍在仓库外。
