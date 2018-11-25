package main

import (
	"flag"
	"log"
	"os"
	"testing"

	"github.com/nats-io/go-nats-streaming"
	"github.com/ory/dockertest"
	"github.com/romanyx/nats_example/internal/docker"
)

var (
	natsStreamConn stan.Conn
)

func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		os.Exit(m.Run())
	}

	// uses a sensible default on windows (tcp/http) and linux/osx (socket).
	pool, err := dockertest.NewPool("")
	if err != nil {
		// TODO(romanyx): don't just fatal, set short mode manualy
		// print warning and continue testing in short mode.
		log.Fatalf("could not connect to docker: %v", err)
	}

	natsDocker, err := docker.NewNatsStream(pool)
	if err != nil {
		log.Fatalf("prepare nats stream connection with docker: %v", err)
	}

	natsStreamConn = natsDocker.Conn

	code := m.Run()

	if err := pool.Purge(natsDocker.Resource); err != nil {
		log.Fatalf("could not purge nats stream docker: %v", err)
	}

	os.Exit(code)
}
