# HIDS MustPass 脚本报告 (files-hids)

生成时间: `2026-02-25 12:04:20`

运行环境: `linux/amd64` Go `go1.24.6`

测试数据目录(TEST_DIR): `/tmp/mustpass-hids-test`

本报告基于 `common/yak/yaktest/mustpass/files-hids/README.md` 的功能清单，并通过 Go mustpass harness 顺序执行每个 `.yak` 脚本来采集真实输出。

## 如何运行

### 1) 运行全部 HIDS mustpass 用例

```bash
go test ./common/yak/yaktest/mustpass -run TestMustPassHIDS -count=1 -v
```

### 2) 仅运行单个脚本(子测试)

Go 的 `-run` 是正则匹配，注意 `.` 需要转义：

```bash
go test ./common/yak/yaktest/mustpass -run 'TestMustPassHIDS/elf_header\.yak' -count=1 -v
```

### 3) 生成本报告(采集输出)

```bash
YAK_MUSTPASS_REPORT=1 go test ./common/yak/yaktest/mustpass -run TestGenerateHIDSMustpassReport -count=1 -timeout 20m
```

报告输出: `common/yak/yaktest/mustpass/files-hids/REPORT.md`

### 4) 用 yak CLI 直接执行脚本

yak CLI 的默认行为是: 参数跟一个文件路径时直接执行该 `.yak` 文件(参见 `common/yak/cmd/yak.go`).

```bash
go build -o yak common/yak/cmd/yak.go
./yak common/yak/yaktest/mustpass/files-hids/connection_list.yak
```

注意: mustpass harness 会注入 `TEST_DIR` 等参数；CLI 直接跑脚本时如果脚本依赖 `getParam("TEST_DIR")`，通常会走脚本内置 fallback 路径(例如从 `/bin/ls` 取 ELF)。

## 功能清单

详见: `common/yak/yaktest/mustpass/files-hids/README.md`

## 执行汇总

| Script | Status | Duration | Return | Notes |
|---|---:|---:|---|---|
| `connection_filter.yak` | PASS | `125.925021ms` | `*antlr4yak.Engine` |  |
| `connection_history.yak` | PASS | `5.042424317s` | `*antlr4yak.Engine` |  |
| `connection_list.yak` | PASS | `112.195861ms` | `*antlr4yak.Engine` |  |
| `elf_header.yak` | PASS | `1.052429ms` | `*antlr4yak.Engine` |  |
| `elf_misc.yak` | PASS | `1.693235ms` | `*antlr4yak.Engine` | output truncated |
| `elf_sections.yak` | PASS | `1.058366ms` | `*antlr4yak.Engine` |  |
| `elf_segments.yak` | PASS | `1.125269ms` | `*antlr4yak.Engine` |  |
| `file_hash.yak` | PASS | `1.113748ms` | `*antlr4yak.Engine` |  |
| `file_malicious_match.yak` | PASS | `3.636087ms` | `*antlr4yak.Engine` |  |
| `file_md5.yak` | PASS | `1.077415ms` | `*antlr4yak.Engine` |  |
| `file_scanner.yak` | PASS | `1.925062ms` | `*antlr4yak.Engine` |  |
| `file_type_extension.yak` | PASS | `2.243156ms` | `*antlr4yak.Engine` |  |
| `file_type_magic.yak` | PASS | `1.294362ms` | `*antlr4yak.Engine` |  |
| `filemonitor_access_log.yak` | PASS | `24.015957683s` | `*antlr4yak.Engine` |  |
| `filemonitor_config.yak` | PASS | `1.268171ms` | `*antlr4yak.Engine` |  |
| `filemonitor_config_change_detail.yak` | PASS | `2.002156429s` | `*antlr4yak.Engine` |  |
| `filemonitor_permission_check.yak` | PASS | `4.120643809s` | `*antlr4yak.Engine` |  |
| `hids_basic.yak` | PASS | `14.401821ms` | `*antlr4yak.Engine` |  |
| `hids_match.yak` | PASS | `6.444989ms` | `*antlr4yak.Engine` |  |
| `process_basic_info.yak` | PASS | `19.072667ms` | `*antlr4yak.Engine` |  |
| `process_create_event.yak` | PASS | `1.369338745s` | `*antlr4yak.Engine` |  |
| `process_create_info.yak` | PASS | `3.306369497s` | `*antlr4yak.Engine` |  |
| `process_exit_event.yak` | PASS | `1.449080875s` | `*antlr4yak.Engine` |  |
| `process_filter.yak` | PASS | `2.725495655s` | `*antlr4yak.Engine` |  |
| `process_list.yak` | PASS | `922.6747ms` | `*antlr4yak.Engine` |  |
| `process_parent_child.yak` | PASS | `69.453707ms` | `*antlr4yak.Engine` |  |
| `process_tree.yak` | PASS | `3.541763827s` | `*antlr4yak.Engine` |  |
| `process_whitelist.yak` | PASS | `7.16466ms` | `*antlr4yak.Engine` |  |
| `rule_engine_basic.yak` | PASS | `6.069894ms` | `*antlr4yak.Engine` |  |

## 逐脚本结果

### connection_filter.yak

- 来自功能清单:
  - 模块: 连接状态监控
  - 条目: 连接列表过滤
  - 描述: 支持按协议、端口、状态等条件过滤连接列表。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/connection_filter.yak`
  - 清单行号: 24
- 测试函数: Netstat, NewConnectionFilter
- 执行结果: PASS

输出(部分):

```text
Created connection filter
TCP filtered connections: 50
UDP filtered connections: 5
LISTEN filtered connections: 24
ESTABLISHED filtered connections: 26
PID 903637 filtered connections: 7
LocalPort 35155 filtered connections: 1
TCP+LISTEN filtered connections: 24
Empty result filter: 0 connections (expected 0 or few)

All connection filter tests passed!
```

### connection_history.yak

- 来自功能清单:
  - 模块: 连接状态监控
  - 条目: 连接状态历史记录
  - 描述: 记录连接状态变化的历史信息。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/connection_history.yak`
  - 清单行号: 25
- 测试函数: NewConnectionMonitor, WithConnectionHistory, GetHistory, ClearHistory, WatchConnections
- 执行结果: PASS

输出(部分):

