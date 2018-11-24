package main

import (
	"flag"
	"log"

	"github.com/romanyx/nats_example/proto"
	"github.com/streadway/amqp"
)

func main() {
	var (
		rabbitURL = flag.String("rabbit", "amqp://guest:guest@127.0.0.1:5672/", "url of NATS server")
		queue     = flag.String("queue", "queue", "nats subject queue name")
	)
	flag.Parse()

	conn, err := amqp.Dial(*rabbitURL)
	if err != nil {
		log.Fatalf("rabbit conn: %v\n", err)
	}
	defer conn.Close()

	var job proto.JobRequest
	job.Id = 1

	data, err := job.Marshal()
	if err != nil {
		log.Fatal(err)
	}

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("failed to open channel: %v", err)
	}

	if err := ch.Publish(
		"",     // exchange
		*queue, // routing key
		false,  // mandatory
		false,  // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        data,
		}); err != nil {
		log.Fatalf("failed to publish: %v", err)
	}
}
