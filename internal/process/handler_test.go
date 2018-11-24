package process

import (
	"context"
	"testing"

	"github.com/nats-io/go-nats-streaming"
	"github.com/nats-io/go-nats-streaming/pb"
	"github.com/pkg/errors"
	"github.com/romanyx/nats_example/proto"
	"github.com/stretchr/testify/assert"
)

func Test_NatsHandler(t *testing.T) {
	tests := []struct {
		name        string
		job         proto.JobRequest
		processFunc func(ctx context.Context, job *proto.JobRequest) error
		wantErr     bool
	}{
		{
			name: "ok",
			job: proto.JobRequest{
				Id: 1,
			},
			processFunc: func(ctx context.Context, job *proto.JobRequest) error {
				return nil
			},
		},
		{
			name: "process err",
			processFunc: func(ctx context.Context, job *proto.JobRequest) error {
				return errors.New("mock error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := tt.job.Marshal()
			if err != nil {
				assert.Nil(t, err)
				return
			}

			h := NewNatsHandler(processerFunc(tt.processFunc))

			msg := stan.Msg{
				MsgProto: pb.MsgProto{
					Data: data,
				},
			}

			ctx := context.Background()
			err = h(ctx, &msg)

			if tt.wantErr {
				assert.NotNil(t, err)
				return
			}

			assert.Nil(t, err)
		})
	}
}

func Benchmark_NatsHandler(b *testing.B) {
	h := NewNatsHandler(processerFunc(func(context.Context, *proto.JobRequest) error {
		return nil
	}))

	ctx := context.Background()
	job := proto.JobRequest{
		Id: 1,
	}
	data, err := job.Marshal()
	if err != nil {
		assert.Nil(b, err)
		return
	}

	msg := stan.Msg{
		MsgProto: pb.MsgProto{
			Data: data,
		},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		h(ctx, &msg)
	}

	b.ReportAllocs()
}

type processerFunc func(context.Context, *proto.JobRequest) error

func (f processerFunc) Process(c context.Context, jr *proto.JobRequest) error {
	return f(c, jr)
}
