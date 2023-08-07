package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	gohttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/boxo/ipns"
	"github.com/ipfs/kubo/core"
	ir "github.com/ipfs/kubo/routing"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"

	"github.com/photon-storage/go-common/testing/require"
	"github.com/photon-storage/go-gw3/common/crypto"
	"github.com/photon-storage/go-gw3/common/http"
)

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
	entry, err := ipns.Create(
		sk,
		[]byte("Qme1knMqwt1hKZbc1BmQFmnm9f36nyQGwXxPGVpVJ9rMK5"),
		1,
		eol,
		0,
	)
	require.NoError(t, err)
	require.NoError(t, ipns.EmbedPublicKey(pk, entry))
	data, err := proto.Marshal(entry)
	require.NoError(t, err)
	v := base64.URLEncoding.EncodeToString(data)

	h := New(
		&core.IpfsNode{
			Routing: newMockRouting(),
		},
		nil,
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
	h.NameBroadcast()(w, r)
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
