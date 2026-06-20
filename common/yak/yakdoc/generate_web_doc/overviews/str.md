`str` 库是 yaklang 的字符串处理超集（约 165 个函数），在标准字符串操作之外，深度集成了网络/HTTP/安全相关的解析与判定工具，是几乎所有脚本都会用到的基础库。

典型使用场景：

- 基础操作：`str.Split` / `str.Join` / `str.Trim*` / `str.Replace` / `str.Contains` / `str.HasPrefix` / `str.ToLower` / `str.f`（格式化），`str.RandStr` / `str.Random` 生成随机串。
- 目标解析：`str.ParseStringToHosts` / `str.ParseStringToPorts` / `str.ParseStringToUrls` / `str.ParseStringToCClassHosts` / `str.HostPort` 把目标描述解析为可扫描列表。
- HTTP 处理：`str.ParseStringToHTTPRequest` / `str.SplitHTTPHeadersAndBodyFromPacket` / `str.ExtractBodyFromHTTPResponseRaw` / `str.FixHTTPRequest` / `str.ExtractTitle` / `str.ExtractURLFromHTTPRequestRaw`。
- 抽取与判定：`str.ExtractDomain` / `str.ExtractRootDomain` / `str.ExtractChineseIDCards`、`str.IsIPv4` / `str.IsHttpURL` / `str.IsJsonResponse` / `str.IsPasswordField` 等大量 `Is*` 判定。
- 相似度与匹配：`str.CalcSimilarity` / `str.CalcSimHash` / `str.CalcSSDeep` 模糊比对，`str.MatchAnyOfGlob` / `str.MatchAllOfRegexp` 批量匹配，`str.Grok` 结构化解析，`str.VersionCompare` 版本比较。

与相邻库的关系：`str` 是纯计算基础库，与 `re`（正则）、`codec`（编解码）、`json`（结构化）协同；它内置的目标/HTTP 解析能力让它在扫描类脚本中尤为关键。
