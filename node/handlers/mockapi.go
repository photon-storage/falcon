package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	coreiface "github.com/ipfs/boxo/coreiface"
	"github.com/ipfs/boxo/coreiface/options"
	"github.com/ipfs/boxo/coreiface/path"
	"github.com/ipfs/boxo/ipld/merkledag"
	ipld "github.com/ipfs/go-ipld-format"
)

var _ coreiface.CoreAPI = &mockAPI{}

type mockAPI struct {
	dag   coreiface.APIDagService
	block *mockAPIBlock
}

func (m *mockAPI) Unixfs() coreiface.UnixfsAPI {
	return nil
}

func (m *mockAPI) Block() coreiface.BlockAPI {
	return m.block
}

// Dag returns an implementation of Dag API
func (m *mockAPI) Dag() coreiface.APIDagService {
	return m.dag
}

// Name returns an implementation of Name API
func (m *mockAPI) Name() coreiface.NameAPI {
	return nil
}

// Key returns an implementation of Key API
func (m *mockAPI) Key() coreiface.KeyAPI {
	return nil
}

// Pin returns an implementation of Pin API
func (m *mockAPI) Pin() coreiface.PinAPI {
	return nil
}

// Object returns an implementation of Object API
func (m *mockAPI) Object() coreiface.ObjectAPI {
	return nil
}

// Dht returns an implementation of Dht API
func (m *mockAPI) Dht() coreiface.DhtAPI {
	return nil
}

// Swarm returns an implementation of Swarm API
func (m *mockAPI) Swarm() coreiface.SwarmAPI {
	return nil
}

// PubSub returns an implementation of PubSub API
func (m *mockAPI) PubSub() coreiface.PubSubAPI {
	return nil
}

// Routing returns an implementation of Routing API
func (m *mockAPI) Routing() coreiface.RoutingAPI {
	return nil
}

// ResolvePath resolves the path using Unixfs resolver
func (m *mockAPI) ResolvePath(context.Context, path.Path) (path.Resolved, error) {
	return nil, errors.New("not implemented")
}

// ResolveNode resolves the path (if not resolved already) using Unixfs
// resolver, gets and returns the resolved Node
func (m *mockAPI) ResolveNode(context.Context, path.Path) (ipld.Node, error) {
	return nil, errors.New("not implemented")
}

// WithOptions creates new instance of CoreAPI based on this instance with
// a set of options applied
func (m *mockAPI) WithOptions(...options.ApiOption) (coreiface.CoreAPI, error) {
	return nil, errors.New("not implemented")
}

type mockAPIDag struct {
	ipld.DAGService
}

func (m *mockAPIDag) Pinning() ipld.NodeAdder {
	return nil
}

type mockAPIBlock struct {
	blocks map[string][]byte
}

func newMockAPIBlock(nodes ...*merkledag.ProtoNode) *mockAPIBlock {
	m := &mockAPIBlock{
		blocks: map[string][]byte{},
	}
	for _, nd := range nodes {
		m.add(nd)
	}
	return m
}

func (m *mockAPIBlock) add(nd *merkledag.ProtoNode) {
	m.blocks[fmt.Sprintf("/ipfs/%v", nd.Cid().String())] = nd.RawData()
}

// Put imports raw block data, hashing it using specified settings.
func (m *mockAPIBlock) Put(
	context.Context,
	io.Reader,
	...options.BlockPutOption,
) (coreiface.BlockStat, error) {
	return nil, errors.New("not implemented")
}

// Get attempts to resolve the path and return a reader for data in the block
func (m *mockAPIBlock) Get(
	ctx context.Context,
	p path.Path,
) (io.Reader, error) {
	b := m.blocks[p.String()]
	if b == nil {
		return nil, ipld.ErrNotFound{}
	}
	return bytes.NewReader(b), nil
}

// Rm removes the block specified by the path from local blockstore.
// By default an error will be returned if the block can't be found locally.
//
// NOTE: If the specified block is pinned it won't be removed and no error
// will be returned
func (m *mockAPIBlock) Rm(
	context.Context,
	path.Path,
	...options.BlockRmOption,
) error {
	return errors.New("not implemented")
}

// Stat returns information on
func (m *mockAPIBlock) Stat(
	context.Context,
	path.Path,
) (coreiface.BlockStat, error) {
	return nil, errors.New("not implemented")
}
