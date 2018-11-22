package process

import (
	"context"
	"sync"

	"github.com/nats-io/go-nats-streaming"
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

// NewHandler returns handle func for nats messages handling.
func NewHandler(srv Processer) func(context.Context, *stan.Msg) error {
	h := func(ctx context.Context, msg *stan.Msg) error {
		ctx, span := trace.StartSpan(ctx, "handler")
		defer span.End()

		buf := bufferPool.Get().(*buffer)
		defer buf.Clear()

		if err := buf.job.Unmarshal(msg.Data); err != nil {
			return errors.Wrap(err, "req unmasrshal")
		}

		if err := srv.Process(ctx, &buf.job); err != nil {
			return errors.Wrap(err, "unexpected error on process")
		}

		return nil
	}

	return h
}

// Processer interface represents job processer.
type Processer interface {
	Process(context.Context, *proto.JobRequest) error
}

// Service object holds logic of job processing.
type Service struct{}

// Process processes job.
func (s Service) Process(ctx context.Context, job *proto.JobRequest) error {
	ctx, span := trace.StartSpan(ctx, "processer")
	defer span.End()

	return nil
}
