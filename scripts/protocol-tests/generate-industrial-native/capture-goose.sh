#!/bin/sh
set -eu
mkdir -p /output
ip link add goose-pub type veth peer name goose-sub
ip link set goose-pub address 02:00:00:00:01:01
ip link set goose-sub address 02:00:00:00:01:02
ip link set goose-pub up
ip link set goose-sub up
for variant in 0 1 2; do
 tcpdump -i goose-sub -U -s 0 -w /output/libiec61850-$variant.pcap 'ether proto 0x88b8 or ether proto 0x8100' > /output/goose-$variant-capture.log 2>&1 &
 capture=$!
 /opt/goose-native subscribe goose-sub "$variant" > /output/goose-$variant-subscriber.log 2>&1 &
 subscriber=$!
 sleep 1
 /opt/goose-native publish goose-pub "$variant" > /output/goose-$variant-publisher.log 2>&1
 wait "$subscriber"
 sleep 1
 kill -INT "$capture"
 wait "$capture"
done
dpkg-query -W > /output/packages.txt
