SHELL := /bin/sh

all: queue

queue:
	cd "$$GOPATH/src/github.com/romanyx/nats_example"
	docker build \
		-t nats/queue:0.0.1 \
		-f docker/queue/Dockerfile \
		.

cover:
	go test --race `go list ./... | grep -v /vendor | grep -v /internal/nats | grep -v /internal/docker | grep -v /cmd/stream_client` -coverprofile cover.out.tmp && \
		cat cover.out.tmp | grep -v "main.go" > cover.out && \
		go tool cover -func cover.out && \
		rm cover.out.tmp && \
		rm cover.out

send_job:
	go run ./cmd/stream_client/main.go -nats=localhost:4222

