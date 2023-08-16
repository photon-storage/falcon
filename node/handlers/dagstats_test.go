package handlers

import (
	"context"
	"testing"

	bs "github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/boxo/exchange/offline"
	"github.com/ipfs/boxo/ipld/merkledag"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"

	"github.com/photon-storage/go-common/testing/require"
)

func TestDagStats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dstore := dssync.MutexWrap(ds.NewMapDatastore())
	bstore := blockstore.NewBlockstore(dstore)
	dserv := merkledag.NewDAGService(bs.New(bstore, offline.Exchange(bstore)))
	// A{B,C{D,E},F{C}}
	a := rndNode(t)
	b := rndNode(t)
	c := rndNode(t)
	d := rndNode(t)
	e := rndNode(t)
	f := rndNode(t)
	c.AddNodeLink("c0", d)
	c.AddNodeLink("c1", e)
	f.AddNodeLink("c0", c)
	a.AddNodeLink("c0", b)
	a.AddNodeLink("c1", c)
	a.AddNodeLink("c1", f)

	require.NoError(t, dserv.Add(ctx, a))
	require.NoError(t, dserv.Add(ctx, b))
	require.NoError(t, dserv.Add(ctx, c))
	require.NoError(t, dserv.Add(ctx, d))
	require.NoError(t, dserv.Add(ctx, e))
	require.NoError(t, dserv.Add(ctx, f))

	stats := NewDagStats()
	require.NoError(t, CalculateDagStats(
		ctx,
		&mockAPI{
			dag: &mockAPIDag{
				DAGService: dserv,
			},
		},
		a.Cid(),
		true,
		stats,
	))
	require.Equal(
		t,
		totalSize(a, b, c, d, e, f, c, d, e),
		stats.TotalSize.Load(),
	)
	require.Equal(
		t,
		int64(9),
		stats.TotalNumBlocks.Load(),
	)
	require.Equal(
		t,
		totalSize(a, b, c, d, e, f),
		stats.DeduplicatedSize.Load(),
	)
	require.Equal(
		t,
		int64(6),
		stats.DeduplicatedNumBlocks.Load(),
	)

	stats = NewDagStats()
	require.NoError(t, CalculateDagStats(
		ctx,
		&mockAPI{
			dag: &mockAPIDag{
				DAGService: dserv,
			},
		},
		c.Cid(),
		true,
		stats,
	))
	require.Equal(
		t,
		totalSize(c, d, e),
		stats.TotalSize.Load(),
	)
	require.Equal(
		t,
		int64(3),
		stats.TotalNumBlocks.Load(),
	)
	require.Equal(
		t,
		totalSize(c, d, e),
		stats.DeduplicatedSize.Load(),
	)
	require.Equal(
		t,
		int64(3),
		stats.DeduplicatedNumBlocks.Load(),
	)

	stats = NewDagStats()
	require.NoError(t, CalculateDagStats(
		ctx,
		&mockAPI{
			dag: &mockAPIDag{
				DAGService: dserv,
			},
		},
		a.Cid(),
		false,
		stats,
	))
	require.Equal(t, totalSize(a), stats.TotalSize.Load())
	require.Equal(t, int64(1), stats.TotalNumBlocks.Load())
	require.Equal(t, totalSize(a), stats.DeduplicatedSize.Load())
	require.Equal(t, int64(1), stats.DeduplicatedNumBlocks.Load())
}

func totalSize(nodes ...*merkledag.ProtoNode) int64 {
	sum := int64(0)
	for _, n := range nodes {
		sum += int64(len(n.RawData()))
	}
	return sum
}
