VERSION  ?= $(shell git describe --tags --always --dirty)
REVISION ?= $(shell git rev-parse HEAD)
IMAGE    ?= demo-checkout
LDFLAGS  := -X main.version=$(VERSION)

.PHONY: test build run image

test:
	go vet ./...
	go test -race ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/checkout .

run:
	go run -ldflags "$(LDFLAGS)" .

image:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg REVISION=$(REVISION) \
		-t $(IMAGE):$(VERSION) .
