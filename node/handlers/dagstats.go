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
	dagStatsCtxKey = "ctx_dagstats"
)

func getDagStatsFromCtx(ctx context.Context) *DagStats {
	v := ctx.Value(dagStatsCtxKey)
	if v == nil {
		return nil
	}
	ds, ok := v.(*DagStats)
	if !ok {
		return nil
	}
	return ds
}

func WithDagStat(ctx context.Context, v *DagStats) context.Context {
	return context.WithValue(ctx, dagStatsCtxKey, v)
}

type DagStats struct {
	TotalCount            *atomic.Int64
	DeduplicatedSize      *atomic.Int64
	DeduplicatedNumBlocks *atomic.Int64
	TotalSize             *atomic.Int64
	TotalNumBlocks        *atomic.Int64
}

func NewDagStats() *DagStats {
	return &DagStats{
		TotalCount:            atomic.NewInt64(0),
		DeduplicatedSize:      atomic.NewInt64(0),
		DeduplicatedNumBlocks: atomic.NewInt64(0),
		TotalSize:             atomic.NewInt64(0),
		TotalNumBlocks:        atomic.NewInt64(0),
	}
}

func (d *DagStats) Add(o *DagStats) {
	d.TotalCount.Add(o.TotalCount.Load())
	d.DeduplicatedSize.Add(o.DeduplicatedSize.Load())
	d.DeduplicatedNumBlocks.Add(o.DeduplicatedNumBlocks.Load())
	d.TotalSize.Add(o.TotalSize.Load())
	d.TotalNumBlocks.Add(o.TotalNumBlocks.Load())
}

func (d *DagStats) Sub(o *DagStats) {
	d.TotalCount.Sub(o.TotalCount.Load())
	d.DeduplicatedSize.Sub(o.DeduplicatedSize.Load())
	d.DeduplicatedNumBlocks.Sub(o.DeduplicatedNumBlocks.Load())
	d.TotalSize.Sub(o.TotalSize.Load())
	d.TotalNumBlocks.Sub(o.TotalNumBlocks.Load())
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

	stats.TotalCount.Store(1)
	if !recursive {
		sz := int64(len(root.RawData()))
		stats.DeduplicatedSize.Store(sz)
		stats.DeduplicatedNumBlocks.Store(1)
		stats.TotalSize.Store(sz)
		stats.TotalNumBlocks.Store(1)
		return nil
	}

	seen := cid.NewSet()
	return traverse.Traverse(root, traverse.Options{
		DAG:   nodeGetter,
		Order: traverse.DFSPre,
		Func: func(st traverse.State) error {
			sz := int64(len(st.Node.RawData()))
			if !seen.Has(st.Node.Cid()) {
				stats.DeduplicatedSize.Add(sz)
				stats.DeduplicatedNumBlocks.Inc()
			}
			seen.Add(st.Node.Cid())

			stats.TotalSize.Add(sz)
			stats.TotalNumBlocks.Inc()

			return nil
		},
		ErrFunc:        nil,
		SkipDuplicates: false,
	})
}
