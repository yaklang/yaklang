#!/usr/bin/env bash
# Record of commands for sw64 vendor yak build.
# Unpacks scripts/sw-go-porter-1.1.0-20250429-9058a7e03f.tar.gz, then runs the
# top-level port.sh from the archive (inner folder name may differ from the .tar.gz name).

set -euo pipefail

log() { printf '%s\n' "[sw64-vendor-yak-build] $*" >&2; }

if [[ -z "${BASH_VERSION:-}" ]]; then
  echo "error: 请用 bash 运行本脚本，例如: bash scripts/sw64-vendor-yak-build.sh 或 chmod +x 后 ./scripts/sw64-vendor-yak-build.sh（不要用 sh 调用）" >&2
  exit 1
fi

trap 'log "失败: 第 ${LINENO} 行退出码 $?"' ERR

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PORTER_TGZ="${SCRIPT_DIR}/sw-go-porter-1.1.0-20250429-9058a7e03f.tar.gz"

log "仓库根目录: ${REPO_ROOT}"

if [[ ! -f "$PORTER_TGZ" ]]; then
  echo "error: missing ${PORTER_TGZ}" >&2
  exit 1
fi

# tar|head 在 pipefail 下常使 tar 收到 SIGPIPE（退出码 141）；子 shell 内关闭 pipefail 仅用于取首行
PORTER_TOP="$(set +o pipefail; tar -tzf "$PORTER_TGZ" | head -n1 | tr -d '\r')"
PORTER_TOP="${PORTER_TOP%/}"
if [[ -z "$PORTER_TOP" || "$PORTER_TOP" == *"/"* ]]; then
  echo "error: could not determine porter root directory from ${PORTER_TGZ}" >&2
  exit 1
fi

PORTER_ROOT="${SCRIPT_DIR}/${PORTER_TOP}"
if [[ -e "$PORTER_ROOT" ]]; then
  log "已存在解压目录，先删除: ${PORTER_ROOT}"
  rm -rf "$PORTER_ROOT"
fi

log "解压: ${PORTER_TGZ} -> ${SCRIPT_DIR}/"
tar -xzf "$PORTER_TGZ" -C "$SCRIPT_DIR"
log "porter 目录: ${PORTER_ROOT}"

cd "$REPO_ROOT"

log "执行: go work vendor"
go work vendor

cp vendor/modules.txt /tmp/modules.txt.backup

log "清空 vendor/ 后运行 porter"
rm -fr vendor/

log "执行: GOWORK=off ${PORTER_ROOT}/port.sh ."
GOWORK=off "$PORTER_ROOT/port.sh" .

cp /tmp/modules.txt.backup vendor/modules.txt

log "追加 sw64 常量到 vendor/golang.org/x/sys/unix/ztypes_linux_sw64.go"
cat >> vendor/golang.org/x/sys/unix/ztypes_linux_sw64.go << 'EOF'
// 网络隧道和虚拟化卸载常量
const (
// 隧道(TUN)设备标志
TUN_F_CSUM = 0x01
TUN_F_TSO4 = 0x02
TUN_F_TSO6 = 0x04
TUN_F_USO4 = 0x08
TUN_F_USO6 = 0x10
// 虚拟网络卸载(Virtio)标志
VIRTIO_NET_HDR_GSO_NONE     = 0
VIRTIO_NET_HDR_F_NEEDS_CSUM = 1
VIRTIO_NET_HDR_GSO_TCPV4    = 1
VIRTIO_NET_HDR_GSO_TCPV6    = 4
VIRTIO_NET_HDR_GSO_UDP_L4   = 5
)
EOF

export CGO_ENABLED=1
export GOOS=linux
export GOARCH=sw64
export CGO_LDFLAGS="-lpcap"

log "执行: go build -mod=vendor ./common/yak/cmd/yak.go"
go build -mod=vendor ./common/yak/cmd/yak.go

trap - ERR
log "完成。构建在仓库根目录执行，可执行文件名以 go build 默认规则为准（工作目录: ${REPO_ROOT}）"
