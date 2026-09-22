# M2A native Redis / Kafka capture

The committed PCAP contains original loopback packets, not synthetic TCP wrappers.
Services used: Redis 7.2.5 built from its tagged source; Apache Kafka 3.9.1 binary
release with Java 17 and a fresh single-node KRaft directory. Services listen only
on 127.0.0.1, ports 16379 (Redis), 19092 (broker), 19093 (controller).

Use a disposable Redis database and Kafka log directory. Redis runs with
`--bind 127.0.0.1 --port 16379 --save '' --appendonly no` (`LC_ALL=C` on macOS).
Configure Kafka with node.id=1, broker/controller roles, loopback listeners,
controller.quorum.voters=1@127.0.0.1:19093, PLAINTEXT listeners, and replication
factors/min ISR of one. Create topic `m2-native` with two partitions before capture.
A fresh topic is essential: the oracle expects offsets 0..9 before the no-ack write.

```sh
# Start capture before client.py, then stop with SIGINT after the client exits.
tcpdump -i lo0 -U -s 0 -w m2-native.pcap 'tcp port 16379 or tcp port 19092'
python3 scripts/protocol-tests/generate-m2a/client.py --output native-oracle.json

tshark -r m2-native.pcap -d tcp.port==19092,kafka -Y kafka -T fields \
  -e frame.number -e kafka.api_key -e kafka.api_version \
  -e kafka.correlation_id -e kafka.error > kafka.tshark.tsv
```

The client uses Python's standard library and real TCP sockets. Request builders
follow Apache Kafka 3.9.1 `clients/src/main/resources/common/message/*.json`.
Actual broker Produce error codes must be zero. The broker independently checks
record batch CRC; the parser is not used by this generator. Raw request/reply hex
is retained alongside separate TShark 4.4.8 fields. Tests also assert decoded
record values and assigned offsets, not just message counts.

The invalid Redis AUTH password is test-only. Never use production credentials
or point this generator at an existing service/database. Redis transactions,
RESP3 subscription/invalidation pushes, Kafka ApiVersions 0..2, Metadata 0..8,
Produce 3..7 (none/gzip), Fetch 4..11, and Produce acks=0 are represented.
RESP3 attribute, malformed, fragmentation, expiry and resource cases are crafted
unit tests and are not labeled real-server captures.

Sources: https://github.com/redis/redis/releases/tag/7.2.5 and
https://archive.apache.org/dist/kafka/3.9.1/ . The manifest pins the original
capture and oracle hashes; regenerated TCP sequence numbers and packetization
can differ, so semantic checks are the reproduction contract.
