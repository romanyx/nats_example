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
	"github.com/nats-io/go-nats-streaming"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
	"go.opencensus.io/exporter/jaeger"
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
	version           = "unset"
	queueErrorsCount  = stats.Int64("queue_errors_count", "number of errors on subscribe reply", stats.UnitDimensionless)
	natsPanicsCount   = stats.Int64("nats_panics_count", "number of nats panics", stats.UnitDimensionless)
	natsRequestsCount = stats.Int64("nats_ruqests_count", "number of nats requests", stats.UnitDimensionless)
)

func main() {
	fmt.Printf("version: %s\n", version)

	var (
		natsURL     = flag.String("nats", "demo.nats.io", "url of NATS server")
		rabbitURL   = flag.String("rabbit", "amqp://guest:guest@127.0.0.1:5672/", "url of NATS server")
		clusterID   = flag.String("cluster", "test-cluster", "NATS Stream cluster ID")
		clientID    = flag.String("client", "test-client", "NATS Stream client unique ID")
		subj        = flag.String("subj", "test.jobs", "nats subject")
		queue       = flag.String("queue", "queue", "nats subject queue name")
		durableName = flag.String("durableName", "dirable-name", "nats durable name for client")
		healthAddr  = flag.String("health", ":8081", "health check addr")
		metricsAddr = flag.String("metrics", ":8082", "prometheus metrics addr")
		jaegerURL   = flag.String("jaeger", "http://127.0.0.1:14268", "jaeger server url")
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
	e, err := jaeger.NewExporter(jaeger.Options{
		Endpoint:    *jaegerURL,
		ServiceName: "queue",
	})
	if err != nil {
		log.Fatalf("failed to create jaeger exporter: %v", err)
	}
	defer e.Flush()
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

	sc, err := stan.Connect(*clusterID, *clientID, opts...)
	if err != nil {
		log.Fatalf("nats conn: %v\n", err)
	}

	natsStream := setupNatsQueue(sc, os.Stdout, *subj, *queue, *durableName)

	// Add NATS status checks.
	natsConnCheck := healthcheck.Check(func() error {
		if status := natsStream.Status(); status != nats.CONNECTED {
			return errors.Errorf("connection status %d", status)
		}

		return nil
	})

	health.AddReadinessCheck("NATS connection ready", natsConnCheck)
	health.AddLivenessCheck("NATS connection live", healthcheck.Async(natsConnCheck, natsConnCheckInterval))

	// Add RabbitMQ queue.
	rc, err := amqp.Dial(*rabbitURL)
	if err != nil {
		log.Fatalf("rabbit conn: %v\n", err)
	}
	defer rc.Close() // TODO(romanyx): handle gracefuly.

	setupRabbitQueue(rc, os.Stdout, *queue)

	// TODO(romanyx): Add rabbitmq live checks.

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
