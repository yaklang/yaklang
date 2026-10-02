#!/bin/sh
set -eu

OUT=${1:-/tmp/sip-sipp-rtp-duplex-loopback.pcap}
IFACE=${CAPTURE_INTERFACE:-lo0}
SIPP=${SIPP:-sipp}
TCPDUMP=${TCPDUMP:-tcpdump}
PYTHON=${PYTHON:-python3}
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
OUT_DIR=$(dirname -- "$OUT")
mkdir -p "$OUT_DIR"

for command in "$SIPP" "$TCPDUMP" "$PYTHON" tshark capinfos; do
	command -v "$command" >/dev/null 2>&1 || {
		echo "required tool not found: $command" >&2
		exit 2
	}
done

TCPDUMP_PID=
UAS_PID=
UAC_PID=
ECHO_PID=
SENDER_PID=
cleanup() {
	if [ -n "$TCPDUMP_PID" ]; then kill -INT "$TCPDUMP_PID" 2>/dev/null || true; fi
	if [ -n "$SENDER_PID" ]; then kill "$SENDER_PID" 2>/dev/null || true; fi
	if [ -n "$ECHO_PID" ]; then kill "$ECHO_PID" 2>/dev/null || true; fi
	if [ -n "$UAC_PID" ]; then kill "$UAC_PID" 2>/dev/null || true; fi
	if [ -n "$UAS_PID" ]; then kill "$UAS_PID" 2>/dev/null || true; fi
	wait 2>/dev/null || true
}
trap cleanup EXIT HUP INT TERM

"$TCPDUMP" -i "$IFACE" -nn -s0 -U -w "$OUT" \
	'udp and (port 15072 or port 15073 or portrange 32200-32301)' \
	>"$OUT_DIR/tcpdump.stdout" 2>"$OUT_DIR/tcpdump.log" &
TCPDUMP_PID=$!
sleep 0.25

"$SIPP" -sn uas -i 127.0.0.1 -p 15072 -m 1 -mp 32200 -mi 127.0.0.1 \
	-trace_err >"$OUT_DIR/sipp-uas.log" 2>&1 &
UAS_PID=$!
sleep 0.25

"$PYTHON" "$SCRIPT_DIR/rtp_endpoint.py" echo 32200 32300 63 \
	>"$OUT_DIR/rtp-echo.log" 2>&1 &
ECHO_PID=$!
"$SIPP" 127.0.0.1:15072 -sn uac -i 127.0.0.1 -p 15073 -m 1 \
	-mp 32300 -mi 127.0.0.1 -d 3000 -trace_err \
	>"$OUT_DIR/sipp-uac.log" 2>&1 &
UAC_PID=$!
sleep 0.45

"$PYTHON" "$SCRIPT_DIR/rtp_endpoint.py" send 32300 32200 63 \
	>"$OUT_DIR/rtp-send.log" 2>&1 &
SENDER_PID=$!
wait "$SENDER_PID"
SENDER_PID=
wait "$ECHO_PID"
ECHO_PID=
# Keep the live capture until SIPp finishes the call scenario. Capturing only
# until the media sender exits can kill the user agent during its final pause
# and leave a valid-looking but incomplete signaling exchange.
wait "$UAC_PID"
UAC_PID=
wait "$UAS_PID"
UAS_PID=
kill -INT "$TCPDUMP_PID"
wait "$TCPDUMP_PID" || true
TCPDUMP_PID=

"$SIPP" -v >"$OUT_DIR/sipp-version.txt" 2>&1 || true
"$TCPDUMP" --version >"$OUT_DIR/tcpdump-version.txt" 2>&1
tshark --version >"$OUT_DIR/tshark-version.txt" 2>&1
capinfos "$OUT" >"$OUT_DIR/capinfos.txt" 2>&1
"$PYTHON" --version >"$OUT_DIR/python-version.txt" 2>&1

# Keep the independent field oracle beside each reproduced capture. These
# columns include the complete media tuple and per-datagram RTP evidence; the
# parser under test is never used to construct expected values.
tshark -r "$OUT" -2 -T fields \
	-E header=y -E separator=/t -E quote=n -E occurrence=f \
	-e frame.number -e _ws.col.protocol -e frame.time_epoch \
	-e ip.src -e udp.srcport -e ip.dst -e udp.dstport \
	-e sip.Method -e sip.Status-Code -e sip.Call-ID \
	-e sdp.connection_info.address -e sdp.media.port \
	-e rtp.p_type -e rtp.seq -e rtp.timestamp -e rtp.ssrc \
	-e udp.length -e rtp.payload >"$OUT_DIR/sip-rtp-tshark-oracle.tsv"

"$PYTHON" - "$OUT_DIR/sip-rtp-tshark-oracle.tsv" <<'PY'
import csv
import sys

with open(sys.argv[1], newline="") as source:
    rows = list(csv.DictReader(source, delimiter="\t"))
sip = [row for row in rows if row["sip.Method"] or row["sip.Status-Code"]]
rtp = [row for row in rows if row["rtp.p_type"]]
methods = {row["sip.Method"] for row in sip if row["sip.Method"]}
statuses = {row["sip.Status-Code"] for row in sip if row["sip.Status-Code"]}
tuples = {
    (row["ip.src"], row["udp.srcport"], row["ip.dst"], row["udp.dstport"])
    for row in rtp
}
if not {"INVITE", "ACK"}.issubset(methods) or not {"180", "200"}.issubset(statuses):
    raise SystemExit(f"TShark did not observe the complete INVITE/180/200/ACK exchange: {methods=} {statuses=}")
if len(rtp) != 126 or len(tuples) != 2:
    raise SystemExit(f"TShark oracle must observe 126 RTP datagrams in two directions: {len(rtp)=} {tuples=}")
if any(not row["rtp.payload"] for row in rtp):
    raise SystemExit("TShark oracle has an RTP datagram with no independently decoded payload")
print(f"oracle verified sip_messages={len(sip)} rtp_datagrams={len(rtp)} directions={len(tuples)}")
PY

kill "$UAS_PID" "$UAC_PID" 2>/dev/null || true
wait "$UAS_PID" "$UAC_PID" 2>/dev/null || true
UAS_PID=
UAC_PID=

grep -q 'tx=63 rx=63' "$OUT_DIR/rtp-send.log"
grep -q 'tx=63 rx=63' "$OUT_DIR/rtp-echo.log"
grep -q 'Total Calls created' "$OUT_DIR/sipp-uac.log"
echo "captured live SIPp + duplex UDP PCMU traffic to $OUT"
