// Package promtest provides a mock implementation of a Prometheus HTTP client
// for testing purposes and some utility functions.
package promtest

import (
	"context"
	"errors"
	"time"

	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/common/model"
)

// PromClient is Prometheus HTTP client's interface
type PromClient interface {
	Query(context.Context, string, time.Time) (model.Value, api.Error)
}

// PrometheusMockClient is a test client that returns fake values only for a
// configurable set of queries. New queries/responses can be added by calling
// Register(string, model.Value).
type PrometheusMockClient struct {
	responses map[string]response
}

type response struct {
	value model.Value
	err   api.Error
}

// NewPrometheusMockClient creates a mock client to test Prometheus queries.
func NewPrometheusMockClient() *PrometheusMockClient {
	var p PrometheusMockClient
	p.responses = make(map[string]response)
	return &p
}

// Register maps a query to the expected model.Value that must be returned.
func (p *PrometheusMockClient) Register(q string, resp model.Value, err api.Error) {
	p.responses[q] = response{
		value: resp,
		err:   err,
	}
}

// Unregister removes a mapped query and returns a function to add it back.
func (p *PrometheusMockClient) Unregister(q string) func() {
	v, ok := p.responses[q]
	if ok {
		delete(p.responses, q)
		return func() {
			p.Register(q, v.value, v.err)
		}
	}

	return func() {}
}

// CreateSample returns a reference to a new model.Sample having labels, value
// and timestamp passed as arguments.
func CreateSample(labels map[string]string, value float64, t model.Time) *model.Sample {
	v := model.Metric(map[model.LabelName]model.LabelValue{})

	for key, val := range labels {
		v[model.LabelName(key)] = model.LabelValue(val)
	}

	return &model.Sample{
		Metric:    v,
		Value:     model.SampleValue(value),
		Timestamp: t,
	}
}

// Query is a mock implementation that returns the model.Value corresponding
// to the query, if any, or an error.
func (p PrometheusMockClient) Query(ctx context.Context, q string, t time.Time) (model.Value, api.Error) {
	resp, ok := p.responses[q]

	if ok {
		return resp.value, nil
	}

	return nil, api.NewErrorAPI(errors.New("Undefined query: "+q), []string{})
}
