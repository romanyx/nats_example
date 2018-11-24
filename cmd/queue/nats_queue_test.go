package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/nats-io/go-nats-streaming"
	"github.com/romanyx/nats_example/internal/process"
	"github.com/romanyx/nats_example/proto"
	"github.com/stretchr/testify/assert"
)

const (
	caseWaitTimeout = 3 * time.Second
)

func TestQueue(t *testing.T) {
	// Skip integration test on short mode.
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	t.Log("After setup of nats queue should start to process queue.")
	{
		var buf bytes.Buffer
		subj := fmt.Sprintf("test-sub-%d", time.Now().UnixNano())
		queue := fmt.Sprintf("%s-queue", subj)
		durableName := fmt.Sprintf("%s-durableName", queue)
		natsStream := setupNatsQueue(natsStreamConn, &buf, subj, queue, durableName)

		job := proto.JobRequest{
			Id: 1,
		}

		data, err := job.Marshal()
		if err != nil {
			t.Errorf("failed to marshal job: %v", err)
			return
		}

		jobDoneChan := make(chan struct{})
		natsStream.SetQueueHandleDone(func() {
			jobDoneChan <- struct{}{}
		})

		t.Logf("\tTest 0:\t When receive valid message should process it.")
		{
			err := natsStream.Publish(subj, data)
			assert.Nil(t, err)

			select {
			case <-jobDoneChan:
			case <-time.After(caseWaitTimeout):
				t.Errorf("timeout on job done wait")
			}

			expect := "job 1 has been processed\n"
			assert.Equal(t, expect, buf.String())
			buf.Reset()
		}
	}
}

func Test_errCatchWrapper(t *testing.T) {
	tests := []struct {
		name       string
		handleFunc func(context.Context, process.Data) error
		expect     string
	}{
		{
			name: "should log error",
			handleFunc: func(context.Context, process.Data) error {
				return errors.New("mock error")
			},
			expect: "mock error",
		},
		{
			name: "should recover panic",
			handleFunc: func(context.Context, process.Data) error {
				panic("mock panic")
			},
			expect: "mock panic",
		},
		{
			name: "should skip without error",
			handleFunc: func(context.Context, process.Data) error {
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log.SetOutput(&buf)

			wrp := errCatchWrapper(tt.handleFunc)
			ctx := context.Background()

			wrp(ctx, new(stan.Msg))
			assert.Contains(t, buf.String(), tt.expect)
		})
	}
}

func Test_Sequence_Swap(t *testing.T) {
	var s Sequence
	var got uint64 = 64
	s.Swap(got)
	assert.Equal(t, got, s.Last())
}
