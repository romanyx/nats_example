FROM golang:latest as build_env

COPY . $GOPATH/src/github.com/romanyx/nats_example
WORKDIR $GOPATH/src/github.com/romanyx/nats_example/cmd/queue

ENV GO111MODULE=on
RUN go get -v
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -o -ldflags "-X main.version=0.0.1" /go/bin/queue

FROM alpine

COPY --from=build_env /go/bin/queue /bin/queue

ENTRYPOINT ["/bin/queue"]