```text
Connection monitor started with history enabled
Monitoring connections for 2 seconds...
History entries collected: 5
  History[0]: Type=new, Local=127.0.0.1:48476, Remote=127.0.0.1:39323, Time=1771992209
  History[1]: Type=new, Local=127.0.0.1:37066, Remote=127.0.0.1:33565, Time=1771992209
  History[2]: Type=disappear, Local=127.0.0.1:33565, Remote=127.0.0.1:37066, Time=1771992209
  History[3]: Type=disappear, Local=127.0.0.1:37066, Remote=127.0.0.1:33565, Time=1771992209
  History[4]: Type=disappear, Local=127.0.0.1:48476, Remote=127.0.0.1:39323, Time=1771992209
ClearHistory test passed (cleared 5 entries)
Continuing to collect history for 1 second...
New history entries after clear: 1
Monitor stopped

Using WatchConnections for 1 second...
WatchConnections captured 0 events
  New connections: 0
  Disappeared connections: 0

Testing callbacks with history...
Callbacks triggered - New: 0, Disappear: 0
History recorded: 0 entries

All connection history tests passed!
```

### connection_list.yak

- 来自功能清单:
  - 模块: 连接状态监控
  - 条目: 连接列表获取
  - 描述: 获取当前系统的网络连接列表。
  - 关联实现: `common/hids/connection_monitor.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/connection_list.yak`
  - 清单行号: 23
- 测试函数: Netstat, GetTCPConnections, GetUDPConnections, GetListeningPorts, GetEstablishedConnections, GetConnectionsByPid, GetConnectionsByPort, GetConnectionStats
- 执行结果: PASS

输出(部分):

```text
Total network connections: 135
Sample connection:
  Fd: 34
  Family: AF_INET
  Type: SOCK_STREAM
  LocalAddr: 127.0.0.1:35155
  LocalIP: 127.0.0.1
  LocalPort: 35155
  RemoteAddr: 0.0.0.0
  RemoteIP: 0.0.0.0
  RemotePort: 0
  Status: LISTEN
  Pid: 831133

TCP connections: 47
UDP connections: 5
Listening ports: 24
  Listen[0]: 127.0.0.1:35155 (PID=831133)
  Listen[1]: 127.0.0.1:18488 (PID=821495)
  Listen[2]: 127.0.0.1:35629 (PID=830697)
  Listen[3]: 127.0.0.54:53 (PID=0)
  Listen[4]: 10.255.255.254:53 (PID=0)

Established connections: 18

Connections for current PID 903637: 3

Connections for port 35155: 1

=== Connection Statistics ===
Total: 135
TCP: 120
UDP: 12
Listening: 24
By Status: map[ESTABLISHED:18 LISTEN:24 NONE:88 SYN_SENT:1 TIME_WAIT:4]
By Protocol: map[TCP:120 UDP:12]

All connection list tests passed!
```

### elf_header.yak

- 来自功能清单:
  - 模块: 文件格式模块
  - 条目: ELF文件头解析
  - 描述: 解析ELF文件头信息，包括魔数、架构类型和入口地址。
  - 关联实现: `common/yak/yaklib/elf.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/elf_header.yak`
  - 清单行号: 7
- 测试函数: IsELF, ReadELFHeader, GetELFArchitecture, GetELFEntryPoint, ParseELF
- 可能依赖参数: `TEST_DIR`
- 执行结果: PASS

输出(部分):

```text
ELF header: class=64-bit machine=EM_X86_64 (AMD x86-64) entry=0x0000000000001060
```

### elf_misc.yak

- 来自功能清单: (未找到对应条目)
- 可能依赖参数: `TEST_DIR`
- 执行结果: PASS

输出(部分):

