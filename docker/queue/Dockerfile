FROM golang:latest as build_env

COPY . $GOPATH/src/github.com/romanyx/nats_example
WORKDIR $GOPATH/src/github.com/romanyx/nats_example/cmd/queue

ENV GO111MODULE=on
RUN go get -v
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -ldflags "-X main.version=$(git describe --abbrev=0)" -a -tags netgo -o /go/bin/queue

FROM alpine
ARG BUILD_DATE
ARG VCS_REF
COPY --from=build_env /go/bin/queue /bin/queue

ENTRYPOINT ["/bin/queue"]

LABEL org.opencontainers.image.created=$BUILD_DATE \
      org.opencontainers.image.title="queue" \
      org.opencontainers.image.authors="Roman Budnikov <work@romanyx.ru>" \
      org.opencontainers.image.source="https://github.com/romanyx/nats_example/cmd/queue" \
      org.opencontainers.image.revision=$VCS_REF \
