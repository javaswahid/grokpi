FROM alpine:3.21

ARG TARGETARCH

COPY dist/linux-${TARGETARCH}/grokpi-linux-${TARGETARCH} /usr/local/bin/grokpi
COPY config.defaults.toml /app/config.toml

RUN apk add --no-cache ca-certificates tzdata && \
    chmod +x /usr/local/bin/grokpi && \
    adduser -D -u 1000 grokpi && \
    mkdir -p /app/data && \
    chown -R grokpi:grokpi /app

USER grokpi

WORKDIR /app
VOLUME ["/app/data"]
EXPOSE 8080

ENTRYPOINT ["grokpi"]
CMD ["-config", "/app/config.toml"]
