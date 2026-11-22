.PHONY: all rust go test demo run clean

all: rust go

rust:
	cargo build --release --manifest-path rust/Cargo.toml

go: rust
	cd go && go build ./...

test: rust
	cargo test --manifest-path rust/Cargo.toml
	cd go && go test ./...
	PYTHONPATH=python python3 -m unittest discover -s python/tests -v

run: all
	cd go && go run ./cmd/proxy -config ../config/apishadow.conf

demo: all
	bash examples/demo.sh

clean:
	cargo clean --manifest-path rust/Cargo.toml
	rm -f capture.jsonl
