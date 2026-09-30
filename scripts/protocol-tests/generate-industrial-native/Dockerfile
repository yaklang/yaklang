FROM debian@sha256:3783cc01769c7b2b1b83a5c5ad96c815348e28ed7da68e2e3687004faa906251
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends build-essential cmake git ca-certificates iproute2 tcpdump && rm -rf /var/lib/apt/lists/*
RUN git clone https://github.com/mz-automation/libiec61850.git /opt/libiec61850 && cd /opt/libiec61850 && git checkout 519b0208cc79d1af09d5ca40fb9ad1fd93822e93 && cmake -S . -B build -DBUILD_EXAMPLES=OFF && cmake --build build -j4
COPY goose.c /opt/goose.c
RUN gcc /opt/goose.c -I/opt/libiec61850/src/common/inc -I/opt/libiec61850/src/goose -I/opt/libiec61850/src/mms/inc -I/opt/libiec61850/src/iec61850/inc -I/opt/libiec61850/src/logging -I/opt/libiec61850/src/r_session -I/opt/libiec61850/config -I/opt/libiec61850/hal/inc /opt/libiec61850/build/src/libiec61850.a /opt/libiec61850/build/hal/libhal.a -lpthread -lm -o /opt/goose-native
COPY capture-goose.sh /opt/capture-goose.sh
ENTRYPOINT ["/bin/sh", "/opt/capture-goose.sh"]
