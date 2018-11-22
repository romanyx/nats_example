package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/go-nats"
	"github.com/pkg/errors"
	"github.com/romanyx/nats_example/proto"
)

func main() {
	var (
		natsURL = flag.String("nats", "demo.nats.io", "url of NATS server")
		subj    = flag.String("subj", "test.registrate", "nats subject")
	)
	flag.Parse()

	// Connect to NATS and subscribe to the subjects.
	opts := []nats.Option{nats.Name("NATS Sample Requester")}
	natsConn, err := connectNATS(*natsURL, opts)
	if err != nil {
		log.Fatal(err)
	}
	defer natsConn.Flush()
	defer natsConn.Close()

	var ur proto.UserRequest
	ur.Email = "work@romanyx.ru"

	data, err := ur.Marshal()
	if err != nil {
		log.Fatal(err)
	}

	msg, err := natsConn.Request(*subj, data, 3*time.Second)
	if err != nil {
		log.Printf("got error on request %v\n", err)
		return
	}

	var urply proto.UserReply
	if err := urply.Unmarshal(msg.Data); err != nil {
		log.Printf("got error on reply unmasrshal: %v\n", err)
	}

	fmt.Printf("%+#v\n", urply)
}

func connectNATS(url string, opts []nats.Option) (*nats.Conn, error) {
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "connect to NATS")
	}

	return nc, nil
}
