package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	gohttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bs "github.com/ipfs/boxo/blockservice"
	"github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/boxo/exchange/offline"
	"github.com/ipfs/boxo/ipld/merkledag"
	pinneriface "github.com/ipfs/boxo/pinning/pinner"
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

func rndNode(t require.TestingTB) *merkledag.ProtoNode {
	nd := new(merkledag.ProtoNode)
	nd.SetData(make([]byte, 32))
	_, err := io.ReadFull(rand, nd.Data())
	require.NoError(t, err)
	return nd
}

func TestPinList(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dstore := dssync.MutexWrap(ds.NewMapDatastore())
	bstore := blockstore.NewBlockstore(dstore)
	bserv := bs.New(bstore, offline.Exchange(bstore))
	dserv := merkledag.NewDAGService(bserv)
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

	// A{B,C{D,E},F}
	a := rndNode(t)
	b := rndNode(t)
	c := rndNode(t)
	d := rndNode(t)
	e := rndNode(t)
	f := rndNode(t)
	require.NoError(t, c.AddNodeLink("c0", d))
	require.NoError(t, c.AddNodeLink("c1", e))
	require.NoError(t, a.AddNodeLink("c0", b))
	require.NoError(t, a.AddNodeLink("c0", c))
	require.NoError(t, a.AddNodeLink("c0", f))
	require.NoError(t, dserv.Add(ctx, a))
	require.NoError(t, dserv.Add(ctx, b))
	require.NoError(t, dserv.Add(ctx, c))
	require.NoError(t, dserv.Add(ctx, d))
	require.NoError(t, dserv.Add(ctx, e))
	require.NoError(t, dserv.Add(ctx, f))

	require.NoError(t, pinner.Pin(ctx, a, true))
	require.NoError(t, pinner.Pin(ctx, a, true))
	require.NoError(t, pinner.Pin(ctx, d, true))
	require.NoError(t, pinner.Pin(ctx, e, true))

	require.NoError(t, pinner.Pin(ctx, a, false))
	require.NoError(t, pinner.Pin(ctx, c, false))
	require.NoError(t, pinner.Pin(ctx, c, false))

	r, err := gohttp.NewRequest(
		gohttp.MethodGet,
		"/api/v0/pin/ls?recursive=1",
		nil,
	)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	h.PinList()(w, r)
	require.Equal(t, gohttp.StatusOK, w.Code)
	var res PinListResult
	decodeResp(t, w, &res)
	require.True(t, res.Success)
	require.False(t, res.InProgress)
	m := batchMap(res.Batch)
	require.Equal(t, 3, len(m))
	require.Equal(t, 2, m[a.Cid().String()])
	require.Equal(t, 1, m[d.Cid().String()])
	require.Equal(t, 1, m[e.Cid().String()])

	r, err = gohttp.NewRequest(
		gohttp.MethodGet,
		"/api/v0/pin/ls?recursive=0",
		nil,
	)
	require.NoError(t, err)
	w = httptest.NewRecorder()
	h.PinList()(w, r)
	require.Equal(t, gohttp.StatusOK, w.Code)
	decodeResp(t, w, &res)
	require.True(t, res.Success)
	require.False(t, res.InProgress)
	m = batchMap(res.Batch)
	require.Equal(t, 2, len(m))
	require.Equal(t, 1, m[a.Cid().String()])
	require.Equal(t, 2, m[c.Cid().String()])
}

func TestPinListMultiBatches(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dstore := dssync.MutexWrap(ds.NewMapDatastore())
	bstore := blockstore.NewBlockstore(dstore)
	bserv := bs.New(bstore, offline.Exchange(bstore))
	dserv := merkledag.NewDAGService(bserv)
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

	var nodes []*merkledag.ProtoNode
	for i := 0; i < 2*cidBatchSize+10; i++ {
		nd := rndNode(t)
		require.NoError(t, dserv.Add(ctx, nd))
		require.NoError(t, pinner.Pin(ctx, nd, true))
		nodes = append(nodes, nd)
	}

	r, err := gohttp.NewRequest(
		gohttp.MethodGet,
		"/api/v0/pin/ls?recursive=1",
		nil,
	)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	h.PinList()(w, r)
	require.Equal(t, gohttp.StatusOK, w.Code)
	dec := json.NewDecoder(w.Body)
	var all []*CidCount
	res := PinListResult{}
	require.NoError(t, dec.Decode(&res))
	require.False(t, res.Success)
	require.True(t, res.InProgress)
	require.Equal(t, 100, len(res.Batch))
	all = append(all, res.Batch...)
	res = PinListResult{}
	require.NoError(t, dec.Decode(&res))
	require.False(t, res.Success)
	require.True(t, res.InProgress)
	require.Equal(t, 100, len(res.Batch))
	all = append(all, res.Batch...)
	res = PinListResult{}
	require.NoError(t, dec.Decode(&res))
	require.True(t, res.Success)
	require.False(t, res.InProgress)
	require.Equal(t, 10, len(res.Batch))
	all = append(all, res.Batch...)

	m := batchMap(all)
	require.Equal(t, 2*cidBatchSize+10, len(m))
	for _, nd := range nodes {
		require.Equal(t, 1, m[nd.Cid().String()])
	}
}

