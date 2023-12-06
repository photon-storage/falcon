package com

import (
	"time"

	"github.com/ipfs/kubo/core/node/helpers"
	libp2p "github.com/ipfs/kubo/core/node/libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	namesys "github.com/libp2p/go-libp2p-pubsub-router"
	record "github.com/libp2p/go-libp2p-record"
	routinghelpers "github.com/libp2p/go-libp2p-routing-helpers"
	"github.com/libp2p/go-libp2p/core/host"
	"go.uber.org/fx"
)

var (
	neverDuration = 100 * 365 * 24 * time.Hour
)

type p2pPSRoutingIn struct {
	fx.In

	Validator record.Validator
	Host      host.Host
	PubSub    *pubsub.PubSub
}

type p2pRouterOut struct {
	fx.Out
	Router libp2p.Router `group:"routers"`
}

func PubsubRouter(
	mctx helpers.MetricsCtx,
	lc fx.Lifecycle,
	in p2pPSRoutingIn,
) (p2pRouterOut, *namesys.PubsubValueStore, error) {
	psRouter, err := namesys.NewPubsubValueStore(
		helpers.LifecycleCtx(mctx, lc),
		in.Host,
		in.PubSub,
		in.Validator,
		namesys.WithRebroadcastInitialDelay(neverDuration),
	)

	if err != nil {
		return p2pRouterOut{}, nil, err
	}

	return p2pRouterOut{
		Router: libp2p.Router{
			Routing: &routinghelpers.Compose{
				ValueStore: &routinghelpers.LimitedValueStore{
					ValueStore: psRouter,
					Namespaces: []string{"ipns"},
				},
			},
			Priority: 100,
		},
	}, psRouter, nil
}
