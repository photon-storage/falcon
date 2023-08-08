package com

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

	return &WrappedPinner{
		Pinner: pinner,
	}
}

func GetRcPinner(v pin.Pinner) *WrappedPinner {
	if p, ok := v.(*WrappedPinner); ok {
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

type WrappedPinner struct {
	Pinner *rcpinner.RcPinner
}

func (p *WrappedPinner) IsPinned(
	ctx context.Context,
	c cid.Cid,
) (string, bool, error) {
	return p.Pinner.IsPinned(ctx, c)
}

func (p *WrappedPinner) IsPinnedWithType(
	ctx context.Context,
	c cid.Cid,
	mode pin.Mode,
) (string, bool, error) {
	return p.Pinner.IsPinnedWithType(ctx, c, mode)
}

func (p *WrappedPinner) Pin(
	ctx context.Context,
	node ipld.Node,
	recursive bool,
) error {
	metrics.CounterInc("rc_pinner_pin_call_total")
	if err := p.Pinner.Pin(ctx, node, recursive); err != nil {
		metrics.CounterInc("rc_pinner_pin_err_total")
		return err
	}
	return nil
}

func (p *WrappedPinner) Unpin(
	ctx context.Context,
	cid cid.Cid,
	recursive bool,
) error {
	metrics.CounterInc("rc_pinner_unpin_call_total")
	if err := p.Pinner.Unpin(ctx, cid, recursive); err != nil {
		metrics.CounterInc("rc_pinner_unpin_err_total")
		return err
	}
	return nil
}

func (p *WrappedPinner) Update(
	ctx context.Context,
	from cid.Cid,
	to cid.Cid,
	unpin bool,
) error {
	return rcpinner.ErrUpdateUnsupported
}

func (p *WrappedPinner) CheckIfPinned(
	ctx context.Context,
	cids ...cid.Cid,
) ([]pin.Pinned, error) {
	return p.Pinner.CheckIfPinned(ctx, cids...)
}

func (p *WrappedPinner) PinWithMode(
	ctx context.Context,
	cid cid.Cid,
	mode pin.Mode) error {
	return p.Pinner.PinWithMode(ctx, cid, mode)
}

func (p *WrappedPinner) Flush(ctx context.Context) error {
	return p.Pinner.Flush(ctx)
}

func (p *WrappedPinner) DirectKeys(
	ctx context.Context,
) <-chan pin.StreamedCid {
	metrics.CounterInc("rc_pinner_direct_keys_call_total")
	return p.Pinner.DirectKeys(ctx)
}

func (p *WrappedPinner) RecursiveKeys(
	ctx context.Context,
) <-chan pin.StreamedCid {
	metrics.CounterInc("rc_pinner_recursive_keys_call_total")
	return p.Pinner.RecursiveKeys(ctx)
}

func (p *WrappedPinner) InternalPins(
	ctx context.Context,
) <-chan pin.StreamedCid {
	return p.Pinner.InternalPins(ctx)
}

func (p *WrappedPinner) PinnedCount(
	ctx context.Context,
	c cid.Cid,
) (uint16, error) {
	rc, err := p.Pinner.PinnedCount(ctx, c, true)
	if err != nil {
		return 0, err
	}

	dc, err := p.Pinner.PinnedCount(ctx, c, false)
	if err != nil {
		return 0, err
	}

	return rc + dc, nil
}

func (p *WrappedPinner) TotalPinnedCount() int64 {
	return int64(p.Pinner.TotalPinnedCount(true) + p.Pinner.TotalPinnedCount(false))
}

func RegisterPinnerMetrics() {
	metrics.NewCounter("rc_pinner_pin_call_total")
	metrics.NewCounter("rc_pinner_pin_err_total")
	metrics.NewCounter("rc_pinner_unpin_call_total")
	metrics.NewCounter("rc_pinner_unpin_err_total")
	metrics.NewCounter("rc_pinner_direct_keys_call_total")
	metrics.NewCounter("rc_pinner_recursive_keys_call_total")
}
