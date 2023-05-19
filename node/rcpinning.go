package node

import (
	"context"

	"github.com/ipfs/go-filestore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	pin "github.com/ipfs/go-ipfs-pinner"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/kubo/repo"

	rcpinner "github.com/photon-storage/go-rc-pinner"
)

// RcPinning creates new pinner which tells GC which blocks should be kept.
func RcPinning(
	bstore blockstore.Blockstore,
	dserv format.DAGService,
	repo repo.Repo,
) pin.Pinner {
	rootDstore := repo.Datastore()
	return rcpinner.New(
		context.TODO(),
		rootDstore,
		&syncDagService{
			DAGService: dserv,
			dstore:     rootDstore,
		},
	)
}

var (
	_ merkledag.SessionMaker = (*syncDagService)(nil)
	_ format.DAGService      = (*syncDagService)(nil)
)

// syncDagService is used by the Pinner to ensure data gets persisted to
// the underlying datastore
type syncDagService struct {
	format.DAGService
	dstore repo.Datastore
}

func (s *syncDagService) Sync(ctx context.Context) error {
	if err := s.dstore.Sync(ctx, blockstore.BlockPrefix); err != nil {
		return err
	}

	return s.dstore.Sync(ctx, filestore.FilestorePrefix)
}

func (s *syncDagService) Session(ctx context.Context) format.NodeGetter {
	return merkledag.NewSession(ctx, s.DAGService)
}
