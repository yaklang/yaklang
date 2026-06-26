# Yak Java Decompiler — Cross-Comparison Report (machine-generated)

- Generated: 2026-06-26T16:45:03Z
- Host: 10 CPUs, Go go1.22.12
- Java: openjdk version "17.0.12" 2024-07-16 LTS
- CFR: CFR 0.152
- Vineflower: === Vineflower Decompiler 1.10.1 ===
- Yak workers: 10

## Axis 3 — Performance (wall-clock, lower is better)

| Jar | classes | yak-serial | yak-concurrent | cfr | vineflower | yak vs cfr |
|-----|---------|------------|----------------|-----|------------|------------|
| guava-28.2-android | 1892 | 18.84s | 1.12s | 6.12s | 5.86s | 5.5x |
| spring-core-6.1.10 | 1142 | 16.54s | 0.75s | 4.47s | 4.48s | 5.9x |
| jackson-databind-2.15.4 | 776 | 12.33s | 0.73s | 4.21s | 4.14s | 5.7x |
| fastjson2-2.0.43 | 681 | 58.15s | 3.25s | 8.28s | 10.14s | 2.6x |
| commons-collections4-4.4 | 524 | 4.41s | 0.29s | 2.13s | 2.02s | 7.3x |
| logback-core-1.4.14 | 453 | 1.85s | 0.20s | 2.01s | 1.56s | 10.0x |
| commons-lang3-3.12.0 | 345 | 7.14s | 1.01s | 2.75s | 2.53s | 2.7x |
| netty-codec-4.1.92.Final | 213 | 5.33s | 0.21s | 1.99s | 2.52s | 9.4x |
| gson-2.8.9 | 195 | 1.89s | 0.12s | 1.36s | 1.22s | 11.0x |
| fastjson-1.2.24 | 179 | 17.10s | 0.37s | 3.09s | 3.68s | 8.3x |

## Axis 1 & 2 — Completeness + Recompilability

| Jar | classes | yak ok | yak stub | yak recompile ok | cfr recompile ok | vf recompile ok |
|-----|---------|--------|----------|------------------|------------------|-----------------|
| guava-28.2-android | 1892 | 1892 | 0 | 18/533 (3%) | 550/558 (99%) | 165/558 (30%) |
| spring-core-6.1.10 | 1142 | 1142 | 0 | 716/717 (100%) | 753/762 (99%) | 753/761 (99%) |
| jackson-databind-2.15.4 | 776 | 776 | 0 | 42/453 (9%) | 470/474 (99%) | 143/473 (30%) |
| fastjson2-2.0.43 | 681 | 681 | 0 | 296/527 (56%) | 517/530 (98%) | 488/529 (92%) |
| commons-collections4-4.4 | 524 | 524 | 0 | 76/307 (25%) | 305/307 (99%) | 293/307 (95%) |
| logback-core-1.4.14 | 453 | 453 | 0 | 226/402 (56%) | 389/408 (95%) | 408/409 (100%) |
| commons-lang3-3.12.0 | 345 | 345 | 0 | 101/197 (51%) | 194/198 (98%) | 194/198 (98%) |
| netty-codec-4.1.92.Final | 213 | 213 | 0 | 28/143 (20%) | 140/143 (98%) | 42/143 (29%) |
| gson-2.8.9 | 195 | 195 | 0 | 31/73 (42%) | 71/74 (96%) | 60/75 (80%) |
| fastjson-1.2.24 | 179 | 179 | 0 | 65/143 (45%) | 141/143 (99%) | 123/143 (86%) |

## Axis 4 — Correctness (structural equivalence, Yak vs original bytecode)

| Jar | classes checked | structure match | member differ | signature differ |
|-----|-----------------|-----------------|---------------|------------------|
| guava-28.2-android | 1892 | 1211 | 681 | 0 |
| spring-core-6.1.10 | 1142 | 598 | 544 | 0 |
| jackson-databind-2.15.4 | 776 | 430 | 346 | 0 |
| fastjson2-2.0.43 | 681 | 389 | 292 | 0 |
| commons-collections4-4.4 | 524 | 373 | 151 | 0 |
| logback-core-1.4.14 | 453 | 211 | 242 | 0 |
| commons-lang3-3.12.0 | 345 | 158 | 187 | 0 |
| netty-codec-4.1.92.Final | 213 | 89 | 124 | 0 |
| gson-2.8.9 | 195 | 98 | 97 | 0 |
| fastjson-1.2.24 | 179 | 88 | 91 | 0 |
