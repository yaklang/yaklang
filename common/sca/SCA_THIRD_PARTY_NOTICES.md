# SCA 来源、许可与维护

产品依然适用仓库根 `LICENSE.md`。下列上游来源的原许可与 NOTICE 随源码/测试保留，不因删除 Go module import 而消失。固定提交、模块校验和与文件摘要见 `upstream-sources.lock.json`；逐文件/符号的本地改动见 `source-extraction-map.json`。

| 来源 | 固定提交 | 用途 | 随附许可 |
|---|---|---|---|
| golang.org/x/mod v0.19.0 | d58be1cb16e62a9821b6dbd0157b8c7ff0b667ec | 只读 go.mod lexer/版本语义片段与测试参考 | `licenses/x-mod/LICENSE`、`core/gomod/LICENSE`，Go BSD |
| BurntSushi/toml v1.3.2 | b324da5ffbb2a87e4a611b1703bd03bffbe7ffbf | 经裁剪 lexer/scalar，固定语法 corpus | `licenses/toml/COPYING`、`core/locktoml/LICENSE`，MIT；corpus 保留自身 LICENSE |
| go-rpmdb v0.1.0 | a8af76a6220fc762827743740291ab88943183be | BDB/NDB/Header 布局参考、数据库 fixtures 与期望 | `licenses/go-rpmdb/LICENSE`、`core/rpm/LICENSE`，MIT；`core/rpm/LICENSE-SUSE` 保留 NDB 的 SUSE 归属 |
| aquasecurity/go-dep-parser | fb7eb3159bd5b83c24e5dbf29e15b4b7ca4618e9 | 多生态纯解析 helper、迁入测试和静态材料；Jar 历史来源 | `licenses/go-dep-parser/LICENSE` 和各 parser 目录 LICENSE，MIT |
| go-yaml/yaml v3.0.1 | f6f7691b1fdeb513f56608cd2c32c51f8194bf51 | 有限 pnpm grammar 的参考，未复制通用运行时 | `licenses/yaml/LICENSE`，保留该文件内的全部归属 |
| cyclonedx-go v0.7.2 | 83031d6697bd6d8b20bce2a0326347a0ea7691c7 | 固定 1.5 schema / DTO 字段参考，离线验证 | `licenses/cyclonedx-go/LICENSE`、`NOTICE`，Apache-2.0 |
| yaklang/go-sqlite3 v0.0.1 | 模块归档 SHA-256 见来源锁 | 核实实际 replace，仅作隔离旧 oracle 依赖；没有复制驱动 | 不进入产品 SCA 闭包；获取状态明确为 module snapshot，非完整 clone |

所有参考 checkout 在产品 module 之外，测试不会自动联网 clone 或运行包管理器。上游测试里的 Docker/Maven/npm/rpm 命令注释是静态材料的生产来源，不是本项目测试执行步骤。生成材料的原始工具版本仅在上游有记录时保留；没有记录的版本不补造。

数据库 fixtures 保存包管理元数据，Go/JAR 测试文件含固定上游二进制材料；其内容来源与摘要逐一列入 `testdata/manifest.json`。该清单是技术来源记录，不宣称为材料内每个包重新授权。

维护责任由 yaklang/yaklang SCA 维护者承担：升级不自动替换为上游 main；先固定新提交、检查上游修复是否影响保留符号、更新来源摘要与许可，再执行对应语义、破坏输入、fuzz、闭包及资源门禁。报告修复时必须关联来源提交与本地回归。没有声称这些派生片段完全原创，也没有把保留完整第三方框架称为接管维护。
