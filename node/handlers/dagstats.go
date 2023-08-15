package handlers

import (
	"context"

	coreiface "github.com/ipfs/boxo/coreiface"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/boxo/ipld/merkledag/traverse"
	"github.com/ipfs/go-cid"
	"go.uber.org/atomic"
)

const (
	DagStatsCtxKey = "ctx_dagstats"
)

func getDagStatsFromCtx(ctx context.Context) *DagStats {
	ds, ok := ctx.Value(DagStatsCtxKey).(*DagStats)
	if !ok {
		return nil
	}
	return ds
}

type DagStats struct {
	DeduplicatedSize      *atomic.Int64
	DeduplicatedNumBlocks *atomic.Int64
	TotalSize             *atomic.Int64
	TotalNumBlocks        *atomic.Int64
}

func NewDagStats() *DagStats {
	return &DagStats{
		DeduplicatedSize:      atomic.NewInt64(0),
		DeduplicatedNumBlocks: atomic.NewInt64(0),
		TotalSize:             atomic.NewInt64(0),
		TotalNumBlocks:        atomic.NewInt64(0),
	}
}

func CalculateDagStats(
	ctx context.Context,
	coreapi coreiface.CoreAPI,
	k cid.Cid,
	recursive bool,
	stats *DagStats,
) error {
	nodeGetter := merkledag.NewSession(ctx, coreapi.Dag())
	root, err := nodeGetter.Get(ctx, k)
	if err != nil {
		return err
	}

	if !recursive {
		if stats != nil {
			sz := int64(len(root.RawData()))
			if stats.DeduplicatedSize != nil {
				stats.DeduplicatedSize.Store(sz)
			}
			if stats.DeduplicatedNumBlocks != nil {
				stats.DeduplicatedNumBlocks.Store(1)
			}
			if stats.TotalSize != nil {
				stats.TotalSize.Store(sz)
			}
			if stats.TotalNumBlocks != nil {
				stats.TotalNumBlocks.Store(1)
			}
		}
		return nil
	}

	seen := cid.NewSet()
	return traverse.Traverse(root, traverse.Options{
		DAG:   nodeGetter,
		Order: traverse.DFSPre,
		Func: func(st traverse.State) error {
			if stats != nil {
				sz := int64(len(st.Node.RawData()))
				if !seen.Has(st.Node.Cid()) {
					if stats.DeduplicatedSize != nil {
						stats.DeduplicatedSize.Add(sz)
					}
					if stats.DeduplicatedNumBlocks != nil {
						stats.DeduplicatedNumBlocks.Inc()
					}
				}
				seen.Add(st.Node.Cid())

				if stats.TotalSize != nil {
					stats.TotalSize.Add(sz)
				}
				if stats.TotalNumBlocks != nil {
					stats.TotalNumBlocks.Inc()
				}
			}

			return nil
		},
		ErrFunc:        nil,
		SkipDuplicates: false,
	})
}
