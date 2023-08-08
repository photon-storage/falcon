package handlers

import (
	"context"
	"fmt"
	"io"
	gohttp "net/http"
	"net/http/httptest"
	"testing"

	bs "github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/boxo/exchange/offline"
	mdag "github.com/ipfs/boxo/ipld/merkledag"
	util "github.com/ipfs/boxo/util"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/ipfs/kubo/core"

	"github.com/photon-storage/go-common/testing/require"
	"github.com/photon-storage/go-gw3/common/http"
	rcpinner "github.com/photon-storage/go-rc-pinner"

	"github.com/photon-storage/falcon/node/com"
)

var rand = util.NewTimeSeededRand()

func rndNode(t require.TestingTB) *mdag.ProtoNode {
	nd := new(mdag.ProtoNode)
	nd.SetData(make([]byte, 32))
	_, err := io.ReadFull(rand, nd.Data())
	require.NoError(t, err)
	return nd
}

func TestPinnedCount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dstore := dssync.MutexWrap(ds.NewMapDatastore())
	bstore := blockstore.NewBlockstore(dstore)
	bserv := bs.New(bstore, offline.Exchange(bstore))
	dserv := mdag.NewDAGService(bserv)
	rcp, err := rcpinner.New(ctx, dstore, dserv)
	require.NoError(t, err)
	pinner := &com.WrappedPinner{
		Pinner: rcp,
	}

	h := New(
		&core.IpfsNode{
			Pinning: pinner,
		},
		nil,
		nil,
		nil,
	)

	nd0 := rndNode(t)
	nd1 := rndNode(t)
	require.NoError(t, dserv.Add(ctx, nd0))
	require.NoError(t, pinner.Pin(ctx, nd0, true))

	// nd0 pinned = 1
	r, err := gohttp.NewRequest(
		gohttp.MethodGet,
		fmt.Sprintf(
			"/api/v0/pin/count?%s=%s",
			http.ParamIPFSArg,
			nd0.Cid().String(),
		),
		nil,
	)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	h.PinnedCount()(w, r)
	require.Equal(t, gohttp.StatusOK, w.Code)
	var res PinnedCountResult
	decodeResp(t, w, &res)
	require.Equal(t, 1, res.Count)

	// nd1 not pinned
	r, err = gohttp.NewRequest(
		gohttp.MethodGet,
		fmt.Sprintf(
			"/api/v0/pin/count?%s=%s",
			http.ParamIPFSArg,
			nd1.Cid().String(),
		),
		nil,
	)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	h.PinnedCount()(w, r)
	require.Equal(t, gohttp.StatusOK, w.Code)
	decodeResp(t, w, &res)
	require.Equal(t, 0, res.Count)

	// nd0 count = 3
	require.NoError(t, pinner.Pin(ctx, nd0, true))
	require.NoError(t, pinner.Pin(ctx, nd0, true))
	r, err = gohttp.NewRequest(
		gohttp.MethodGet,
		fmt.Sprintf(
			"/api/v0/pin/count?%s=%s",
			http.ParamIPFSArg,
			nd0.Cid().String(),
		),
		nil,
	)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	h.PinnedCount()(w, r)
	require.Equal(t, gohttp.StatusOK, w.Code)
	decodeResp(t, w, &res)
	require.Equal(t, 3, res.Count)

	// invalid cid
	r, err = gohttp.NewRequest(
		gohttp.MethodGet,
		fmt.Sprintf(
			"/api/v0/pin/count?%s=invalid_cid",
			http.ParamIPFSArg,
		),
		nil,
	)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	h.PinnedCount()(w, r)
	require.Equal(t, gohttp.StatusBadRequest, w.Code)
	decodeResp(t, w, &res)
	require.Equal(t, -1, res.Count)
	require.Equal(t, "invalid CID", res.Message)
}