```text
DisplayELF output: ELF Header:
  Magic:   7f 45 4c 46 02 01 01 00 00 00 00 00 00 00 00 00
  Class:                             64-bit
  Data:                              little-endian
  Version:                           0x1
  OS/ABI:                            0
  ABI Version:                       0
  Type:                              ET_DYN (Shared object file)
  Machine:                           EM_X86_64 (AMD x86-64)
  Entry point address:               0x0000000000001060

Program Headers:
  Type           Offset             VirtAddr           PhysAddr
                 FileSiz            MemSiz              Flags  Align
  PT_PHDR        0x0000000000000040 0x0000000000000040 0x0000000000000040
                 0x00000000000002d8 0x00000000000002d8  R     0x8
  PT_INTERP      0x0000000000000318 0x0000000000000318 0x0000000000000318
                 0x000000000000001c 0x000000000000001c  R     0x1
  PT_LOAD        0x0000000000000000 0x0000000000000000 0x0000000000000000
                 0x0000000000000628 0x0000000000000628  R     0x1000
  PT_LOAD        0x0000000000001000 0x0000000000001000 0x0000000000001000
                 0x0000000000000175 0x0000000000000175  RX    0x1000
  PT_LOAD        0x0000000000002000 0x0000000000002000 0x0000000000002000
                 0x00000000000000f4 0x00000000000000f4  R     0x1000
  PT_LOAD        0x0000000000002db8 0x0000000000003db8 0x0000000000003db8
                 0x0000000000000258 0x0000000000000260  RW    0x1000
  PT_DYNAMIC     0x0000000000002dc8 0x0000000000003dc8 0x0000000000003dc8
                 0x00000000000001f0 0x00000000000001f0  RW    0x8
  PT_NOTE        0x0000000000000338 0x0000000000000338 0x0000000000000338
                 0x0000000000000030 0x0000000000000030  R     0x8
  PT_NOTE        0x0000000000000368 0x0000000000000368 0x0000000000000368
                 0x0000000000000044 0x0000000000000044  R     0x4
  Unknown (0x50545f474e555f50524f5045525459) 0x0000000000000338 0x0000000000000338 0x0000000000000338
                 0x0000000000000030 0x0000000000000030  R     0x8
  PT_GNU_EH_FRAME 0x0000000000002014 0x0000000000002014 0x0000000000002014
                 0x0000000000000034 0x0000000000000034  R     0x4
  PT_GNU_STACK   0x0000000000000000 0x0000000000000000 0x0000000000000000
                 0x0000000000000000 0x0000000000000000  RW    0x10
  PT_GNU_RELRO   0x0000000000002db8 0x0000000000003db8 0x0000000000003db8
                 0x0000000000000248 0x0000000000000248  R     0x1

Section Headers:
  [Nr] Name              Type            Address          Off    Size   ES Flg Lk Inf Al
  [ 0]                  SHT_NULL        0000000000000000 000000 000000 00 -    0   0  0
  [ 1] .interp          SHT_PROGBITS    0000000000000318 000318 00001c 00 A    0   0  1
  [ 2] .note.gnu.property SHT_NOTE        0000000000000338 000338 000030 00 A    0   0  8
  [ 3] .note.gnu.build-id SHT_NOTE        0000000000000368 000368 000024 00 A    0   0  4
  [ 4] .note.ABI-tag    SHT_NOTE        000000000000038c 00038c 000020 00 A    0   0  4
  [ 5] .gnu.hash        Unknown (0x5348545f474e555f48415348) 00000000000003b0 0003b0 000024 00 A    6   0  8
  [ 6] .dynsym          SHT_DYNSYM      00000000000003d8 0003d8 0000a8 18 A    7   1  8
  [ 7] .dynstr          SHT_STRTAB      0000000000000480 000480 00008d 00 A    0   0  1
  [ 8] .gnu.version     Unknown (0x5348545f474e555f56455253594d) 000000000000050e 00050e 00000e 02 A    6   0  2
  [ 9] .gnu.version_r   Unknown (0x5348545f474e555f5645524e454544) 0000000000000520 000520 000030 00 A    7   1  8
  [10] .rela.dyn        SHT_RELA        0000000000000550 000550 0000c0 18 A    6   0  8
  [11] .rela.plt        SHT_RELA        0000000000000610 000610 000018 18 AI   6  24  8
  [12] .init            SHT_PROGBITS    0000000000001000 001000 00001b 00 AX   0   0  4
  [13] .plt             SHT_PROGBITS    0000000000001020 001020 000020 10 AX   0   0 16
  [14] .plt.got         SHT_PROGBITS    0000000000001040 001040 000010 10 AX   0   0 16
  [15] .plt.sec         SHT_PROGBITS    0000000000001050 001050 000010 10 AX   0   0 16
  [16] .text            SHT_PROGBITS    0000000000001060 001060 000107 00 AX   0   0 16
  [17] .fini            SHT_PROGBITS    0000000000001168 001168 00000d 00 AX   0   0  4
  [18] .rodata          SHT_PROGBITS    0000000000002000 002000 000012 00 A    0   0  4
  [19] .eh_frame_hdr    SHT_PROGBITS    0000000000002014 002014 000034 00 A    0   0  4
  [20] .eh_frame        SHT_PROGBITS    0000000000002048 002048 0000ac 00 A    0   0  8
  [21] .init_array      SHT_INIT_ARRAY  0000000000003db8 002db8 000008 08 WA   0   0  8
  [22] .fini_array      SHT_FINI_ARRAY  0000000000003dc0 002dc0 000008 08 WA   0   0  8
  [23] .dynamic         SHT_DYNAMIC     0000000000003dc8 002dc8 0001f0 10 WA   7   0  8
  [24] .got             SHT_PROGBITS    0000000000003fb8 002fb8 000048 08 WA   0   0  8
  [25] .data            SHT_PROGBITS    0000000000004000 003000 000010 00 WA   0   0  8
  [26] .bss             SHT_NOBITS      0000000000004010 003010 000008 00 WA   0   0  1
  [27] .comment         SHT_PROGBITS    0000000000000000 003010 00002b 01 MS   0   0  1
  [28] .symtab          SHT_SYMTAB      0000000000000000 003040 000360 18 -   29  18  8
  [29] .strtab          SHT_STRTAB      0000000000000000 0033a0 0001dc 00 -    0   0  1
  [30] .shstrtab        SHT_STRTAB      0000000000000000 00357c 00011a 00 -    0   0  1

Key to Flags:
  W (write), A (alloc), X (execute), M (merge), S (strings), I (info),
  L (link order), O (extra OS processing required), G (group), T (TLS),
  C (compressed), x (unknown), o (OS specific), E (exclude),
  l (large), p (processor specific)

DisplayELF from file path:  ELF Header:
  Magic:   7f 45 4c 46 02 01 01 00 00 00 00 00 00 00 00 00
  Class:                             64-bit
  Data:                              little-endian
  Version:                           0x1
  OS/ABI:                            0
  ABI Version:                       0
  Type:                              ET_DYN (Shared object file)
  Machine:                           EM_X86_64 (AMD x86-64)
  Entry point address:               0x0000000000001060

Program Headers:
  Type           Offset             VirtAddr           PhysAddr
                 FileSiz            MemSiz              Flags  Align
  PT_PHDR        0x0000000000000040 0x0000000000000040 0x0000000000000040
                 0x00000000000002d8 0x00000000000002d8  R     0x8
  PT_INTERP      0x0000000000000318 0x0000000000000318 0x0000000000000318
                 0x000000000000001c 0x000000000000001c  R     0x1
  PT_LOAD        0x0000000000000000 0x0000000000000000 0x0000000000000000
                 0x0000000000000628 0x0000000000000628  R     0x1000
  PT_LOAD        0x0000000000001000 0x0000000000001000 0x0000000000001000
                 0x0000000000000175 0x0000000000000175  RX    0x1000
  PT_LOAD        0x0000000000002000 0x0000000000002000 0x0000000000002000
                 0x00000000000000f4 0x00000000000000f4  R     0x1000
  PT_LOAD        0x0000000000002db8 0x0000000000003db8 0x0000000000003db8
                 0x0000000000000258 0x0000000000000260  RW    0x1000
  PT_DYNAMIC     0x0000000000002dc8 0x0000000000003dc8 0x0000000000003dc8
                 0x00000000000001f0 0x00000000000001f0  RW    0x8
  PT_NOTE        0x0000000000000338 0x0000000000000338 0x0000000000000338
                 0x0000000000000030 0x0000000000000030  R     0x8
  PT_NOTE        0x0000000000000368 0x0000000000000368 0x0000000000000368
                 0x0000000000000044 0x0000000000000044  R     0x4
  Unknown (0x50545f474e555f50524f5045525459) 0x0000000000000338 0x0000000000000338 0x0000000000000338
                 0x0000000000000030 0x0000000000000030  R     0x8
  PT_GNU_EH_FRAME 0x0000000000002014 0x0000000000002014 0x0000000000002014
                 0x0000000000000034 0x0000000000000034  R     0x4
  PT_GNU_STACK   0x0000000000000000 0x0000000000000000 0x0000000000000000
                 0x0000000000000000 0x0000000000000000  RW    0x10
  PT_GNU_RELRO   0x0000000000002db8 0x0000000000003db8 0x0000000000003db8
                 0x0000000000000248 0x0000000000000248  R     0x1

Section Headers:
  [Nr] Name              Type            Address          Off    Size   ES Flg Lk Inf Al
  [ 0]                  SHT_NULL        0000000000000000 000000 000000 00 -    0   0  0
  [ 1] .interp          SHT_PROGBITS    0000000000000318 000318 00001c 00 A    0   0  1
  [ 2] .note.gnu.property SHT_NOTE        0000000000000338 000338 000030 00 A    0   0  8
  [ 3] .note.gnu.build-id SHT_NOTE        0000000000000368 000368 000024 00 A    0   0  4
  [ 4] .note.ABI-tag    SHT_NOTE        000000000000038c 00038c 000020 00 A    0   0  4
  [ 5] .gnu.hash        Unknown (0x5348545f474e555f48415348) 00000000000003b0 0003b0 000024 00 A    6   0  8
  [ 6] .dynsym          SHT_DYNSYM      00000000000003d8 0003d8 0000a8 18 A    7   1  8
  [ 7] .dynstr          SHT_STRTAB      0000000000000480 000480 00008d 00 A    0   0  1
  [ 8] .gnu.version     Unknown (0x5348545f474e555f56455253594d) 000000000000050e 00050e 00000e 02 A    6   0  2
  [ 9] .gnu.version_r   Unknown (0x5348545f474e555f5645524e454544) 0000000000000520 000520 000030 00 A    7   1  8
  [10] .rela.dyn        SHT_RELA        0000000000000550 000550 0000c0 18 A    6   0  8
  [11] .rela.plt        SHT_RELA        0000000000000610 000610 000018 18 AI   6  24  8
  [12] .init            SHT_PROGBITS    0000000000001000 001000 00001b 00 AX   0   0  4
  [13] .plt             SHT_PROGBITS    0000000000001020 001020 000020 10 AX   0   0 16
  [14] .plt.got         SHT_PROGBITS    0000000000001040 001040 000010 10 AX   0   0 16
  [15] .plt.sec         SHT_PROGBITS    0000000000001050 001050 000010 10 AX   0   0 16
  [16] .text            SHT_PROGBITS    0000000000001060 001060 000107 00 AX   0   0 16
  [17] .fini            SHT_PROGBITS    0000000000001168 001168 00000d 00 AX   0   0  4
  [18] .rodata          SHT_PROGBITS    0000000000002000 002000 000012 00 A    0   0  4
  [19] .eh_frame_hdr    SHT_PROGBITS    0000000000002014 002014 000034 00 A    0   0  4
  [20] .eh_frame        SHT_PROGBITS    0000000000002048 002048 0000ac 00 A    0   0  8
  [21] .init_array      SHT_INIT_AR
...(truncated)
```

