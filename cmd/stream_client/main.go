package main

import (
	"context"
	"flag"
	"log"

	"github.com/nats-io/go-nats-streaming"
	natsCli "github.com/romanyx/nats_example/internal/nats"
	"github.com/romanyx/nats_example/proto"
)

func main() {
	var (
		natsURL   = flag.String("nats", "demo.nats.io", "url of NATS server")
		subj      = flag.String("subj", "test.jobs", "nats subject")
		clusterID = flag.String("cluster", "test-cluster", "NATS Stream cluster ID")
		clientID  = flag.String("client", "test-stream-client", "NATS Stream client unique ID")
	)
	flag.Parse()

	opts := []stan.Option{
		stan.NatsURL(*natsURL),
	}
	natsStream, err := natsCli.NewStreamCli(*clusterID, *clientID, opts)
	if err != nil {
		log.Fatalf("nats client: %v\n", err)
	}
	defer natsStream.Close(context.Background())

	var job proto.JobRequest
	job.Id = 1

	data, err := job.Marshal()
	if err != nil {
		log.Fatal(err)
	}

	if err := natsStream.Publish(*subj, data); err != nil {
		log.Fatalf("publish to the NATS conn: %v", err)
	}
}
