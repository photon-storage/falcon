package node

import (
	"context"
	"fmt"
	"time"

	core "github.com/ipfs/kubo/core"

	"github.com/photon-storage/go-common/metrics"

	"github.com/photon-storage/falcon/node/com"
)

// initMetrics register the metrics to prometheus.
func initMetrics(ctx context.Context, port int) {
	metrics.Init(ctx, "p3_falcon", port)
	metrics.NewGauge("restart_at_seconds")

	metrics.RegisterDiskMetrics(ctx)
	metrics.RegisterIfaceMetrics(ctx)

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
	com.RegisterPinnerMetrics()
	metrics.NewGauge("pinned_count_total")
	metrics.NewGauge("connected_peers_total")

	metrics.GaugeSet("restart_at_seconds", float64(time.Now().Unix()))
}

func updateNodeMetrics(
	ctx context.Context,
	nd *core.IpfsNode,
) {
	ticker := time.NewTicker(15 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			connected := nd.PeerHost.Network().Peers()
			metrics.GaugeSet(
				"connected_peers_total",
				float64(len(connected)),
			)

			metrics.GaugeSet(
				"pinned_count_total",
				float64(com.GetRcPinner(nd.Pinning).TotalPinnedCount()),
			)
		}
	}
}