### elf_sections.yak

- 来自功能清单:
  - 模块: 文件格式模块
  - 条目: ELF节信息提取
  - 描述: 提取ELF文件的节（Section）信息，包括符号表和字符串表。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/elf_sections.yak`
  - 清单行号: 9
- 测试函数: ReadELFSections, ParseELF
- 可能依赖参数: `TEST_DIR`
- 执行结果: PASS

输出(部分):

```text
ELF sections: total=31 symtabs=2 strtabs=3
```

### elf_segments.yak

- 来自功能清单:
  - 模块: 文件格式模块
  - 条目: ELF段信息提取
  - 描述: 提取ELF文件的段（Segment）信息，包括代码段和数据段。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/elf_segments.yak`
  - 清单行号: 8
- 测试函数: ReadELFSegments, ParseELF
- 可能依赖参数: `TEST_DIR`
- 执行结果: PASS

输出(部分):

```text
ELF segments: total=13 PT_LOAD=true code=1 data=3
```

### file_hash.yak

- 来自功能清单:
  - 模块: 文件变化监控
  - 条目: 文件哈希计算
  - 描述: 计算文件的哈希值用于完整性校验。
  - 关联实现: `common/yak/yaklib/file.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/file_hash.yak`
  - 清单行号: 42
- 执行结果: PASS

输出(部分):

```text
MD5 hash: 7f2ae341186f06563dc8bebe590ac947
SHA1 hash: a31ef409403ca79b6666cb09bc1dece5ace876ae
SHA256 hash: 99e4e2180a9fcf2bfb90634d20b01d13744adeb44a229d32fcbb98a299c53f7f
File hash calculation tests passed!
```

### file_malicious_match.yak

- 来自功能清单:
  - 模块: 文件变化监控
  - 条目: 内容特征匹配
  - 描述: 基于特征库匹配文件内容特征。
  - 关联实现: `common/yak/yaklib/file_malicious_signatures.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/file_malicious_match.yak`
  - 清单行号: 45
- 执行结果: PASS

输出(部分):

```text
=== Testing PHP WebShell detection (file path) ===
PHP WebShell matches: [php_webshell_system php_webshell_eval php_webshell_base64_decode python_eval_exec]
=== Testing PHP WebShell detection (content) ===
Content matches: [python_eval_exec php_webshell_eval php_webshell_base64_decode php_webshell_system]
=== Testing privilege escalation detection ===
Privilege escalation matches: [privilege_escalation_whoami privilege_escalation_chmod_777 privilege_escalation_passwd_access privilege_escalation_sudo]
=== Testing backdoor detection ===
Backdoor matches: [backdoor_hidden_eval backdoor_login_bypass python_eval_exec php_webshell_eval backdoor_password_hardcoded]
=== Testing Python malicious script detection ===
Python malicious matches: [python_os_system php_webshell_system privilege_escalation_whoami php_webshell_eval python_subprocess python_eval_exec]
=== Testing Shell malicious script detection ===
Shell malicious matches: [shell_bash_reverse shell_wget_pipe]
=== Testing normal file (should not match) ===
Normal file matches: [] (should be empty or minimal)
=== Testing detailed matching (file path) ===
Detail: php_webshell_eval, Category: php_webshell, Severity: high
Detail: python_eval_exec, Category: python_malicious, Severity: high
Detail: php_webshell_base64_decode, Category: php_webshell, Severity: medium
Detail: php_webshell_system, Category: php_webshell, Severity: critical
=== Testing detailed matching (content) ===
=== Testing custom matcher ===
[[custom_suspicious_pattern] <nil>]
Custom matcher matches: [[custom_suspicious_pattern] <nil>]
=== Testing get signatures by category ===
PHP WebShell signatures count: 13
Privilege escalation signatures count: 5
=== Testing get all categories ===
All categories: [privilege_escalation backdoor python_malicious shell_malicious custom php_webshell]
File malicious match tests passed!
```

### file_md5.yak

- 来自功能清单:
  - 模块: 工具模块
  - 条目: MD5哈希计算
  - 描述: 使用MD5算法计算文件哈希值。
  - 关联实现: `common/yak/yaklib/file.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/file_md5.yak`
  - 清单行号: 10
