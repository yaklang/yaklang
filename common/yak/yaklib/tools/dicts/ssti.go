// ssti.go — SSTI payload 字典已迁移至 ssti_embed.go，
// 通过 embed + XOR 加载，避免二进制中出现明文恶意特征。
// 原始明文文件位于 embed_data/ssti.txt，编码后为 embed_data/ssti.txt.enc。
package dicts
