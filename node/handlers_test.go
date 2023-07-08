package node

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	gohttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	bs "github.com/ipfs/go-blockservice"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	util "github.com/ipfs/go-ipfs-util"
	"github.com/ipfs/go-ipns"
	mdag "github.com/ipfs/go-merkledag"
	"github.com/ipfs/kubo/core"
	ir "github.com/ipfs/kubo/routing"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"go.uber.org/atomic"

	"github.com/photon-storage/go-common/testing/require"
	"github.com/photon-storage/go-gw3/common/crypto"
	"github.com/photon-storage/go-gw3/common/http"
	rcpinner "github.com/photon-storage/go-rc-pinner"
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
	pinner := &wrappedPinner{
		pinner:      rcpinner.New(ctx, dstore, dserv),
		pinnedCount: atomic.NewInt64(0),
	}

	h := newExtendedHandlers(
		&core.IpfsNode{
			Pinning: pinner,
		},
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
	h.pinnedCount()(w, r)
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
	h.pinnedCount()(w, r)
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
	h.pinnedCount()(w, r)
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
	h.pinnedCount()(w, r)
	require.Equal(t, gohttp.StatusBadRequest, w.Code)
	decodeResp(t, w, &res)
	require.Equal(t, -1, res.Count)
	require.Equal(t, "invalid CID", res.Message)
}

type mockRouting struct {
	ir.Composer
}

func newMockRouting() *mockRouting {
	return &mockRouting{
		ir.Composer{},
	}
}

func (m *mockRouting) PutValue(ctx context.Context, key string, val []byte, opts ...routing.Option) error {
	return nil
}

func TestNameBroadcast(t *testing.T) {
	sk := crypto.PregenEd25519(0)
	pk := sk.GetPublic()
	peerID, err := peer.IDFromPublicKey(pk)
	require.NoError(t, err)
	k := peerID.String()
	eol := time.Now().Add(5 * time.Minute)
	entry, err := ipns.Create(sk, []byte("Qme1knMqwt1hKZbc1BmQFmnm9f36nyQGwXxPGVpVJ9rMK5"), 1, eol, 0)
	require.NoError(t, err)
	require.NoError(t, ipns.EmbedPublicKey(pk, entry))
	data, err := proto.Marshal(entry)
	require.NoError(t, err)
	v := base64.URLEncoding.EncodeToString(data)

	h := newExtendedHandlers(
		&core.IpfsNode{
			Routing: newMockRouting(),
		},
		nil,
		nil,
	)

	r, err := gohttp.NewRequest(
		gohttp.MethodPost,
		"/api/v0/name/broadcast",
		nil,
	)
	require.NoError(t, err)
	query := r.URL.Query()
	query.Set(http.ParamIPFSKey, k)
	query.Set(http.ParamIPFSArg, v)
	r.URL.RawQuery = query.Encode()

	w := httptest.NewRecorder()
	h.nameBroadcast()(w, r)
	require.Equal(t, gohttp.StatusOK, w.Code)
	var res NameBroadcastResult
	decodeResp(t, w, &res)
	require.Equal(t, "ok", res.Message)
}

func decodeResp(t *testing.T, w *httptest.ResponseRecorder, v interface{}) {
	enc, err := ioutil.ReadAll(w.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(enc, v))
}