- 可能依赖参数: `TEST_DIR`
- 执行结果: PASS

输出(部分):

```text
=== MD5 sample ===
file=hello.elf
md5(file)=d1b598d24de6ac00c59658dcaf931485 md5(codec)=d1b598d24de6ac00c59658dcaf931485
md5(empty)=d41d8cd98f00b204e9800998ecf8427e
md5(same content)=155907e50dc2e2bc0acd30750ca3dbc4
md5(different content)=0f27b3a523b957612bb947878cbf373e
All MD5 hash calculation tests passed!
```

### file_scanner.yak

- 来自功能清单:
  - 模块: 文件扫描
  - 条目: 单文件扫描
  - 描述: 支持扫描单个文件，提取文件特征进行匹配。
  - 关联实现: `common/yak/yaklib/filescanner.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/file_scanner.yak`
  - 清单行号: 59
- 来自功能清单:
  - 模块: 文件扫描
  - 条目: 扫描结果处理
  - 描述: 处理文件扫描结果，包括匹配规则和告警生成。
  - 关联实现: `common/yak/yaklib/filescanner.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/file_scanner.yak`
  - 清单行号: 60
- 来自功能清单:
  - 模块: 文件扫描
  - 条目: 批量文件扫描
  - 描述: 对目录中的文件进行批量扫描处理。
  - 关联实现: `common/yak/yaklib/filescanner.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/file_scanner.yak`
  - 清单行号: 61
- 执行结果: PASS

输出(部分):

```text
=== ScanFile (malicious sample) ===
name=webshell.php ext=.php size=52 mime=text/x-php matched=true
md5=8b32456d0928c9709f2f775c75fd56d1 match_names=[python_eval_exec php_webshell_eval php_webshell_system]
=== ScanFile (normal sample) ===
name=normal.php ext=.php size=28 mime=text/x-php skipped=false matched=false
=== ScanDir summary ===
total=3 matched=1 skipped=0
webshell.php: matched=true skipped=false mime=text/x-php md5=8b32456d0928c9709f2f775c75fd56d1
normal.php: matched=false skipped=false mime=text/x-php md5=9d0c6054b625425d41e561ee8ccf6975
note.txt: matched=false skipped=false mime=text/plain; charset=utf-8 md5=20a6adb3ed692b77400559a4702671fd
=== Exclude pattern sample ===
name=normal.php skipped=true reason=excluded
File scanner tests passed!
```

### file_type_extension.yak

- 来自功能清单:
  - 模块: 工具模块
  - 条目: 文件扩展名匹配
  - 描述: 通过文件扩展名匹配文件类型。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/file_type_extension.yak`
  - 清单行号: 12
- 执行结果: PASS

输出(部分):

```text
=== Extension -> MIME (sample) ===
.txt -> text/plain; charset=utf-8
jpg -> image/jpeg
.jpg -> image/jpeg
.png -> image/png
.pdf -> application/pdf
.html -> text/html; charset=utf-8
.css -> text/css; charset=utf-8
.js -> text/javascript; charset=utf-8
.json -> application/json
.xml -> text/xml; charset=utf-8
.zip -> application/zip
unknown -> application/octet-stream
empty ext -> application/octet-stream
.TXT -> text/plain; charset=utf-8
DetectFileType(test_extension_3262329073.json) -> application/json
All file extension matching tests passed!
```

### file_type_magic.yak

- 来自功能清单:
  - 模块: 工具模块
  - 条目: 文件类型识别
  - 描述: 通过文件头魔数识别文件类型。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/file_type_magic.yak`
  - 清单行号: 11
- 可能依赖参数: `TEST_DIR`
- 执行结果: PASS

输出(部分):

```text
=== MIME detection sample ===
file=hello.elf
mime(from file)=application/x-sharedlib
mime(from raw)=application/x-sharedlib
mime(text/plain)=text/plain; charset=utf-8
mime(elf)=application/x-sharedlib
mime(png raw)=image/png
All file type detection (magic number) tests passed!
```

### filemonitor_access_log.yak

- 来自功能清单:
  - 模块: 文件变化监控
  - 条目: 文件访问记录
  - 描述: 记录用户对文件的访问操作，包括读取、写入等。
  - 关联实现: `common/yak/yaklib/filemonitor.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/filemonitor_access_log.yak`
  - 清单行号: 35
