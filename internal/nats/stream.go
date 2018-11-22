package nats

import (
	"context"
	"fmt"

	nats "github.com/nats-io/go-nats"
	"github.com/nats-io/go-nats-streaming"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

var (
	defaultSubscribeOpts = []stan.SubscriptionOption{
		stan.MaxInflight(1),
		stan.SetManualAckMode(),
	}
)

// Stream represents stream client  for NATS.
type Stream struct {
	conn     stan.Conn
	natsConn *nats.Conn
	clientID string
	subs     map[string]stan.Subscription
}

// NewStreamCli connects NATS steamng client.
func NewStreamCli(clusterID, clientID string, opts []stan.Option) (*Stream, error) {
	sc, err := stan.Connect(clusterID, clientID, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "connect to nats stream")
	}

	s := Stream{
		conn:     sc,
		natsConn: sc.NatsConn(),
		clientID: clientID,
		subs:     make(map[string]stan.Subscription),
	}

	return &s, nil
}

// StreamHandler handler for subscription.
type StreamHandler func(context.Context, *stan.Msg)

// Sequence get and set last sequence.
// Implementation should be atomic.
type Sequence interface {
	Last() uint64
	Swap(uint64)
}

// QueueFunc registers client to given queue.
// Supports I want exactly once processing with
// by sequence.
func (s *Stream) QueueFunc(subj, queue string, sq Sequence, h StreamHandler) error {
	if _, ok := s.subs[subj]; ok {
		return errors.Errorf("subject %s already subscribed", subj)
	}

	sOpts := append(defaultSubscribeOpts, stan.DurableName(fmt.Sprintf("%s.subscription.%s", s.clientID, subj)))
	sOpts = append(defaultSubscribeOpts, stan.StartAtSequence(sq.Last()))

	sub, err := s.conn.QueueSubscribe(subj, queue, func(msg *stan.Msg) {
		if l := sq.Last(); msg.Sequence > l { // process only messages with sequence > last sequence.
			ctx := context.Background()
			ctx, span := trace.StartSpan(ctx, fmt.Sprintf("subscribe.%s", subj))
			defer span.End()

			h(ctx, msg)
		}

		// Ack with the server.
		if err := msg.Ack(); err != nil {
			// Swap sequence.
			sq.Swap(msg.Sequence)
		}
	}, sOpts...)

	if err != nil {
		return errors.Wrapf(err, "subscribe to %s", subj)
	}

	s.subs[subj] = sub
	return nil
}

// Publish calls nats connection Publish func.
func (s *Stream) Publish(subj string, data []byte) error {
	return s.conn.Publish(subj, data)
}

// Status calls nats connection Status func.
func (s *Stream) Status() nats.Status {
	return s.natsConn.Status()
}

// Close unsubscribes subscriptions and
// closes the connection.
func (s *Stream) Close(ctx context.Context) error {
	errChan := make(chan error)

	go func() {
		if err := s.close(); err != nil {
			errChan <- err
			return
		}

		errChan <- nil
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

func (s *Stream) close() error {
	defer s.natsConn.Close()

	for subj, sub := range s.subs {
		if err := sub.Unsubscribe(); err != nil {
			return errors.Wrapf(err, "unsubscribe %s", subj)
		}
	}

	return nil
}

// StreamMiddleware represents middleware for NATS stream client.
type StreamMiddleware func(StreamHandler) StreamHandler

// StreamChain represents chain for NATS stream client.
type StreamChain struct {
	middlewares []StreamMiddleware
}

// NewStreamChain creates new chain with middlewares.
func NewStreamChain(middlewares ...StreamMiddleware) StreamChain {
	chain := StreamChain{
		middlewares: append(([]StreamMiddleware)(nil), middlewares...),
	}

	return chain
}

// Then chains middleware and returns the final StreamHandler.
func (c StreamChain) Then(h StreamHandler) StreamHandler {
	for i := range c.middlewares {
		h = c.middlewares[len(c.middlewares)-1-i](h)
	}

	return h
}
