package promtest

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

func TestPrometheusMockClient_Query(t *testing.T) {
	type fields struct {
		responses map[string]response
	}
	type args struct {
		ctx context.Context
		q   string
		t   time.Time
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    model.Value
		wantErr bool
	}{
		{
			name: "undefined query",
			fields: fields{
				responses: map[string]response{},
			},
			args: args{
				ctx: context.Background(),
				q:   "",
				t:   time.Now(),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PrometheusMockClient{
				responses: tt.fields.responses,
			}
			got, err := p.Query(tt.args.ctx, tt.args.q, tt.args.t)
			if (err != nil) != tt.wantErr {
				t.Errorf("PrometheusMockClient.Query() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PrometheusMockClient.Query() = %v, want %v", got, tt.want)
			}
		})
	}
}
