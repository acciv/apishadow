FROM rust:latest AS rust-builder
WORKDIR /src
COPY rust/ rust/
RUN cargo build --release --manifest-path rust/Cargo.toml

FROM golang:1.23 AS go-builder
WORKDIR /src
COPY go/ go/
COPY rust/include/ rust/include/
COPY --from=rust-builder /src/rust/target/release/libapishadow_capture.so /src/rust/target/release/
RUN cd go && CGO_ENABLED=1 go build -o /out/apishadow-proxy ./cmd/proxy

FROM python:3.12-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
ENV LD_LIBRARY_PATH=/usr/local/lib PYTHONPATH=/opt/apishadow/python
COPY --from=rust-builder /src/rust/target/release/libapishadow_capture.so /usr/local/lib/
COPY --from=go-builder /out/apishadow-proxy /usr/local/bin/apishadow-proxy
COPY python/ /opt/apishadow/python/
COPY config/ /etc/apishadow/
EXPOSE 8080
ENTRYPOINT ["apishadow-proxy", "-config", "/etc/apishadow/apishadow.conf"]
