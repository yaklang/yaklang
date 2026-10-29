# SSH加密字节来源：UI 7fcb

7fcb修复把NEWKEYS后不透明流字节称为完整PDU的展示。只有既有的明确SSH加密诊断
选择这条路径；普通SSH、错误方向的malformed消息、TLS解密子消息和QUIC沿用原来源规则。
左侧用observation记录已保留的观察，私有pdu.id和查询身份保持不变。

## 实际Windows页面

同一115物理帧的人工来源跑了3个实际控制台案例；不是新增真实SSH连接。

| 情况 | 物理帧 | 实际捕获源 | 当前保留源 | 页面标题 |
|---|---:|---:|---:|---|
| 客户端错误发送reply |36|246B|完整192B消息封装|SSH · reassembled PDU，malformed，具体角色诊断|
| NEWKEYS后截余预览 |13|246B|64/192B|SSH · encrypted stream preview · 64/192 observed bytes|
| NEWKEYS后整段保留 |11|102B|48/48B|SSH · encrypted stream bytes · 48 observed bytes|

三种情况的完整原始捕获、保留源和切换后的字节逐一与独立来源核对。帧长度始终正确。
选择Expected Sender Role=server不制造字节高亮。加密两种情况均不再自称完整PDU，
48B全部保留也没有被当作已解码消息。原始115个缓存包、默认20000容量和64MiB历史
设置、CP936恢复、鼠标/标签/resize均检查。图像来自实际控制台cells/attributes，
不是Windows Terminal像素截图。原先64/64长度的视觉误读已纠正，不是产品数据损坏。

## 验证口径

CLI全包895通过、2跳过、0失败；新增回归先出现7个失败动作/6个通过动作，修复后全过。
其中JSON往返只证明DTO字段的序列化，真实页面由上面3个实际运行补充验证。
新版重新运行source50的SDK/CLI：645个API原语、243个SSH别名、12项描述符、
64个payload加7条FIN诊断的完整树及桥均通过。三个完整导出与26f逐字节一致。

新版events/flows二进制也与26f逐字节相同，后台Go/rules没有变化。37输入完整回放的
307819个native对象、384518条CLI记录和两份whole-flow连续性依据仍是26f闭合执行；
本报告不把它写成新做的7fcb大样本回放，也不把同字节当作全部协议正确性证明。
CI以PR当前精确提交的检查为准，不在报告中预填未来成功状态。

8个精确Python控制器及聚合摘要已归档。完整PCAP/keylog/raw、plans/expected、
导出/收据/日志、图像/二进制、完整RFC/vendor C仍在仓库外。SSH叶子范围、完整重协商、
主机/签名认证和TOP30完备仍未由本检查点证明。外部future wire-range source51账本
只是下一步准备，和本UI51修复是两个独立工作。