- 来自功能清单:
  - 模块: 文件变化监控
  - 条目: 文件操作事件捕获
  - 描述: 捕获指定重要文件的读取、修改或删除操作。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/filemonitor_access_log.yak`
  - 清单行号: 41
- 来自功能清单:
  - 模块: 文件变化监控
  - 条目: 文件访问日志记录
  - 描述: 记录访问时间、用户和操作类型等详细信息。
  - 关联实现: `common/yak/yaklib/filemonitor.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/filemonitor_access_log.yak`
  - 清单行号: 43
- 执行结果: PASS

输出(部分):

```text
=== Testing CREATE operation ===
[CREATE] Path: /tmp/file_access_log_test_1771992214/create_test.txt, User: go0p, UID: 1000, GID: 1000, Mode: -rwxr-xr-x, Size: 19, IsDir: false, Timestamp: 1771992216
[CREATE] Path: /tmp/file_access_log_test_1771992214/write_test.txt, User: go0p, UID: 1000, GID: 1000, Mode: -rwxr-xr-x, Size: 15, IsDir: false, Timestamp: 1771992218
Create logs: 2
=== Testing WRITE operation ===
[WRITE] Path: /tmp/file_access_log_test_1771992214/write_test.txt, User: go0p, UID: 1000, GID: 1000, Mode: -rwxr-xr-x, Size: 16, IsDir: false, Timestamp: 1771992221
Write logs: 1
=== Testing DELETE operation ===
[DELETE] Path: /tmp/file_access_log_test_1771992214/delete_test.txt, User: go0p, UID: 1000, GID: 1000, Mode: -rwxr-xr-x, Size: 19, IsDir: false, Timestamp: 1771992226
Delete logs: 1
[SIZE_TEST] Path: /tmp/file_access_log_test_1771992214/size_test.txt, Operation: write, Size: 15 bytes, Timestamp: 1771992232
[SIZE_TEST] Path: /tmp/file_access_log_test_1771992214/size_test.txt, Operation: write, Size: 1000 bytes, Timestamp: 1771992233
File size recorded: 15 bytes
File size recorded: 1000 bytes
Logs with user info: 4
User: go0p, UID: 1000, GID: 1000
User: go0p, UID: 1000, GID: 1000
User: go0p, UID: 1000, GID: 1000
User: go0p, UID: 1000, GID: 1000
[DIR_CREATE] Path: /tmp/file_access_log_test_1771992214/new_dir, Operation: create, User: go0p, UID: 1000, GID: 1000, Mode: drwxr-xr-x, IsDir: true, Timestamp: 1771992236
Directory log: /tmp/file_access_log_test_1771992214/new_dir
=== Testing READ operation ===
[READ] Path: /tmp/file_access_log_test_1771992214/read_test.txt, User: go0p, UID: 1000, GID: 1000, Mode: -rwxr-xr-x, Size: 17, IsDir: false, Timestamp: 1771992238
File access log tests passed!
```

### filemonitor_config.yak

- 来自功能清单:
  - 模块: 系统配置审计
  - 条目: 配置文件监控
  - 描述: 监控系统关键配置文件的变更。
  - 关联实现: `common/yak/yaklib/filemonitor.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/filemonitor_config.yak`
  - 清单行号: 38
- 来自功能清单:
  - 模块: 文件变化监控
  - 条目: 文件监控规则配置
  - 描述: 配置需要监控的重要文件列表和监控规则。
  - 关联实现: `common/yak/yaklib/filemonitor.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/filemonitor_config.yak`
  - 清单行号: 40
- 执行结果: PASS

输出(部分):

```text
=== File monitor config (sample) ===
watch_paths=1 recursive=true max_file_size=10485760 include=2 exclude=1 ops=5
monitor_ops=[create write delete chmod chown]
File monitor config tests passed!
```

### filemonitor_config_change_detail.yak

- 来自功能清单:
  - 模块: 系统配置审计
  - 条目: 配置文件监控
  - 描述: 监控系统关键配置文件的变更。
  - 关联实现: `common/yak/yaklib/filemonitor.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/filemonitor_config_change_detail.yak`
  - 清单行号: 38
- 来自功能清单:
  - 模块: 系统配置审计
  - 条目: 配置变更详情记录
  - 描述: 记录修改内容、时间和操作用户等详细信息。
  - 关联实现: `common/yak/yaklib/filemonitor.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/filemonitor_config_change_detail.yak`
  - 清单行号: 39
- 执行结果: PASS

输出(部分):

```text
[CONFIG_WRITE] Path: /tmp/file_config_change_test_1771992239/test.conf, OldLen: 19, NewLen: 36, Time: 2026-02-25 12:04:00.172570987 +0800 CST m=+31.657310013
Captured 1 config write events (oldLen=19 newLen=36)
Config change detail tests passed!
```

### filemonitor_permission_check.yak

- 来自功能清单:
  - 模块: 文件变化监控
  - 条目: 权限变更事件捕获
  - 描述: 捕获用户权限变更操作事件。
  - 关联实现: `common/yak/yaklib/filemonitor.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/filemonitor_permission_check.yak`
  - 清单行号: 36
- 来自功能清单:
  - 模块: 文件变化监控
  - 条目: 权限变更日志
  - 描述: 记录权限变更的详细信息，包括变更前后权限对比。
  - 关联实现: `common/yak/yaklib/filemonitor.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/filemonitor_permission_check.yak`
  - 清单行号: 37
- 来自功能清单:
  - 模块: 文件变化监控
  - 条目: 权限变更事件捕获
  - 描述: 监测文件权限或属主的变更事件。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/filemonitor_permission_check.yak`
  - 清单行号: 44
- 执行结果: PASS

输出(部分):

```text
Current file mode: -rw------- (0600)
Attempting to change file mode from 0600 to 0700
File mode after chmod: -rwx------ (0700)
Total permission events captured: 3
Event 0: Type=chmod, Path=/home/go0p/yakit-projects/temp/file_permission_test_2225427564.txt
Event 1: Type=chmod, Path=/home/go0p/yakit-projects/temp/file_permission_test_2857300234.txt
Event 2: Type=chmod, Path=/home/go0p/yakit-projects/temp/file_permission_test_2225427564.txt
Permission changed from -rw------- to -rwx------
File mode recorded: -rw-------
File permission tests passed!
```

### hids_basic.yak

- 来自功能清单:
  - 模块: 网络行为识别
  - 条目: 数据泄露告警
  - 描述: 对检测到的数据泄露行为进行告警处理。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/hids_basic.yak`
  - 清单行号: 27
- 执行结果: PASS

输出(部分):

```text
========================================
HIDS/NIDS 综合测试开始
========================================
[TEST] Suricata 规则解析基础测试...
[PASS] 基础规则解析测试通过
[TEST] 多规则批量解析测试...
[PASS] 多规则批量解析测试通过
[TEST] 规则匹配器创建测试...
[PASS] 规则匹配器创建测试通过
[TEST] 协议识别测试...
[PASS] 协议识别测试通过
[TEST] 内容匹配关键字测试...
[PASS] 内容匹配关键字测试通过
[TEST] 流量分类规则测试...
[PASS] 流量分类规则测试通过
[TEST] 规则组匹配器回调测试...
[PASS] 规则组匹配器回调测试通过
[TEST] 环境变量支持测试...
[PASS] 环境变量支持测试通过
[TEST] 高级规则选项测试...
[PASS] 高级规则选项测试通过
[TEST] 规则存储和查询功能测试...
[PASS] 规则存储和查询功能测试通过
[CLEANUP] 清理测试规则...
[CLEANUP] 测试规则清理完成
========================================
所有 HIDS/NIDS 测试通过!
========================================
```

### hids_match.yak

- 来自功能清单:
  - 模块: 网络行为识别
  - 条目: 通信协议识别
  - 描述: 识别网络通信使用的协议类型。
  - 关联实现: `common/suricata/`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/hids_match.yak`
  - 清单行号: 26
- 来自功能清单:
  - 模块: 网络行为识别
  - 条目: 攻击特征库
  - 描述: 维护网络攻击流量特征库。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/hids_match.yak`
  - 清单行号: 28
- 来自功能清单:
  - 模块: 网络行为识别
  - 条目: 攻击流量识别
  - 描述: 识别网络攻击流量特征并告警。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/hids_match.yak`
  - 清单行号: 29
- 来自功能清单:
  - 模块: 网络行为识别
  - 条目: 流量分类规则
  - 描述: 定义网络流量的分类规则。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/hids_match.yak`
  - 清单行号: 30
- 来自功能清单:
  - 模块: 网络行为识别
  - 条目: 流量自动分类
  - 描述: 基于规则对网络流量进行自动分类。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/hids_match.yak`
  - 清单行号: 31
