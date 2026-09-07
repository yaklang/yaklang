# ScanNode SSA object-store replacement report

Date: 2026-09-01

## Environment

- Host: Apple M1 Max, 64 GiB RAM, Darwin 23.1.0 arm64
- Go: go1.22.12 darwin/arm64
- Server: MinIO RELEASE.2025-09-07T16-13-09Z, darwin/arm64
- MinIO binary SHA-256: `7c3b3039b76e55a1b80935848ed83998d5e8d317374f87851f46a019ff5c0aa4`
- Client and server ran on the same host over `127.0.0.1`
- Payload: one 64 MiB file, 16 MiB multipart size
- Samples: 10 independent one-iteration benchmark runs for each implementation

The baseline and replacement were built from the same repository revision. The
baseline used `minio-go/v7`; the replacement used the standard-library S3
client with multipart concurrency 2. P95 below uses the nearest-rank method;
with 10 samples it is the slowest observed sample.

## Results

| Metric | `minio-go/v7` baseline | Standard-library client | Change |
| --- | ---: | ---: | ---: |
| Median upload time | 203.1 ms | 163.1 ms | -19.7% |
| P95 upload time | 222.9 ms | 191.0 ms | -14.3% |
| Median throughput | 330.5 MB/s | 411.4 MB/s | +24.5% |
| Allocations per upload | about 51,245 | about 1,638 | -96.8% |
| `yak` binary (`go build -trimpath`) | 303,612,914 bytes | 301,800,562 bytes | -1,812,352 bytes (-1.73 MiB) |

Raw elapsed-time samples in nanoseconds:

- Baseline: `186773792, 184285166, 205682625, 222888250, 215296084, 195676500, 203400250, 208298500, 187629166, 202733792`
- Replacement: `159929000, 135358208, 170638125, 160434708, 191023792, 177717083, 168708000, 165789083, 151967375, 159623292`

The replacement intentionally keeps two reusable payload buffers to overlap
part uploads. Its bounded multipart payload memory is:

`part_size * concurrency`

That is 32 MiB by default (`16 MiB * 2`) and at most 256 MiB (`128 MiB * 2`).
It does not grow with object size.

## Dependency and interoperability checks

- `go list -deps ./common/yak/cmd` and `go version -m yak` contain neither
  `github.com/minio/minio-go/v7` nor `github.com/goccy/go-json` after the
  replacement.
- The old binary included `minio-go/v7`, `goccy/go-json`, `md5-simd`,
  `go-ini/ini`, and `rs/xid`; the new binary includes none of them.
- The real MinIO test covered small Put, a 5 MiB boundary object, a
  16 MiB multipart object, Unicode/space/special-character keys, read-back
  verification, and explicit multipart Abort followed by a list assertion that
  the Upload ID no longer exists.
- SigV4 is checked against AWS's published S3 signing example.
- Live AWS S3 and Ceph RGW tests require external credentials/services and
  were not run in this environment. The opt-in integration test accepts an
  existing bucket for those runs and does not create one when a bucket is
  explicitly supplied.

## Commands

```sh
go test ./scannode -run 'Test(SignS3|AWSURI|SecretValue|ValidateSSAUploadConfig|SSATLS|S3ObjectStore|ObjectStoreSession|ClassifyObjectStore)' -count=1
go test -race ./scannode -run 'Test(SignS3|AWSURI|SecretValue|ValidateSSAUploadConfig|SSATLS|S3ObjectStore|ObjectStoreSession|ClassifyObjectStore)' -count=1
go vet ./scannode
go list -deps ./common/yak/cmd
go build -trimpath -o /tmp/yak ./common/yak/cmd
go version -m /tmp/yak
```

Real-service checks use the `SCANNODE_S3_INTEGRATION_*` environment variables
defined in `ssa_object_store_integration_test.go`; credentials are deliberately
omitted from this report.
