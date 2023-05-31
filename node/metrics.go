package node

import (
	"context"
	"fmt"
	"time"

	"github.com/photon-storage/go-common/metrics"
)

// initMetrics register the metrics to prometheus.
func initMetrics(ctx context.Context, port int) {
	metrics.Init(ctx, "p3_falcon", port)
	metrics.NewGauge("restart_at_seconds")

	metrics.RegisterDiskMetrics(ctx)

	metrics.NewCounter("ingress_bytes")
	metrics.NewCounter("egress_bytes")
	metrics.NewCounter("request_call_total")
	for _, rl := range rules {
		metrics.NewCounter(fmt.Sprintf(
			"request_blocked_total.rule#%v",
			rl.name,
		))
	}
	metrics.NewCounter("request_served_total")
	metrics.NewCounter("request_log_total")
	metrics.NewCounter("request_log_err_total")

	// Node metrics.
	metrics.NewCounter("rc_pinner_pin_call_total")
	metrics.NewCounter("rc_pinner_pin_err_total")
	metrics.NewCounter("rc_pinner_unpin_call_total")
	metrics.NewCounter("rc_pinner_unpin_err_total")
	metrics.NewCounter("rc_pinner_recursive_keys_call_total")
	metrics.NewCounter("rc_pinner_recursive_keys_err_total")
	metrics.NewGauge("pinned_count_total")
	metrics.NewGauge("connected_peers_total")

	metrics.GaugeSet("restart_at_seconds", float64(time.Now().Unix()))
}
