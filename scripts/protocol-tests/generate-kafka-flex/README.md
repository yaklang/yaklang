# Kafka flexible fixture reproduction

Use Apache Kafka **3.9.1** (`kafka_2.13-3.9.1`) and JDK 17. The fixture manifest
pins the client JAR and official message schemas. These programs use Apache's
independently generated Java codecs, not Yaklang encoders.

Start an isolated plaintext KRaft broker listening at `127.0.0.1:19092` with a
fresh data directory. Before capturing, create topic `zip-flex` with **two**
partitions and replication factor 1. A fresh topic is required because Fetch
assertions deliberately verify offsets 0/1 and exactly two batches per partition.
Do not run the producer twice against the same topic when regenerating evidence.

```sh
KAFKA_DIST=/path/to/kafka_2.13-3.9.1
KAFKA_FIXTURE_BUILD=/tmp/kafka-flex-classes
mkdir -p "$KAFKA_FIXTURE_BUILD"
javac -cp "$KAFKA_DIST/libs/*" -d "$KAFKA_FIXTURE_BUILD" scripts/protocol-tests/generate-kafka-flex/*.java
# In another terminal; stop with Ctrl-C after KafkaFlex exits:
tcpdump -i lo0 -U -w /tmp/kafka-flex-native.pcap 'tcp port 19092'
# In the original terminal, from the repository root:
java -cp "$KAFKA_FIXTURE_BUILD:$KAFKA_DIST/libs/*" KafkaFlex /tmp/java-oracle.json
java -cp "$KAFKA_FIXTURE_BUILD:$KAFKA_DIST/libs/*" KafkaFlexTags /tmp/java-tags-oracle.json
```

Use `lo` instead of `lo0` on Linux. Save tcpdump's packet/drop counters and update
SHA256 entries in the fixture manifest when replacing captured artifacts. The
checked-in capture has 50 packets, 9 requests and 8 responses, zero kernel drops.
Metadata's null-topic query may expose other local test topic names; use only an
isolated fixture broker, never production traffic or credentials.

`KafkaFlex` sends ApiVersions v3, Metadata v9 (empty/null/named topic lists),
Produce v9 (none/gzip, two partitions, acks=0/1) and Fetch v12. Request headers
and request root structs include unknown tags. Actual responses are decoded by
Kafka's Java codec with exact consumption and matching correlation IDs.
`KafkaFlexTags` emits five offline (not captured) cases: Fetch known partition
and cluster tags, nested unknown tags, null/empty records/aborted transactions,
ApiVersions migration tag and Produce batch errors with null/empty messages.

Run the checked-in oracles without a broker/JDK:

```sh
go test ./common/bin-parser/parser/stream_parser ./common/pcapx/pcaputil -run TestKafkaFlexible -count=1
go test ./common/bin-parser/parser/stream_parser -run '^$' -fuzz '^FuzzKafkaFlexible$' -fuzztime=45s -parallel=2
```

This is a fixed wire-version profile. It does not claim compatibility with every
version negotiated by Kafka 3.9.1's high-level producer/consumer or add codecs
other than none/gzip.
