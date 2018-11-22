package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/heptiolabs/healthcheck"
	"github.com/nats-io/go-nats"
	"github.com/nats-io/go-nats-streaming"
	"github.com/pkg/errors"
	natsCli "github.com/romanyx/nats_example/internal/nats"
	"github.com/romanyx/nats_example/internal/process"
	"go.opencensus.io/examples/exporter"
	"go.opencensus.io/exporter/prometheus"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
)

const (
	natsConnCheckInterval = 5 * time.Second
	metricsReportPeriod   = 3 * time.Second
	shutdownTimeout       = 15 * time.Second
)

var (
	queueErrorsCount  = stats.Int64("queue_errors_count", "number of errors on subscribe reply", stats.UnitDimensionless)
	natsPanicsCount   = stats.Int64("nats_panics_count", "number of nats panics", stats.UnitDimensionless)
	natsRequestsCount = stats.Int64("nats_ruqests_count", "number of nats requests", stats.UnitDimensionless)
)

func main() {
	var (
		natsURL     = flag.String("nats", "demo.nats.io", "url of NATS server")
		clusterID   = flag.String("cluster", "test-cluster", "NATS Stream cluster ID")
		clientID    = flag.String("client", "test-client", "NATS Stream client unique ID")
		subj        = flag.String("subj", "test.jobs", "nats subject")
		queue       = flag.String("queue", "queue", "nats subject queue name")
		healthAddr  = flag.String("health", ":8081", "health check addr")
		metricsAddr = flag.String("metrics", ":8082", "prometheus metrics addr")
	)
	flag.Parse()

	// Make a channel for errors.
	errChan := make(chan error)

	// Health checker handler.
	health := healthcheck.NewHandler()

	// Register metrics exporter.
	pex, err := prometheus.NewExporter(prometheus.Options{})
	if err != nil {
		log.Fatalf("prometheus exporter: %v\n", err)
	}
	view.RegisterExporter(pex)

	if err := view.Register(
		&view.View{
			Name:        "nats errors on queue",
			Description: "number of errors in queue",
			Measure:     queueErrorsCount,
			Aggregation: view.Count(),
		},
		&view.View{
			Name:        "nats requests",
			Description: "number of requests from nats",
			Measure:     natsRequestsCount,
			Aggregation: view.Count(),
		},
		&view.View{
			Name:        "nats panics",
			Description: "number of panics processing nats messages",
			Measure:     natsPanicsCount,
			Aggregation: view.Count(),
		},
	); err != nil {
		log.Fatalf("failed to register subscribe reply errors view: %v\n", err)
	}

	view.SetReportingPeriod(metricsReportPeriod)

	// Build and start metrics server.
	mux := http.NewServeMux()
	mux.Handle("/metrics", pex)
	metricsServer := http.Server{
		Addr:    *metricsAddr,
		Handler: mux,
	}

	go func() {
		if err := metricsServer.ListenAndServe(); err != nil {
			errChan <- errors.Wrap(err, "metrics server")
		}
	}()

	// Register trace exporter.
	// TODO(romanyx): export to Jaeger.
	e := &exporter.PrintExporter{}
	trace.RegisterExporter(e)

	// Always trace for this demo. In a production application, you should
	// configure this to a trace.ProbabilitySampler set at the desired
	// probability.
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})

	// Connect to NATS and subscribe to the subjects.
	// TODO(romanyx): move queue subscription to factory
	// func for future integration tests.
	opts := []stan.Option{
		stan.NatsURL(*natsURL),
	}

	natsStream, err := natsCli.NewStreamCli(*clusterID, *clientID, opts)
	if err != nil {
		log.Fatalf("nats client: %v\n", err)
	}

	mdlw := natsCli.NewStreamChain(metricsMiddleware)

	// Sequence must be retrieved from storage to be valid one.
	var sq Sequence
	srv := process.Service{}
	h := errorCatchWrapper(process.NewHandler(srv))
	if err := natsStream.QueueFunc(*subj, *queue, &sq, mdlw.Then(h)); err != nil {
		log.Fatalf("nats reply func for %s: %v", *subj, err)
	}

	// Add NATS status checks.
	natsConnCheck := healthcheck.Check(func() error {
		if status := natsStream.Status(); status != nats.CONNECTED {
			return errors.Errorf("connection status %d", status)
		}

		return nil
	})

	health.AddReadinessCheck("NATS connection ready", natsConnCheck)
	health.AddLivenessCheck("NATS connection live", healthcheck.Async(natsConnCheck, natsConnCheckInterval))

	// Build and start healt server.
	healthServer := http.Server{
		Addr:    *healthAddr,
		Handler: health,
	}

	go func() {
		if err := healthServer.ListenAndServe(); err != nil {
			errChan <- errors.Wrap(err, "health server")
		}
	}()

	// Make a channel to listen for an interrupt or terminate signal from the OS.
	// Use a buffered channel because the signal package requires it.
	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errChan:
		log.Fatalf("critical error: %v\n", err)
	case <-osSignals:
		log.Println("stopping by signal")

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := natsStream.Close(ctx); err != nil {
			log.Fatalf("gracefull shutdown failed: %v", err)
		}
	}
}

type handleFunc func(context.Context, *stan.Msg) error

func errorCatchWrapper(h handleFunc) natsCli.StreamHandler {
	wrp := func(ctx context.Context, msg *stan.Msg) {
		ctx, span := trace.StartSpan(ctx, "errorsWrapper")
		defer span.End()

		defer func() {
			if r := recover(); r != nil {
				stats.Record(ctx, natsPanicsCount.M(1))
				log.Printf("trace: %s, panic on subscription reg", span.SpanContext().TraceID)
				span.SetStatus(trace.Status{Code: trace.StatusCodeInternal, Message: "recover panic"})
			}
		}()

		if err := h(ctx, msg); err != nil {
			stats.Record(ctx, queueErrorsCount.M(1))
			span.SetStatus(trace.Status{Code: trace.StatusCodeInternal, Message: err.Error()})
			err := errors.WithStack(err)
			log.Printf("trace: %s, error to respond on subscription reg: %+v", span.SpanContext().TraceID, err)
		}
	}

	return wrp
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

// Swap previous sequences to new one.
func (s *Sequence) Swap(ns uint64) {
	atomic.SwapUint64(&s.l, ns)
}
