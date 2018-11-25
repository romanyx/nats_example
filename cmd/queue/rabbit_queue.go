package main

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/romanyx/nats_example/internal/process"
	"github.com/streadway/amqp"
	"go.opencensus.io/trace"
)

func setupRabbitQueue(conn *amqp.Connection, w io.Writer, queue string) {
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("failed to open channel: %v", err)
	}

	if _, err := ch.QueueDeclare(
		queue, // name
		false, // durable
		false, // delete when usused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	); err != nil {
		log.Fatalf("failed to declaire queue: %v", err)
	}

	msgs, err := ch.Consume(
		queue, // queue
		"",    // consumer
		true,  // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		log.Fatalf("failed to consume queue: %v", err)
	}

	srv := processWithWriter{Writer: w}
	h := process.NewNatsHandler(srv)

	go func() {
		for d := range msgs {
			d := d
			go func() {
				// TODO(romanyx): add execute only once with sequence checking,
				// handle panics and so on, this is just a demonstration.
				ctx := context.Background()
				ctx, span := trace.StartSpan(ctx, fmt.Sprintf("rabbit.%s", queue))
				defer span.End()
				m := wrapRabbitMsg{d}

				if err := h(ctx, &m); err != nil {
					log.Printf("rabbit error or message process: %+v", err)
				}
			}()
		}
	}()
}

type wrapRabbitMsg struct {
	msg amqp.Delivery
}

func (m wrapRabbitMsg) Data() []byte {
	return m.msg.Body

}
