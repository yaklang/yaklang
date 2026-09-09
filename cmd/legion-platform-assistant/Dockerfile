# Build context contains the prebuilt Linux binary legion-platform-assistant.
# Build the exact source outside Docker; do not rebuild during promotion.
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --chmod=0555 legion-platform-assistant /usr/local/bin/legion-platform-assistant
USER 65532:65532
EXPOSE 8094
ENTRYPOINT ["/usr/local/bin/legion-platform-assistant"]
CMD ["--listen", "0.0.0.0:8094"]
