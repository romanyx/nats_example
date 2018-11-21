package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/heptiolabs/healthcheck"
	"github.com/nats-io/go-nats"
	"github.com/pkg/errors"
	"github.com/romanyx/nats_example/internal/reg"
	"go.opencensus.io/examples/exporter"
	"go.opencensus.io/exporter/prometheus"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
)

const (
	natsConnCheckInterval = 5 * time.Second
	metricsReportPeriod   = 3 * time.Second
)

var (
	subscribeReplyErrorsCount = stats.Int64("example.com/measures/subscribe_reply_errors_count", "number of errors on subscribe reply", stats.UnitDimensionless)
	natsPanicsCount           = stats.Int64("example.com/measures/nats_panics_count", "number of nats panics", stats.UnitDimensionless)
	natsRequestsCount         = stats.Int64("example.com/measures/nats_ruqests_count", "number of nats requests", stats.UnitDimensionless)
)

func main() {
	var (
		natsURL     = flag.String("nats", "demo.nats.io", "url of NATS server")
		subj        = flag.String("subj", "test.registrate", "nats subject")
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
			Name:        "nats errors on subscribe",
			Description: "number of errors in subscribe reply",
			Measure:     subscribeReplyErrorsCount,
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
	opts := []nats.Option{nats.Name("NATS Sample Responder")}
	natsConn, natsTeardown := connectNATS(*natsURL, *subj, opts)

	// Add NATS status checks.
	natsConnCheck := healthcheck.Check(func() error {
		if status := natsConn.Status(); status != nats.CONNECTED {
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

		if err := natsTeardown(); err != nil {
			log.Printf("unsubscribe: %v\n", err)
		}
	}
}

type handleFunc func(context.Context, *nats.Msg) error

func newHandle(h handleFunc, subj string) func(*nats.Msg) {
	return func(msg *nats.Msg) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ctx, span := trace.StartSpan(ctx, fmt.Sprintf("nats.rply.%s", subj))
		defer span.End()

		defer func() {
			if r := recover(); r != nil {
				stats.Record(ctx, natsPanicsCount.M(1))
				log.Printf("trace: %s, panic on subscription %s", span.SpanContext().TraceID, subj)
			}
		}()

		if err := h(ctx, msg); err != nil {
			stats.Record(ctx, subscribeReplyErrorsCount.M(1))
			span.SetStatus(trace.Status{Code: trace.StatusCodeInternal, Message: err.Error()})
			err := errors.WithStack(err)
			log.Printf("trace: %s, error to respond on subscription %s: %+v", span.SpanContext().TraceID, subj, err)
		}

		stats.Record(ctx, natsRequestsCount.M(1))
	}
}

func connectNATS(url string, subj string, opts []nats.Option) (*nats.Conn, func() error) {
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		log.Fatalf("connect to NATS: %v", err)
	}

	// Build replier for registration subscription and
	// registrate async handler for it on NATS connection.
	// TODO(romanyx): use middlewares pattern for errors, panics
	// and metrics.
	srv := reg.Registrater{}
	h := reg.NewHandler(srv, nc.Publish)
	sub, err := nc.Subscribe(subj, newHandle(h, subj))
	if err != nil {
		log.Fatalf("subscribe to %s failed: %v\n", subj, err)
	}

	return nc, func() error {
		nc.Flush()
		if err := sub.Unsubscribe(); err != nil {
			return errors.Wrapf(err, "unsubscribe subj %s: %v", subj, err)
		}
		nc.Close()

		return nil
	}
}
