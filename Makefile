SHELL := /bin/sh

all: queue start

queue:
	cd "$$GOPATH/src/github.com/romanyx/nats_example"
	docker build \
		-t nats/queue:0.0.1 \
		-f docker/queue/Dockerfile \
		--build-arg VCS_REF=`git rev-parse HEAD` \
		--build-arg BUILD_DATE=`date -u +”%Y-%m-%dT%H:%M:%SZ”` \
		.

cover:
	go test --race `go list ./... | grep -v /vendor | grep -v /internal/nats | grep -v /internal/docker | grep -v /cmd/stream_client` -coverprofile cover.out.tmp && \
		cat cover.out.tmp | grep -v "main.go" | grep -v "rabbit_queue.go" > cover.out && \
		go tool cover -func cover.out && \
		rm cover.out.tmp && \
		rm cover.out

bench: 
	go test -bench=. ./...

send_job:
	go run ./cmd/stream_client/main.go -nats=localhost:4222

start:
	docker-compose up -d

