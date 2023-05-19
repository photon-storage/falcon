package node

import (
	"context"
	"time"

	"github.com/photon-storage/go-common/metrics"
)

// initMetrics register the metrics to prometheus.
func initMetrics(ctx context.Context, port int) {
	metrics.Init(ctx, "p3_falcon", port)
	metrics.NewGauge("restart_at_seconds")

	metrics.RegisterDiskMetrics(ctx)
	// Put request.
	metrics.NewCounter("ingress_bytes")
	metrics.NewCounter("egress_bytes")
	metrics.NewCounter("request_call_total")
	metrics.NewCounter("request_log_err_total")

	metrics.GaugeSet("restart_at_seconds", float64(time.Now().Unix()))
}
