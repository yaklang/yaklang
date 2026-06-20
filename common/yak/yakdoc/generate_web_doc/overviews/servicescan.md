servicescan 是服务指纹识别(全连接扫描)模块：对目标主机的端口建立完整 TCP 连接，发送探测包并比对指纹规则，识别出端口上运行的服务名称、版本与 CPE 信息。相比 synscan 只判断"端口是否开放"，servicescan 更精准，能回答"这个开放端口上跑的是什么服务"。

典型用法是先用 synscan 快速筛出开放端口，再用 servicescan.ScanFromSynResult 对开放端口做指纹识别，兼顾速度与精度；也可以直接用 servicescan.Scan 对少量目标做端到端扫描。结果以 channel 流式返回 *MatchResult，可一边扫描一边消费，对每个结果调用 IsOpen() 判断开放、String() 获取可读摘要、GetCPEs()/GetProto() 获取 CPE 与协议。

模块支持服务(nmap)指纹与 Web 指纹两套规则：用 service() 仅跑服务指纹、web() 仅跑 Web 指纹、all() 同时启用；并提供并发(concurrent)、探测超时(probeTimeout)、主动发包(active)、代理(proxy)、可取消上下文(context) 等大量可选项。可与 synscan、ping、spacengine(网络空间测绘) 等模块联动，是资产发现阶段的核心工具。
