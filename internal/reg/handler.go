package reg

import (
	"context"
	"sync"

	nats "github.com/nats-io/go-nats"
	"github.com/pkg/errors"
	"github.com/romanyx/nats_example/proto"
	"go.opencensus.io/trace"
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		var buf buffer
		return &buf
	},
}

type buffer struct {
	req  proto.UserRequest
	rply proto.UserReply
}

func (b *buffer) Clear() {
	b.req.Email = ""
	b.rply.Email = ""
	bufferPool.Put(b)
}

type publishFunc func(subject string, data []byte) error

// NewHandler returns handle func for nats messages handling.
func NewHandler(srv registrater, pbl publishFunc) func(context.Context, *nats.Msg) error {
	return func(ctx context.Context, msg *nats.Msg) error {
		ctx, span := trace.StartSpan(ctx, "handler")
		defer span.End()

		buf := bufferPool.Get().(*buffer)
		defer buf.Clear()

		if err := buf.req.Unmarshal(msg.Data); err != nil {
			return errors.Wrap(err, "req unmasrshal")
		}

		if err := srv.Registrate(ctx, &buf.req, &buf.rply); err != nil {
			return errors.Wrap(err, "unexpected error on registrate")
		}

		data, err := buf.rply.Marshal()
		if err != nil {
			return errors.Wrap(err, "rply marshal")
		}

		if err := pbl(msg.Reply, data); err != nil {
			return errors.Wrap(err, "publish")
		}

		return nil
	}
}

type registrater interface {
	Registrate(ctx context.Context, req *proto.UserRequest, rply *proto.UserReply) error
}

// Registrater holds everithing required for registration.
// TODO(romanyx): implement logic of the registration this
// struct now is only stub.
type Registrater struct{}

// Registrate regisrates user and sets rply fields.
func (r Registrater) Registrate(ctx context.Context, req *proto.UserRequest, rply *proto.UserReply) error {
	ctx, span := trace.StartSpan(ctx, "registrate")
	defer span.End()

	rply.Email = req.Email
	return nil
}