- 执行结果: PASS

输出(部分):

```text
========================================
NIDS 网络流量检测测试开始
========================================
[TEST] SQL 注入攻击检测...
[PASS] SQL 注入攻击检测测试通过
[TEST] XSS 攻击检测...
[PASS] XSS 攻击检测测试通过
[TEST] UDP DNS 查询内容检测...
[PASS] UDP DNS 查询内容检测测试通过
[TEST] ICMP Ping 扫描检测...
[PASS] ICMP Ping 扫描检测测试通过
[TEST] 数据泄露检测...
[PASS] 数据泄露检测测试通过
[TEST] 流量分类测试...
[PASS] 流量分类测试通过
[TEST] 规则元数据解析测试...
[PASS] 规则元数据解析测试通过
[TEST] 多协议内容匹配测试...
[PASS] 多协议内容匹配测试通过
[TEST] 偏移和深度匹配测试...
[PASS] 偏移和深度匹配测试通过
[TEST] IP 地址和端口匹配测试...
[PASS] IP 地址和端口匹配测试通过
[TEST] 命令注入检测...
[PASS] 命令注入检测测试通过
[TEST] 目录遍历攻击检测...
[PASS] 目录遍历攻击检测测试通过
========================================
所有 NIDS 测试通过!
========================================
```

### process_basic_info.yak

- 来自功能清单:
  - 模块: 进程信息收集
  - 条目: 进程基本信息查看
  - 描述: 查看进程的基本信息，包括PID、PPID、用户等。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/process_basic_info.yak`
  - 清单行号: 16
- 测试函数: GetProcessByPid, GetCurrentProcessInfo
- 执行结果: PASS

输出(部分):

```text
=== Process Basic Info ===
PID: 903637
PPID: 902954
Name: mustpass.test
Username: go0p
Exe: /tmp/go-build2843009527/b001/mustpass.test
Cmdline: /tmp/go-build2843009527/b001/mustpass.test -test.paniconexit0 -test.run=TestGenerateHIDSMustpassReport -test.count=1 -test.timeout=30m0s
Cwd: /home/go0p/code/go/yaklang/common/yak/yaktest/mustpass
Status: running
CreateTime: 1771992208500
CPUPercent: 7.58%
MemPercent: 0.56%
NumThreads: 27
IsRunning: true
Nice: 20
NumFds: 23

GetProcessByPid verified: PID=903637, Name=mustpass.test
Non-existent PID error handling test passed

Parent Process Info:
  PID: 902954
  Name: go
  Username: go0p

All process basic info tests passed!
```

### process_create_event.yak

- 来自功能清单:
  - 模块: 进程行为监控
  - 条目: 进程启动事件捕获
  - 描述: 捕获新进程的启动事件。
  - 关联实现: `common/hids/process_monitor.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/process_create_event.yak`
  - 清单行号: 19
- 测试函数: NewProcessMonitor, WithOnProcessCreate, WatchProcess
- 执行结果: PASS

输出(部分):

```text
Process monitor started
Monitor stopped. Captured 0 create events
Monitor configuration test passed

All process create event tests passed!
```

### process_create_info.yak

- 来自功能清单:
  - 模块: 进程行为监控
  - 条目: 进程启动信息记录
  - 描述: 记录进程启动的执行文件路径和启动用户信息。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/process_create_info.yak`
  - 清单行号: 20
- 测试函数: NewProcessMonitor, WithOnProcessCreate, GetFileHashMD5, GetFileHashSHA256
- 执行结果: PASS

输出(部分):

```text
Monitor started, recording process creation info...
Captured 0 process creations

=== Executable File Info ===
Exe Path: /tmp/go-build2843009527/b001/mustpass.test
MD5: cc97ca7b841a66c82e0b398e21e10adf
SHA256: 90ae21e3f0df2e143ef1ed5b30726065a766ffce22e99b14307b51d19d901a1e

Hash error handling test passed

All process create info tests passed!
```

### process_exit_event.yak

- 来自功能清单:
  - 模块: 进程行为监控
  - 条目: 进程退出事件捕获
  - 描述: 捕获进程的退出事件。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/process_exit_event.yak`
  - 清单行号: 21
- 测试函数: NewProcessMonitor, WithOnProcessExit, WatchProcess
- 执行结果: PASS

输出(部分):

```text
Process monitor started
Monitor stopped. Captured 0 exit events
Combined monitor configuration test passed

All process exit event tests passed!
```

### process_filter.yak

- 来自功能清单:
  - 模块: 进程信息收集
  - 条目: 进程列表过滤
  - 描述: 支持按用户、状态等条件过滤进程列表。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/process_filter.yak`
  - 清单行号: 15
- 测试函数: PS, NewProcessFilter
- 执行结果: PASS

输出(部分):

```text
Current process: PID=903637, Name=mustpass.test, User=go0p
PID filter test passed: found process 903637
Name filter test passed: found 2 processes with name 'mustpass.test'
Empty result filter test passed
Filter properties can be set: Username=test, Status=running
All process filter tests passed!
```

### process_list.yak

- 来自功能清单:
  - 模块: 进程信息收集
  - 条目: 进程列表获取
  - 描述: 获取当前系统中所有运行进程的列表。
  - 关联实现: `common/hids/process_info.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/process_list.yak`
  - 清单行号: 14
- 测试函数: PS, GetCurrentProcessInfo, ProcessExists
- 执行结果: PASS

输出(部分):

```text
Total processes in system: 163
Process[0]: PID=1, Name=systemd, User=root
Process[1]: PID=2, Name=init-systemd(Ub, User=root
Process[2]: PID=6, Name=init, User=root
Process[3]: PID=55, Name=systemd-journald, User=root
Process[4]: PID=86, Name=systemd-udevd, User=root
Current process: PID=903637, Name=mustpass.test
All process list tests passed!
```

### process_parent_child.yak

