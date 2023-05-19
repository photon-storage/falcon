package node

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-libipfs/blocks"
	ipfsgw "github.com/ipfs/go-libipfs/gateway"
	"github.com/ipfs/go-namesys"
	iface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/options"
	nsopts "github.com/ipfs/interface-go-ipfs-core/options/namesys"
	"github.com/ipfs/interface-go-ipfs-core/path"
	"github.com/ipfs/kubo/core"
	"github.com/ipfs/kubo/core/coreapi"
)

var _ ipfsgw.API = (*gatewayAPI)(nil)

// ***********************************************************************//
// gatewayAPI is copied from github.com/ipfs/kubo/core/corehttp/gateway.go
// with code format. No logic change is made.
// ***********************************************************************//
type gatewayAPI struct {
	ns         namesys.NameSystem
	api        iface.CoreAPI
	offlineAPI iface.CoreAPI
}

func newGatewayAPI(n *core.IpfsNode) (*gatewayAPI, error) {
	cfg, err := n.Repo.Config()
	if err != nil {
		return nil, err
	}

	api, err := coreapi.NewCoreAPI(
		n,
		options.Api.FetchBlocks(!cfg.Gateway.NoFetch),
	)
	if err != nil {
		return nil, err
	}

	offlineAPI, err := api.WithOptions(options.Api.Offline(true))
	if err != nil {
		return nil, err
	}

	return &gatewayAPI{
		ns:         n.Namesys,
		api:        api,
		offlineAPI: offlineAPI,
	}, nil
}

func (gw *gatewayAPI) GetUnixFsNode(
	ctx context.Context,
	pth path.Resolved,
) (files.Node, error) {
	return gw.api.Unixfs().Get(ctx, pth)
}

func (gw *gatewayAPI) LsUnixFsDir(
	ctx context.Context,
	pth path.Resolved,
) (<-chan iface.DirEntry, error) {
	// Optimization: use Unixfs.Ls without resolving children, but using the
	// cumulative DAG size as the file size. This allows for a fast listing
	// while keeping a good enough Size field.
	return gw.api.Unixfs().Ls(ctx, pth,
		options.Unixfs.ResolveChildren(false),
		options.Unixfs.UseCumulativeSize(true),
	)
}

func (gw *gatewayAPI) GetBlock(
	ctx context.Context,
	cid cid.Cid,
) (blocks.Block, error) {
	r, err := gw.api.Block().Get(ctx, path.IpfsPath(cid))
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return blocks.NewBlockWithCid(data, cid)
}

func (gw *gatewayAPI) GetIPNSRecord(
	ctx context.Context,
	c cid.Cid,
) ([]byte, error) {
	return gw.api.Routing().Get(ctx, "/ipns/"+c.String())
}

func (gw *gatewayAPI) GetDNSLinkRecord(
	ctx context.Context,
	hostname string,
) (path.Path, error) {
	p, err := gw.ns.Resolve(ctx, "/ipns/"+hostname, nsopts.Depth(1))
	if err == namesys.ErrResolveRecursion {
		err = nil
	}
	return path.New(p.String()), err
}

func (gw *gatewayAPI) IsCached(ctx context.Context, pth path.Path) bool {
	_, err := gw.offlineAPI.Block().Stat(ctx, pth)
	return err == nil
}

func (gw *gatewayAPI) ResolvePath(
	ctx context.Context,
	pth path.Path,
) (path.Resolved, error) {
	return gw.api.ResolvePath(ctx, pth)
}
