// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/metric"
)

// The default time buckets for latency metrics.
// The unit can however change for different units i.e. for one metric the unit could be microseconds and for another it could be milliseconds.
var defaultLatencyDistribution = metric.WithExplicitBucketBoundaries(1, 2, 3, 4, 5, 6, 8, 10, 13, 16, 20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500, 650, 800, 1000, 2000, 5000, 10000, 20000, 50000, 100000)

// Pair of CacheHit and ReadType attributes
type CacheHitReadType struct {
	CacheHit bool
	ReadType string
}

// Pair of FSOp and ErrorCategory attributes
type FSOpsErrorCategory struct {
	FSOps         string
	ErrorCategory string
}

// MetricAttr represents the attributes associated with a metric.
type MetricAttr struct {
	Key, Value string
}

func (a *MetricAttr) String() string {
	return fmt.Sprintf("Key: %s, Value: %s", a.Key, a.Value)
}

type GCSMetricHandle interface {
	GCSReadBytesCount(ctx context.Context, inc int64)
	GCSReaderCount(ctx context.Context, inc int64, ioMethod string)
	GCSRequestCount(ctx context.Context, inc int64, gcsMethod string)
	GCSRequestLatency(ctx context.Context, latency time.Duration, gcsMethod string)
	GCSReadCount(ctx context.Context, inc int64, readType string)
	GCSDownloadBytesCount(ctx context.Context, inc int64, readType string)
	GCSRetryCount(ctx context.Context, inc int64, gcsRetryErr string)
}

type OpsMetricHandle interface {
	OpsCount(ctx context.Context, inc int64, fsOp string)
	OpsLatency(ctx context.Context, latency time.Duration, fsOp string)
	OpsErrorCount(ctx context.Context, inc int64, attrs FSOpsErrorCategory)
}

type FileCacheMetricHandle interface {
	FileCacheReadCount(ctx context.Context, inc int64, attrs CacheHitReadType)
	FileCacheReadBytesCount(ctx context.Context, inc int64, readType string)
	FileCacheReadLatency(ctx context.Context, latency time.Duration, cacheHit bool)
}
type MetricHandle interface {
	GCSMetricHandle
	OpsMetricHandle
	FileCacheMetricHandle
}

func CaptureGCSReadMetrics(ctx context.Context, metricHandle MetricHandle, readType string, requestedDataSize int64) {
	metricHandle.GCSReadCount(ctx, 1, readType)
	metricHandle.GCSDownloadBytesCount(ctx, requestedDataSize, readType)
}
