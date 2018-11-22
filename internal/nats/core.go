package nats

import (
	"context"
	"fmt"

	nats "github.com/nats-io/go-nats"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

// Core represents core client for NATS.
type Core struct {
	conn *nats.Conn
	subs map[string]*nats.Subscription
}

// NewCli connects NATS core client.
func NewCli(url string, opts []nats.Option) (*Core, error) {
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "connect to %s", url)
	}

	b := Core{
		conn: nc,
		subs: make(map[string]*nats.Subscription),
	}

	return &b, nil
}

// CoreHandler handler for subscription.
type CoreHandler func(context.Context, *nats.Msg)

// ReplyFunc registers the replier funtion for given subject.
func (b *Core) ReplyFunc(subj string, h CoreHandler) error {
	if _, ok := b.subs[subj]; ok {
		return errors.Errorf("subject %s already subscribed", subj)
	}

	sub, err := b.conn.Subscribe(subj, func(msg *nats.Msg) {
		ctx := context.Background()
		ctx, span := trace.StartSpan(ctx, fmt.Sprintf("subscribe.%s", subj))
		defer span.End()

		h(ctx, msg)
	})

	if err != nil {
		return errors.Wrapf(err, "subscribe to %s", subj)
	}

	b.subs[subj] = sub
	return nil
}

// Publish calls nats connection Publish func.
func (b *Core) Publish(subj string, data []byte) error {
	return b.conn.Publish(subj, data)
}

// Status calls nats connection Status func.
func (b *Core) Status() nats.Status {
	return b.conn.Status()
}

// Close unsubscribes subscriptions and
// closes the connection.
func (b *Core) Close(ctx context.Context) error {
	errChan := make(chan error)

	go func() {
		if err := b.close(); err != nil {
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

func (b *Core) close() error {
	defer b.conn.Close()

	for subj, sub := range b.subs {
		if err := sub.Unsubscribe(); err != nil {
			return errors.Wrapf(err, "unsubscribe %s", subj)
		}
	}

	return nil
}

// CoreMiddleware represents middleware for NATS core client.
type CoreMiddleware func(CoreHandler) CoreHandler

// CoreChain represents chain for NATS core client.
type CoreChain struct {
	middlewares []CoreMiddleware
}

// NewCoreChain creates new chain with middlewares.
func NewCoreChain(middlewares ...CoreMiddleware) CoreChain {
	chain := CoreChain{
		middlewares: append(([]CoreMiddleware)(nil), middlewares...),
	}

	return chain
}

// Then chains middleware and returns the final nats.MsgHandler.
func (c CoreChain) Then(h CoreHandler) CoreHandler {
	for i := range c.middlewares {
		h = c.middlewares[len(c.middlewares)-1-i](h)
	}

	return h
}
