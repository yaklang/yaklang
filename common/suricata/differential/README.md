# Suricata differential audit

This package compares Yaklang's stateful Suricata engine with official Suricata
and, for their shared rule dialect, Snort 3. It only replays offline PCAP files.
It never opens a live capture device or injects traffic.

The built-in matrix creates deterministic PCAP files for rule semantics, TCP
stream reconstruction, alert counts, and malformed direction assumptions:

```sh
go run ./common/suricata/cmd/suricata-diff \
  -work-dir /tmp/yak-suricata-audit
```

The command exits with 0 when all comparable results are equal, 1 when an alert
set or count differs, and 2 when orchestration fails. Output is JSON. Each case
contains the rules and PCAP SHA-256, per-engine alerts, Suricata rule-load
statistics, local diagnostics, and the normalized comparison.

A custom rules/PCAP pair can be compared with:

```sh
go run ./common/suricata/cmd/suricata-diff \
  -rules /path/to/rules.rules \
  -pcap /path/to/capture.pcap
```

By default the command discovers `suricata` and `snort` from `PATH`, and uses the
Homebrew or system Suricata configuration. Override them with `-suricata`,
`-suricata-config`, and `-snort`.

Important interpretation rules:

- A rule rejected by an external engine is a runner failure, not a zero-alert
  result.
- Yaklang's implicit GID 0 is normalized to Suricata's default GID 1.
- SID counts are compared as a multiset. Equal SID sets with different counts
  are still a difference.
- Snort is only included in cases that use a confirmed shared rule dialect.
- External engines run in fresh temporary log directories. Snort uses `-k none`
  so capture-offload checksums do not create false negatives.
