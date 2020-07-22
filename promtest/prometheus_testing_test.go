package promtest

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

var (
	testQuery    = "sum_over_time(probe_success{instance=~\"s1.*\", module=\"icmp\"}[15m]) == 0"
	testResponse = model.Vector{
		CreateSample(map[string]string{
			"instance": "s1.iad0t.measurement-lab.org",
			"job":      "blackbox-targets",
			"module":   "icmp",
			"site":     "iad0t",
		}, 0, model.Time(time.Now().Unix())),
	}
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
		name      string
		responses map[string]response
		args      args
		want      model.Value
		wantErr   bool
	}{
		{
			name: "success",
			responses: map[string]response{
				testQuery: {
					value: testResponse,
					err:   nil,
				},
			},
			args: args{
				ctx: context.Background(),
				q:   testQuery,
				t:   time.Now(),
			},
			want: testResponse,
		},
		{
			name:      "error-undefined-query",
			responses: map[string]response{},
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
				responses: tt.responses,
			}

			got, _, err := p.Query(tt.args.ctx, tt.args.q, tt.args.t)
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

func TestPrometheusMockClient_Unregister(t *testing.T) {

	responses := map[string]response{
		testQuery: {
			value: testResponse,
			err:   nil,
		},
	}

	t.Run("success", func(t *testing.T) {
		p := &PrometheusMockClient{
			responses: responses,
		}

		got := p.Unregister(testQuery)

		if _, ok := p.responses[testQuery]; ok {
			t.Error("PrometheusMockClient.Unregister() did not unregister the query.")
		}

		gotErr := p.Unregister("invalid-query")
		if gotErr == nil {
			t.Error("PrometheusMockClient.Unregister() did not return a function.")
		}

		// Register the query again
		got()

		if _, ok := p.responses[testQuery]; !ok {
			t.Error("PrometheusMockClient.Unregister() the function did not register the query again.")
		}
	})

}

func TestNewPrometheusMockClient(t *testing.T) {
	p := NewPrometheusMockClient()
	if p == nil || p.responses == nil {
		t.Errorf("NewPrometheusMockClient() did not initialize PrometheusMockClient correctly.")
	}
}
