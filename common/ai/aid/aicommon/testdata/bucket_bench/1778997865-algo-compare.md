# Bucket Bench: Dynamic Algorithm Compare

generated at 2026-05-17T14:04:25+08:00

| scenario | budget | events | flush | stable | avg-frozen | p95-frozen | max-frozen | est-create | est-hit | net-cost | sub-blocks |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| short_query | A_Fixed_16K(baseline) | 30 | 1 | 28 | 1.6K | 16.0K | 16.0K | 20.0K | 19.2K | 820B | 1 |
| short_query | A_Fixed_64K | 30 | 0 | 29 | 0B | 0B | 0B | 0B | 0B | 0B | 0 |
| short_query | B_TimeRemaining(64K->8K) | 30 | 0 | 29 | 0B | 0B | 0B | 0B | 0B | 0B | 0 |
| short_query | C_EntryAdaptive(8x,32K-256K) | 30 | 0 | 29 | 0B | 0B | 0B | 0B | 0B | 0B | 0 |
| short_query | D_TokenAware(5000tok) | 30 | 1 | 28 | 1.1K | 0B | 16.6K | 20.7K | 10.0K | 10.8K | 1 |
| dense_tools | A_Fixed_16K(baseline) | 20 | 7 | 12 | 47.1K | 82.7K | 98.3K | 486.9K | 331.2K | 155.7K | 7 |
| dense_tools | A_Fixed_64K | 20 | 1 | 18 | 27.1K | 60.3K | 60.3K | 75.4K | 289.4K | -214.1K | 1 |
| dense_tools | B_TimeRemaining(64K->8K) | 20 | 6 | 13 | 41.9K | 93.9K | 98.2K | 584.8K | 222.5K | 362.3K | 6 |
| dense_tools | C_EntryAdaptive(8x,32K-256K) | 20 | 2 | 17 | 36.5K | 75.5K | 75.5K | 143.7K | 368.6K | -224.9K | 2 |
| dense_tools | D_TokenAware(5000tok) | 20 | 7 | 12 | 46.9K | 88.9K | 98.3K | 516.6K | 314.7K | 201.9K | 7 |
| single_huge | A_Fixed_16K(baseline) | 6 | 0 | 5 | 0B | 0B | 0B | 0B | 0B | 0B | 0 |
| single_huge | A_Fixed_64K | 6 | 0 | 5 | 0B | 0B | 0B | 0B | 0B | 0B | 0 |
| single_huge | B_TimeRemaining(64K->8K) | 6 | 0 | 5 | 0B | 0B | 0B | 0B | 0B | 0B | 0 |
| single_huge | C_EntryAdaptive(8x,32K-256K) | 6 | 0 | 5 | 0B | 0B | 0B | 0B | 0B | 0B | 0 |
| single_huge | D_TokenAware(5000tok) | 6 | 0 | 5 | 0B | 0B | 0B | 0B | 0B | 0B | 0 |
| mixed | A_Fixed_16K(baseline) | 36 | 19 | 16 | 92.8K | 176.9K | 193.5K | 2.40M | 823.9K | 1.60M | 19 |
| mixed | A_Fixed_64K | 36 | 4 | 31 | 69.4K | 134.7K | 191.8K | 641.1K | 1.16M | -551.2K | 4 |
| mixed | B_TimeRemaining(64K->8K) | 36 | 13 | 22 | 87.8K | 188.7K | 192.8K | 1.66M | 1.06M | 621.3K | 13 |
| mixed | C_EntryAdaptive(8x,32K-256K) | 36 | 5 | 30 | 81.4K | 171.5K | 171.5K | 633.4K | 1.42M | -821.5K | 5 |
| mixed | D_TokenAware(5000tok) | 36 | 13 | 22 | 90.4K | 176.4K | 192.8K | 1.66M | 1.11M | 556.6K | 13 |
| real_redhaze | A_Fixed_16K(baseline) | 90 | 16 | 73 | 87.9K | 168.6K | 168.6K | 1.79M | 3.78M | -1.99M | 16 |
| real_redhaze | A_Fixed_64K | 90 | 7 | 82 | 82.7K | 167.6K | 167.6K | 862.6K | 3.96M | -3.12M | 7 |
| real_redhaze | B_TimeRemaining(64K->8K) | 90 | 18 | 71 | 87.7K | 168.8K | 168.8K | 2.04M | 3.65M | -1.61M | 18 |
| real_redhaze | C_EntryAdaptive(8x,32K-256K) | 90 | 8 | 81 | 83.1K | 167.7K | 167.7K | 961.9K | 3.93M | -2.99M | 8 |
| real_redhaze | D_TokenAware(5000tok) | 90 | 14 | 75 | 87.2K | 168.4K | 168.4K | 1.59M | 3.83M | -2.24M | 14 |

## Per-scenario winner (lowest net cost)

| scenario | best algo | net-cost | flush | avg-frozen |
| --- | --- | --- | --- | --- |
| short_query | A_Fixed_64K | 0B | 0 | 0B |
| dense_tools | C_EntryAdaptive(8x,32K-256K) | -224.9K | 2 | 36.5K |
| single_huge | A_Fixed_16K(baseline) | 0B | 0 | 0B |
| mixed | C_EntryAdaptive(8x,32K-256K) | -821.5K | 5 | 81.4K |
| real_redhaze | A_Fixed_64K | -3.12M | 7 | 82.7K |
