.PHONY: build run test lint fmt tidy clean samples demo

# nodynamic keeps codecs from picking up system libraries, so a build behaves
# the same on every machine.
TAGS := nodynamic

build:
	go build -tags $(TAGS) -o bin/squish ./cmd/squish

run:
	go run -tags $(TAGS) ./cmd/squish

test:
	go test -tags $(TAGS) -race ./...

lint:
	golangci-lint run --build-tags $(TAGS)

fmt:
	golangci-lint fmt

tidy:
	go mod tidy

samples:
	./scripts/fetch-samples.sh

demo: build samples
	vhs docs/demo.tape

clean:
	rm -rf bin dist
