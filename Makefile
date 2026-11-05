.PHONY: build run test lint fmt tidy clean

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
	golangci-lint run

fmt:
	golangci-lint fmt

tidy:
	go mod tidy

clean:
	rm -rf bin dist
