package docker

import (
	"fmt"
	"net"
	"time"

	"github.com/nats-io/go-nats-streaming"
	"github.com/ory/dockertest"
	"github.com/pkg/errors"
)

const (
	clusterID            = "test-cluster"
	natsConnCheckTimeout = 15 * time.Second
)

// NatsStreamDocker holds connection and
// data to shutdown later.
type NatsStreamDocker struct {
	Conn     stan.Conn
	Resource *dockertest.Resource
}

// NewNatsStream starts nats-stream docker image
// and connects client to the given port.
func NewNatsStream(pool *dockertest.Pool) (*NatsStreamDocker, error) {
	res, err := pool.Run("nats-streaming", "latest", []string{})
	if err != nil {
		return nil, errors.Wrap(err, "start nats-streaming")
	}

	purge := func() {
		pool.Purge(res)
	}

	addr := fmt.Sprintf("127.0.0.1:%s", res.GetPort("4222/tcp"))
	errChan := make(chan error)
	done := make(chan struct{})

	var nsc stan.Conn

	go func() {
		if err := pool.Retry(func() error {
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				return errors.Wrapf(err, "dial tcp %s", addr)
			}
			conn.Close()

			nsc, err = stan.Connect(clusterID, fmt.Sprintf("client-%d", time.Now().UnixNano()), stan.NatsURL(addr))
			if err != nil {
				return errors.Wrap(err, "connect to nats")
			}

			return nil

		}); err != nil {
			errChan <- err
		}

		close(done)
	}()

	select {
	case err := <-errChan:
		purge()
		return nil, errors.Wrap(err, "check connection")
	case <-time.After(natsConnCheckTimeout):
		purge()
		return nil, errors.New("timeout on checking nats connection")
	case <-done:
		close(errChan)
	}

	nsd := NatsStreamDocker{
		Conn:     nsc,
		Resource: res,
	}

	return &nsd, nil
}
