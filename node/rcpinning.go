package node

import (
	"context"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-filestore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	pin "github.com/ipfs/go-ipfs-pinner"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/kubo/repo"

	"github.com/photon-storage/go-common/metrics"
	rcpinner "github.com/photon-storage/go-rc-pinner"
)

// RcPinning creates new pinner which tells GC which blocks should be kept.
func RcPinning(
	bstore blockstore.Blockstore,
	dserv ipld.DAGService,
	repo repo.Repo,
) pin.Pinner {
	rootDstore := repo.Datastore()
	return &wrappedPinner{
		pinner: rcpinner.New(
			context.TODO(),
			rootDstore,
			&syncDagService{
				DAGService: dserv,
				dstore:     rootDstore,
			},
		),
	}
}

var (
	_ merkledag.SessionMaker = (*syncDagService)(nil)
	_ ipld.DAGService        = (*syncDagService)(nil)
)

// syncDagService is used by the Pinner to ensure data gets persisted to
// the underlying datastore
type syncDagService struct {
	ipld.DAGService
	dstore repo.Datastore
}

func (s *syncDagService) Sync(ctx context.Context) error {
	if err := s.dstore.Sync(ctx, blockstore.BlockPrefix); err != nil {
		return err
	}

	return s.dstore.Sync(ctx, filestore.FilestorePrefix)
}

func (s *syncDagService) Session(ctx context.Context) ipld.NodeGetter {
	return merkledag.NewSession(ctx, s.DAGService)
}

type wrappedPinner struct {
	pinner pin.Pinner
}

func (p *wrappedPinner) IsPinned(
	ctx context.Context,
	c cid.Cid,
) (string, bool, error) {
	return p.pinner.IsPinned(ctx, c)
}

func (p *wrappedPinner) IsPinnedWithType(
	ctx context.Context,
	c cid.Cid,
	mode pin.Mode,
) (string, bool, error) {
	return p.pinner.IsPinnedWithType(ctx, c, mode)
}

func (p *wrappedPinner) Pin(
	ctx context.Context,
	node ipld.Node,
	recursive bool,
) error {
	metrics.CounterInc("rc_pinner_pin_call_total")
	if err := p.pinner.Pin(ctx, node, recursive); err != nil {
		metrics.CounterInc("rc_pinner_pin_err_total")
		return err
	}
	return nil
}
func (p *wrappedPinner) Unpin(
	ctx context.Context,
	cid cid.Cid,
	recursive bool,
) error {
	metrics.CounterInc("rc_pinner_unpin_call_total")
	if err := p.pinner.Unpin(ctx, cid, recursive); err != nil {
		metrics.CounterInc("rc_pinner_unpin_err_total")
		return err
	}
	return nil
}

func (p *wrappedPinner) Update(
	ctx context.Context,
	from cid.Cid,
	to cid.Cid,
	unpin bool,
) error {
	return p.pinner.Update(ctx, from, to, unpin)
}

func (p *wrappedPinner) CheckIfPinned(
	ctx context.Context,
	cids ...cid.Cid,
) ([]pin.Pinned, error) {
	return p.pinner.CheckIfPinned(ctx, cids...)
}

func (p *wrappedPinner) PinWithMode(
	ctx context.Context,
	cid cid.Cid,
	mode pin.Mode) error {
	return p.pinner.PinWithMode(ctx, cid, mode)
}

func (p *wrappedPinner) Flush(ctx context.Context) error {
	return p.pinner.Flush(ctx)
}

func (p *wrappedPinner) DirectKeys(ctx context.Context) ([]cid.Cid, error) {
	cids, err := p.pinner.DirectKeys(ctx)
	if err != nil {
		return nil, err
	}

	metrics.GaugeSet("rc_pinner_direct_pinned_total", float64(len(cids)))
	return cids, nil
}

func (p *wrappedPinner) RecursiveKeys(ctx context.Context) ([]cid.Cid, error) {
	cids, err := p.pinner.RecursiveKeys(ctx)
	if err != nil {
		return nil, err
	}

	metrics.GaugeSet("rc_pinner_recursive_pinned_total", float64(len(cids)))
	return cids, nil
}

func (p *wrappedPinner) InternalPins(ctx context.Context) ([]cid.Cid, error) {
	cids, err := p.pinner.InternalPins(ctx)
	if err != nil {
		return nil, err
	}

	metrics.GaugeSet("rc_pinner_internal_pinned_total", float64(len(cids)))
	return cids, nil
}
