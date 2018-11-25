package process

import (
	"context"
	"sync"

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
	job proto.JobRequest
}

func (b *buffer) Clear() {
	b.job.Id = 0
	bufferPool.Put(b)
}

// Processer is a job processer.
type Processer interface {
	Process(context.Context, *proto.JobRequest) error
}

// Data interface represents content of
// the incoming message.
type Data interface {
	Data() []byte
}

// PolyHandler is a polymorphic handler
// to perform any messaging queue.
type PolyHandler func(context.Context, Data) error

// NewNatsHandler returns handle func for nats messages handling.
func NewNatsHandler(srv Processer) PolyHandler {
	h := func(ctx context.Context, d Data) error {
		ctx, span := trace.StartSpan(ctx, "handler")
		defer span.End()

		buf := bufferPool.Get().(*buffer)
		defer buf.Clear()

		if err := buf.job.Unmarshal(d.Data()); err != nil {
			return errors.Wrap(err, "req unmasrshal")
		}

		if err := srv.Process(ctx, &buf.job); err != nil {
			return errors.Wrap(err, "unexpected error on process")
		}

		return nil
	}

	return h
}
