package process

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/romanyx/nats_example/proto"
	"github.com/stretchr/testify/assert"
)

func Test_NatsHandler(t *testing.T) {
	job := proto.JobRequest{
		Id: 1,
	}

	data, err := job.Marshal()
	if err != nil {
		t.Errorf("failed to mastshal job: %v", err)
	}

	tests := []struct {
		name        string
		dataFunc    func() []byte
		processFunc func(ctx context.Context, job *proto.JobRequest) error
		wantErr     bool
	}{
		{
			name: "ok",
			dataFunc: func() []byte {
				return data
			},
			processFunc: func(ctx context.Context, job *proto.JobRequest) error {
				return nil
			},
		},
		{
			name: "unmarshal err",
			dataFunc: func() []byte {
				return []byte("\n\n\n")
			},
			processFunc: func(ctx context.Context, job *proto.JobRequest) error {
				return errors.New("mock error")
			},
			wantErr: true,
		},
		{
			name: "process err",
			dataFunc: func() []byte {
				return data
			},
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

			h := NewNatsHandler(processerFunc(tt.processFunc))

			ctx := context.Background()
			err = h(ctx, dataFunc(tt.dataFunc))

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

	msg := dataFunc(func() []byte {
		return data
	})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		h(ctx, &msg)
	}

	b.ReportAllocs()
}

type dataFunc func() []byte

func (f dataFunc) Data() []byte {
	return f()
}

type processerFunc func(context.Context, *proto.JobRequest) error

func (f processerFunc) Process(c context.Context, jr *proto.JobRequest) error {
	return f(c, jr)
}
