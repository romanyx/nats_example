package reg

import (
	"context"
	"testing"

	nats "github.com/nats-io/go-nats"
	"github.com/pkg/errors"
	"github.com/romanyx/nats_example/proto"
	"github.com/stretchr/testify/assert"
)

func Test_Handler(t *testing.T) {
	publishFunc := func(got *proto.UserReply) func(string, []byte) error {
		return func(subject string, data []byte) error {
			if err := got.Unmarshal(data); err != nil {
				return errors.Wrap(err, "unmarshal body")
			}
			return nil
		}
	}

	tests := []struct {
		name           string
		ur             proto.UserRequest
		registrateFunc func(ctx context.Context, req *proto.UserRequest, rply *proto.UserReply) error
		wantErr        bool
		expect         proto.UserReply
	}{
		{
			name: "ok",
			ur: proto.UserRequest{
				Email: "work@romanyx.ru",
			},
			registrateFunc: func(ctx context.Context, req *proto.UserRequest, rply *proto.UserReply) error {
				rply.Email = req.Email
				return nil
			},
			expect: proto.UserReply{
				Email: "work@romanyx.ru",
			},
		},
		{
			name: "registrate err",
			registrateFunc: func(ctx context.Context, req *proto.UserRequest, rply *proto.UserReply) error {
				return errors.New("mock error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := tt.ur.Marshal()
			if err != nil {
				assert.Nil(t, err)
				return
			}

			var got proto.UserReply
			h := NewHandler(registraterFunc(tt.registrateFunc), publishFunc(&got))

			msg := nats.Msg{
				Data:    data,
				Subject: "test",
			}

			ctx := context.Background()
			err = h(ctx, &msg)

			if tt.wantErr {
				assert.NotNil(t, err)
				return
			}

			assert.Nil(t, err)
			assert.Equal(t, tt.expect.Email, got.Email)
		})
	}
}

type registraterFunc func(context.Context, *proto.UserRequest, *proto.UserReply) error

func (f registraterFunc) Registrate(ctx context.Context, req *proto.UserRequest, rply *proto.UserReply) error {
	return f(ctx, req, rply)
}
