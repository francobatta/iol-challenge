// Package prom asks a Prometheus server for the values of metrics.
package prom

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	"github.com/francobatta/iol-challenge/notification-system/api/internal/insight"
)

// A Client evaluates PromQL on one Prometheus server. It is safe for concurrent use.
type Client struct {
	api v1.API
}

// NewClient returns a Client for the Prometheus at baseURL. It does not connect.
func NewClient(baseURL string) (*Client, error) {
	c, err := api.NewClient(api.Config{Address: baseURL})
	if err != nil {
		return nil, fmt.Errorf("prom: %v", err)
	}
	return &Client{api: v1.NewAPI(c)}, nil
}

// Instant returns the value of each series of query at the given moment. Values that are
// not finite numbers are left out.
func (c *Client) Instant(ctx context.Context, query string, at time.Time) ([]insight.Sample, error) {
	// Warnings are about the query's efficiency, and the queries are fixed.
	value, _, err := c.api.Query(ctx, query, at)
	if err != nil {
		return nil, fmt.Errorf("prom: query %q: %v", query, err)
	}
	vector, ok := value.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("prom: query %q returned a %s, want a vector", query, value.Type())
	}
	samples := make([]insight.Sample, 0, len(vector))
	for _, s := range vector {
		if v := float64(s.Value); finite(v) {
			samples = append(samples, insight.Sample{Labels: labels(s.Metric), Value: v})
		}
	}
	return samples, nil
}

// Range returns the values of each series of query from start to end, one every step.
// Values that are not finite numbers are left out.
func (c *Client) Range(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]insight.Series, error) {
	value, _, err := c.api.QueryRange(ctx, query, v1.Range{Start: start, End: end, Step: step})
	if err != nil {
		return nil, fmt.Errorf("prom: query %q: %v", query, err)
	}
	matrix, ok := value.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("prom: query %q returned a %s, want a matrix", query, value.Type())
	}
	series := make([]insight.Series, 0, len(matrix))
	for _, stream := range matrix {
		points := make([]insight.Point, 0, len(stream.Values))
		for _, p := range stream.Values {
			if v := float64(p.Value); finite(v) {
				points = append(points, insight.Point{float64(p.Timestamp) / 1000, v})
			}
		}
		series = append(series, insight.Series{Labels: labels(stream.Metric), Points: points})
	}
	return series, nil
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func labels(metric model.Metric) map[string]string {
	labels := make(map[string]string, len(metric))
	for name, value := range metric {
		labels[string(name)] = string(value)
	}
	return labels
}
