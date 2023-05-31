package node

import (
	"context"
	"time"

	"github.com/ipfs/kubo/core"

	"github.com/photon-storage/go-common/metrics"
)

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

			if p, ok := nd.Pinning.(*wrappedPinner); ok {
				metrics.GaugeSet(
					"pinned_count_total",
					float64(p.getPinnedCount()),
				)
			}
		}
	}
}
