.PHONY: fmtcheck test demo verify

fmtcheck:
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then \
		echo "gofmt needed on:"; echo "$$out"; \
		exit 1; \
	fi

test:
	go test ./...

demo:
	go run ./cmd/pfbackfill demo --out ./out/demo

verify: fmtcheck test demo
