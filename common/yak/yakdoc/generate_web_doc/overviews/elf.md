`elf` 库用于解析 ELF（Linux/Unix 可执行与可链接格式）文件，读取其头部、节（section）、段（segment）、架构与入口点信息，常用于二进制分析、固件检查与恶意样本研判。

典型使用场景：

- 识别与概览：`elf.IsELF` 判断是否 ELF，`elf.DisplayELF` 输出可读概览，`elf.ParseELF` 解析为结构。
- 读取结构：`elf.ReadELFHeader` 读头部，`elf.ReadELFSections` / `elf.GetELFSection` 读节，`elf.ReadELFSegments` / `elf.GetELFSegment` 读段，`elf.GetELFArchitecture` / `elf.GetELFEntryPoint` 取架构与入口。

与相邻库的关系：`elf` 属于二进制分析工具，与 `bin`（通用二进制解析）、`java`（Java 字节码）、`sca`（成分分析）同属逆向/审计方向。