- 来自功能清单:
  - 模块: 进程信息收集
  - 条目: 进程父子关系识别
  - 描述: 识别进程之间的父子关系。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/process_parent_child.yak`
  - 清单行号: 17
- 测试函数: GetProcessParent, GetProcessChildren, GetProcessAncestors
- 执行结果: PASS

输出(部分):

```text
Current process: PID=903637, Name=mustpass.test, PPID=902954
Parent process: PID=902954, Name=go
Parent-child relationship verified: Parent 902954 has child 903637
Current process has 0 children
Ancestor chain (from parent to root): 7 processes
  Ancestor[0]: PID=902954, Name=go, PPID=218102
  Ancestor[1]: PID=218102, Name=opencode, PPID=213236
  Ancestor[2]: PID=213236, Name=zsh, PPID=213234
  Ancestor[3]: PID=213234, Name=Relay(213236), PPID=213233
  Ancestor[4]: PID=213233, Name=SessionLeader, PPID=2
  Ancestor[5]: PID=2, Name=init-systemd(Ub, PPID=1
  Ancestor[6]: PID=1, Name=systemd, PPID=0
Ancestor chain verification passed: first ancestor is parent
Ancestor chain continuity verified
Non-existent PID handling test passed

All process parent-child tests passed!
```

### process_tree.yak

- 来自功能清单:
  - 模块: 进程信息收集
  - 条目: 进程依赖关系识别
  - 描述: 识别进程之间的依赖关系，构建进程依赖图。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/process_tree.yak`
  - 清单行号: 18
- 测试函数: GetProcessTree
- 执行结果: PASS

输出(部分):

```text
Current process: PID=903637, Name=mustpass.test, PPID=902954

=== Process Tree (root: PPID=902954) ===
Root: PID=902954, Name=go
  Found current process in tree: PID=903637, Name=mustpass.test
Tree root has 1 direct children

=== Process Tree (root: current PID=903637) ===
Root: PID=903637, Name=mustpass.test
Direct children: 0
Total descendants: 0

=== Tree Structure (max depth 3) ===
PID=903637, Name=mustpass.test, Children=0

=== System Process Tree (root: PID=1) ===
System root: PID=1, Name=systemd
Direct children of init: 33

Non-existent PID handling test passed

All process tree tests passed!
```

### process_whitelist.yak

- 来自功能清单:
  - 模块: 进程行为监控
  - 条目: 白名单规则配置
  - 描述: 配置进程白名单规则，支持路径、哈希值等多种匹配方式。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/process_whitelist.yak`
  - 清单行号: 22
- 测试函数: NewWhitelistRule, NewProcessMonitor, WithWhitelist, IsWhitelisted, AddWhitelistRule, ClearWhitelist
- 执行结果: PASS

输出(部分):

```text
Current process: PID=903637, Name=mustpass.test, Exe=/tmp/go-build2843009527/b001/mustpass.test
Created rule with Name: mustpass.test
Name-based whitelist test passed
Username-based whitelist test passed
ExePath-based whitelist test passed
NamePattern-based whitelist test passed
ExePattern-based whitelist test passed
Non-matching rule test passed
Multiple rules test passed
ClearWhitelist test passed
WithWhitelist option test passed

All process whitelist tests passed!
```

### rule_engine_basic.yak

- 来自功能清单:
  - 模块: 规则配置
  - 条目: 条件表达式定义
  - 描述: 定义规则触发条件的表达式语法。
  - 关联实现: `common/yak/sandbox.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/rule_engine_basic.yak`
  - 清单行号: 46
- 来自功能清单:
  - 模块: 规则配置
  - 条目: 条件规则配置
  - 描述: 配置具体的规则触发条件。
  - 关联实现: `common/yak/sandbox.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/rule_engine_basic.yak`
  - 清单行号: 47
- 来自功能清单:
  - 模块: 规则配置
  - 条目: 动作参数配置
  - 描述: 配置规则动作的具体参数和执行方式。
  - 关联实现: `common/yak/sandbox.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/rule_engine_basic.yak`
  - 清单行号: 48
- 来自功能清单:
  - 模块: 规则配置
  - 条目: 规则启用执行
  - 描述: 启用指定规则，使其开始监控事件并触发相应动作。
  - 关联实现: `common/yak/sandbox.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/rule_engine_basic.yak`
  - 清单行号: 49
- 来自功能清单:
  - 模块: 规则配置
  - 条目: 规则禁用执行
  - 描述: 禁用指定规则，暂停其监控和触发动作。
  - 关联实现: `common/yak/sandbox.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/rule_engine_basic.yak`
  - 清单行号: 50
- 来自功能清单:
  - 模块: 规则执行
  - 条目: 事件匹配引擎
  - 描述: 实时检测系统事件是否满足规则条件。
  - 关联实现: `common/yak/sandbox.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/rule_engine_basic.yak`
  - 清单行号: 51
- 来自功能清单:
  - 模块: 规则执行
  - 条目: 日志记录功能
  - 描述: 记录规则触发的时间、条件和执行结果。
  - 关联实现: `common/yak/sandbox.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/rule_engine_basic.yak`
  - 清单行号: 52
- 来自功能清单:
  - 模块: 规则编写
  - 条目: 规则语法定义
  - 描述: 定义使用类C语法编写规则的语法规范。
  - 关联实现: `common/yak/sandbox.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/rule_engine_basic.yak`
  - 清单行号: 53
- 来自功能清单:
  - 模块: 规则编写
  - 条目: 规则结构定义
  - 描述: 定义规则的结构，包括规则标识符、元数据、字符串和条件部分。
  - 关联实现: `common/yak/sandbox.go`
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/rule_engine_basic.yak`
  - 清单行号: 54
- 来自功能清单:
  - 模块: 规则编写
  - 条目: 文本字符串匹配
  - 描述: 支持文本字符串的匹配功能。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/rule_engine_basic.yak`
  - 清单行号: 55
- 来自功能清单:
  - 模块: 规则编写
  - 条目: 正则表达式匹配
  - 描述: 支持正则表达式的匹配功能。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/rule_engine_basic.yak`
  - 清单行号: 56
- 来自功能清单:
  - 模块: 规则编写
  - 条目: 元数据定义
  - 描述: 定义元数据的结构和格式规范。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/rule_engine_basic.yak`
  - 清单行号: 57
- 来自功能清单:
  - 模块: 规则编写
  - 条目: 元数据解析
  - 描述: 解析规则中的描述性元数据信息。
  - 脚本路径: `common/yak/yaktest/mustpass/files-hids/rule_engine_basic.yak`
  - 清单行号: 58
- 执行结果: PASS

输出(部分):

```text
Sandbox rule tests passed!
```

## 备注

- HIDS mustpass 在 `common/yak/yaktest/mustpass/mustpass_base_test.go` 中明确**不并行**执行，避免文件系统监控/临时目录冲突。
- `TEST_DIR` 来自 `common/yak/yaktest/mustpass/test-hids/`，运行时会复制到系统临时目录。
- 连接/进程相关脚本依赖宿主 OS 能够枚举进程与连接信息；在受限容器环境下可能失败。
