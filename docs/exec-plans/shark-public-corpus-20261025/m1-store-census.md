# M1 捕获样本消费者盘点

这是 `92620d72b0` 迁移前的只读来源盘点，不能冒称新存储的运行验证。
17,198 个 tracked 文件中有 2 个散装捕获输入，36 个现存流量 ZIP，共 38 项
18,907,231 bytes。其他系统的 86 个 ZIP 未选；318 个 lexical Go embed
指令与迁移目标的匹配数为 0。还读取了 hash-pinned 批次中的 1,518 份唯一历史源码，
检查旧 ZIP 目录引用，保留旧脚本、预期和来源字节不改。

消费者包括 trafficfixture 批次/pack/SourceArchive/ExportTools、PCAPDB capture
测试及构建器、AI 工具的独立全字节断言和 live Yak 脚本。历史工具从私有导出 workspace
继续获得旧路径。当前 Go、Windows、大包和 CI 验收另见 M1 收尾记录。

来源 proof hashes 和纯源码目录见 [聚合清单](m1-store-census-summary.json)；
只归档 4 份 Python 控制器及依赖 SHA。完整 inventory、closure、样本和输出仍在仓库外。
封存控制器复现需迁移前布局或其显式兼容 workspace。
