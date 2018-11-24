package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync/atomic"

	"github.com/nats-io/go-nats-streaming"
	"github.com/pkg/errors"
	natsCli "github.com/romanyx/nats_example/internal/nats"
	"github.com/romanyx/nats_example/internal/process"
	"github.com/romanyx/nats_example/proto"
	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
)

// setupNatsQueue register given queue on the subject with the handler.
func setupNatsQueue(nsc stan.Conn, w io.Writer, subj, queue, durableName string) *natsCli.Stream {
	natsStream := natsCli.NewStreamCli(nsc)

	mdlw := natsCli.NewStreamChain(metricsMiddleware)

	var sq Sequence
	srv := processWithWriter{Writer: w}
	h := errCatchWrapper(process.NewNatsHandler(srv))
	if err := natsStream.QueueFunc(subj, queue, durableName, &sq, mdlw.Then(h)); err != nil {
		log.Fatalf("nats reply func for %s: %v", subj, err)
	}

	return natsStream
}

func errCatchWrapper(h process.PolyHandler) natsCli.StreamHandler {
	wrp := func(ctx context.Context, msg *stan.Msg) {
		ctx, span := trace.StartSpan(ctx, "errorsWrapper")
		defer span.End()

		defer func() {
			if r := recover(); r != nil {
				stats.Record(ctx, natsPanicsCount.M(1))
				log.Printf("trace: %s, panic on subscription reg: %v", span.SpanContext().TraceID, r)
				span.SetStatus(trace.Status{Code: trace.StatusCodeInternal, Message: fmt.Sprint(r)})
			}
		}()

		m := polyMsg{msg}
		if err := h(ctx, m); err != nil {
			stats.Record(ctx, queueErrorsCount.M(1))
			span.SetStatus(trace.Status{Code: trace.StatusCodeInternal, Message: err.Error()})
			err := errors.WithStack(err)
			log.Printf("trace: %s, error to respond on subscription reg: %+v", span.SpanContext().TraceID, err)
		}
	}

	return wrp
}

type polyMsg struct {
	msg *stan.Msg
}

func (m polyMsg) Data() []byte {
	return m.msg.Data
}

func metricsMiddleware(next natsCli.StreamHandler) natsCli.StreamHandler {
	h := func(ctx context.Context, msg *stan.Msg) {
		ctx, span := trace.StartSpan(ctx, "metricsMiddleware")
		defer span.End()

		next(ctx, msg)

		// TODO(romanyx): add other metrics.
		stats.Record(ctx, natsRequestsCount.M(1))
	}

	return h
}

// Sequence used to check exactly once processing.
type Sequence struct {
	l uint64
}

// Last returns last sequence.
func (s *Sequence) Last() uint64 {
	return s.l
}

// Swap previous sequences with the new one.
func (s *Sequence) Swap(ns uint64) {
	atomic.SwapUint64(&s.l, ns)
}

type processWithWriter struct {
	Writer io.Writer
}

// Process holds business logic for processWithWriter
// implementation for job processer.
func (p processWithWriter) Process(ctx context.Context, job *proto.JobRequest) error {
	ctx, span := trace.StartSpan(ctx, "processWithWriter")
	defer span.End()

	if _, err := fmt.Fprintf(p.Writer, "job %d has been processed\n", job.Id); err != nil {
		return errors.Wrap(err, "unable to write")
	}

	return nil

}
