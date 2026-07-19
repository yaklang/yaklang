FROM python:3.12-slim AS builder
COPY challenge/server.py /tmp/server.py
RUN python3 -m compileall -q -b /tmp/server.py \
    && mkdir -p /out \
    && mv /tmp/server.pyc /out/server.pyc

FROM python:3.12-slim
WORKDIR /app
COPY app/ /app/
COPY --from=builder /out/server.pyc /opt/challenge/server.pyc
COPY entrypoint.sh /opt/challenge/entrypoint.sh
RUN chmod 0755 /opt/challenge/entrypoint.sh
ENTRYPOINT ["/opt/challenge/entrypoint.sh"]
CMD ["sleep", "infinity"]