func TestPinnedCount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dstore := dssync.MutexWrap(ds.NewMapDatastore())
	bstore := blockstore.NewBlockstore(dstore)
	bserv := bs.New(bstore, offline.Exchange(bstore))
	dserv := merkledag.NewDAGService(bserv)
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
			"/api/v0/pin/count?%s=%s&recursive=1",
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
	require.True(t, res.Success)
	require.Equal(t, 1, res.Count)
	r, err = gohttp.NewRequest(
		gohttp.MethodGet,
		fmt.Sprintf(
			"/api/v0/pin/count?%s=%s&recursive=0",
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
	require.True(t, res.Success)
	require.Equal(t, 0, res.Count)

	// nd1 not pinned
	r, err = gohttp.NewRequest(
		gohttp.MethodGet,
		fmt.Sprintf(
			"/api/v0/pin/count?%s=%s&recursive=1",
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
	require.True(t, res.Success)
	require.Equal(t, 0, res.Count)
	r, err = gohttp.NewRequest(
		gohttp.MethodGet,
		fmt.Sprintf(
			"/api/v0/pin/count?%s=%s&recursive=0",
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
	require.True(t, res.Success)
	require.Equal(t, 0, res.Count)

	// nd0 count = 3
	require.NoError(t, pinner.Pin(ctx, nd0, true))
	require.NoError(t, pinner.Pin(ctx, nd0, true))
	require.NoError(t, pinner.Pin(ctx, nd0, false))
	r, err = gohttp.NewRequest(
		gohttp.MethodGet,
		fmt.Sprintf(
			"/api/v0/pin/count?%s=%s&recursive=1",
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
	require.True(t, res.Success)
	require.Equal(t, 3, res.Count)
	r, err = gohttp.NewRequest(
		gohttp.MethodGet,
		fmt.Sprintf(
			"/api/v0/pin/count?%s=%s&recursive=0",
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
	require.True(t, res.Success)
	require.Equal(t, 1, res.Count)

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
	require.False(t, res.Success)
	require.Equal(t, 0, res.Count)
	require.True(t, strings.Contains(res.Message, "invalid CID"))
}

func TestPinChildrenUpdate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dstore := dssync.MutexWrap(ds.NewMapDatastore())
	bstore := blockstore.NewBlockstore(dstore)
	bserv := bs.New(bstore, offline.Exchange(bstore))
	dserv := merkledag.NewDAGService(bserv)
	rcp, err := rcpinner.New(ctx, dstore, dserv)
	require.NoError(t, err)
	pinner := &com.WrappedPinner{
		Pinner: rcp,
	}

	a := rndNode(t)
	b := rndNode(t)
	c := rndNode(t)
	d := rndNode(t)
	require.NoError(t, a.AddNodeLink("c0", b))
	require.NoError(t, a.AddNodeLink("c1", c))
	require.NoError(t, dserv.Add(ctx, a))
	require.NoError(t, dserv.Add(ctx, b))
	require.NoError(t, dserv.Add(ctx, c))
	require.NoError(t, dserv.Add(ctx, d))
	require.NoError(t, pinner.Pin(ctx, a, true))
	require.NoError(t, pinner.Pin(ctx, b, true))
	require.NoError(t, pinner.Pin(ctx, c, false))

	h := New(
		&core.IpfsNode{
			Pinning: pinner,
		},
		nil,
		&mockAPI{
			dag: &mockAPIDag{
				DAGService: dserv,
			},
			block: newMockAPIBlock(a),
		},
		nil,
	)

	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "root not pinned",
			run: func(t *testing.T) {
				data, err := json.Marshal(&PinChildrenUpdateRequest{
					Root: d.Cid().String(),
					Incs: []*PinChildrenUpdate{
						&PinChildrenUpdate{
							Cid:       b.Cid().String(),
							Recursive: true,
						},
					},
					Decs: []*PinChildrenUpdate{},
				})
				require.NoError(t, err)

				r, err := gohttp.NewRequest(
					gohttp.MethodGet,
					"/api/v0/pin/children_update",
					bytes.NewReader(data),
				)
				require.NoError(t, err)

				w := httptest.NewRecorder()
				h.PinChildrenUpdate()(w, r)
				require.Equal(t, gohttp.StatusBadRequest, w.Code)
				var res PinChildrenUpdateResult
				decodeResp(t, w, &res)
				require.False(t, res.Success)
				require.True(t, strings.Contains(res.Message, pinneriface.ErrNotPinned.Error()))
			},
		},
		{
			name: "not a child",
			run: func(t *testing.T) {
				data, err := json.Marshal(&PinChildrenUpdateRequest{
					Root: a.Cid().String(),
					Incs: []*PinChildrenUpdate{
						&PinChildrenUpdate{
							Cid:       d.Cid().String(),
							Recursive: true,
						},
					},
					Decs: []*PinChildrenUpdate{},
				})
				require.NoError(t, err)

				r, err := gohttp.NewRequest(
					gohttp.MethodGet,
					"/api/v0/pin/children_update",
					bytes.NewReader(data),
				)
				require.NoError(t, err)

				w := httptest.NewRecorder()
				h.PinChildrenUpdate()(w, r)
				require.Equal(t, gohttp.StatusBadRequest, w.Code)
				var res PinChildrenUpdateResult
				decodeResp(t, w, &res)
				require.False(t, res.Success)
				require.True(t, strings.Contains(res.Message, ErrCIDNotChild.Error()))
			},
		},
		{
			name: "deupped cid",
			run: func(t *testing.T) {
				data, err := json.Marshal(&PinChildrenUpdateRequest{
					Root: a.Cid().String(),
					Incs: []*PinChildrenUpdate{
						&PinChildrenUpdate{
							Cid:       b.Cid().String(),
							Recursive: true,
						},
					},
					Decs: []*PinChildrenUpdate{
						&PinChildrenUpdate{
							Cid:       b.Cid().String(),
							Recursive: true,
						},
					},
				})
				require.NoError(t, err)

				r, err := gohttp.NewRequest(
					gohttp.MethodGet,
					"/api/v0/pin/children_update",
					bytes.NewReader(data),
				)
				require.NoError(t, err)

				w := httptest.NewRecorder()
				h.PinChildrenUpdate()(w, r)
				require.Equal(t, gohttp.StatusBadRequest, w.Code)
				var res PinChildrenUpdateResult
				decodeResp(t, w, &res)
				require.False(t, res.Success)
				require.True(t, strings.Contains(res.Message, ErrCIDDuplicated.Error()))
			},
		},
		{
			name: "pin not found",
			run: func(t *testing.T) {
				data, err := json.Marshal(&PinChildrenUpdateRequest{
					Root: a.Cid().String(),
					Incs: []*PinChildrenUpdate{
						&PinChildrenUpdate{
							Cid:       b.Cid().String(),
							Recursive: true,
						},
					},
					Decs: []*PinChildrenUpdate{
						&PinChildrenUpdate{
							Cid:       c.Cid().String(),
							Recursive: true,
						},
					},
				})
				require.NoError(t, err)

				r, err := gohttp.NewRequest(
					gohttp.MethodGet,
					"/api/v0/pin/children_update",
					bytes.NewReader(data),
				)
				require.NoError(t, err)

				w := httptest.NewRecorder()
				h.PinChildrenUpdate()(w, r)
				require.Equal(t, gohttp.StatusBadRequest, w.Code)
				var res PinChildrenUpdateResult
				decodeResp(t, w, &res)
				require.False(t, res.Success)
				require.True(t, strings.Contains(res.Message, pinneriface.ErrNotPinned.Error()))

				pinner := com.GetRcPinner(h.nd.Pinning)
				cnt, err := pinner.GetCount(ctx, b.Cid(), true)
				require.NoError(t, err)
				require.Equal(t, uint16(1), cnt)
				cnt, err = pinner.GetCount(ctx, c.Cid(), false)
				require.NoError(t, err)
				require.Equal(t, uint16(1), cnt)
			},
		},
		{
			name: "success",
			run: func(t *testing.T) {
				data, err := json.Marshal(&PinChildrenUpdateRequest{
					Root: a.Cid().String(),
					Incs: []*PinChildrenUpdate{
						&PinChildrenUpdate{
							Cid:       b.Cid().String(),
							Recursive: true,
						},
						&PinChildrenUpdate{
							Cid:       c.Cid().String(),
							Recursive: true,
						},
					},
					Decs: []*PinChildrenUpdate{
						&PinChildrenUpdate{
							Cid:       c.Cid().String(),
							Recursive: false,
						},
					},
				})
				require.NoError(t, err)

				r, err := gohttp.NewRequest(
					gohttp.MethodGet,
					"/api/v0/pin/children_update",
					bytes.NewReader(data),
				)
				require.NoError(t, err)

				w := httptest.NewRecorder()
				h.PinChildrenUpdate()(w, r)
				require.Equal(t, gohttp.StatusOK, w.Code)
				var res PinChildrenUpdateResult
				decodeResp(t, w, &res)
				require.True(t, res.Success)

				pinner := com.GetRcPinner(h.nd.Pinning)
				cnt, err := pinner.GetCount(ctx, b.Cid(), true)
				require.NoError(t, err)
				require.Equal(t, uint16(2), cnt)
				cnt, err = pinner.GetCount(ctx, c.Cid(), true)
				require.NoError(t, err)
				require.Equal(t, uint16(1), cnt)
				cnt, err = pinner.GetCount(ctx, c.Cid(), false)
				require.NoError(t, err)
				require.Equal(t, uint16(0), cnt)
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, c.run)
	}
}

func batchMap(b []*CidCount) map[string]int {
	m := map[string]int{}
	for _, v := range b {
		m[v.Cid] = v.Count
	}
	return m
}
