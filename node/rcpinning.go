package node

import (
	"context"

	"github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/boxo/filestore"
	"github.com/ipfs/boxo/ipld/merkledag"
	pin "github.com/ipfs/boxo/pinning/pinner"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/kubo/repo"

	"github.com/photon-storage/go-common/log"
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
	pinner, err := rcpinner.New(
		context.TODO(),
		rootDstore,
		&syncDagService{
			DAGService: dserv,
			dstore:     rootDstore,
		},
	)
	if err != nil {
		log.Error("Error creating RC pinner", "error", err)
		panic(err)
	}

	return &wrappedPinner{
		pinner: pinner,
	}
}

func getRcPinner(v pin.Pinner) *wrappedPinner {
	if p, ok := v.(*wrappedPinner); ok {
		return p
	}
	return nil
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
	pinner *rcpinner.RcPinner
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
	return rcpinner.ErrUpdateUnsupported
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

func (p *wrappedPinner) DirectKeys(
	ctx context.Context,
) <-chan pin.StreamedCid {
	return p.pinner.DirectKeys(ctx)
}

func (p *wrappedPinner) RecursiveKeys(
	ctx context.Context,
) <-chan pin.StreamedCid {
	metrics.CounterInc("rc_pinner_recursive_keys_call_total")
	return p.pinner.RecursiveKeys(ctx)
}

func (p *wrappedPinner) InternalPins(
	ctx context.Context,
) <-chan pin.StreamedCid {
	return p.pinner.InternalPins(ctx)
}

func (p *wrappedPinner) PinnedCount(
	ctx context.Context,
	c cid.Cid,
) (uint16, error) {
	rc, err := p.pinner.PinnedCount(ctx, c, true)
	if err != nil {
		return 0, err
	}

	dc, err := p.pinner.PinnedCount(ctx, c, false)
	if err != nil {
		return 0, err
	}

	return rc + dc, nil
}

func (p *wrappedPinner) getTotalPinnedCount() int64 {
	return int64(p.pinner.TotalPinnedCount(true) + p.pinner.TotalPinnedCount(false))
}
