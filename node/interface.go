package node

import (
	"errors"

	cmds "github.com/ipfs/go-ipfs-cmds"
	oldcmds "github.com/ipfs/kubo/commands"
	"github.com/ipfs/kubo/core"
	corenode "github.com/ipfs/kubo/core/node"
	"github.com/ipfs/kubo/repo"
	"go.uber.org/fx"

	"github.com/photon-storage/go-common/log"
)

// This file defines interfaces for hooking Falcon logic into the
// official Kubo implementation. Thanks to the fx injection system,
// we could custom Kubo configuration to some extent without heavily
// modifying the Kubo implementation, which could be a headache for
// future upgrades.
//
// All kubo CLI options are preserved. A typical run uses `falcon daemon`
// to start the process. Run `falcon init` to initialize IPFS profile
// if necessary.
//
// Upgrade procedure (as of Kubo v1.9.0):
//
// 1. Clean up files in cmd/falcon/, keeping config/ and e2e/ directories.
//    cp -rf github.com/ipfs/kubo/cmd/ipfs/* cmd/falcon/
//
// 2. Add falcon.ConfigOption() to daemonCmd.Options to receive falcon config
//    path from CLI flag.
//
// 3. Locate node creation code core.NewNode(...) in daemonFunc() from
//    cmd/falcon/daemon.go and add the following code before it:
//
//		//////////////////// Falcon ////////////////////
//		if err := falcon.InitFalconBeforeNodeConstruction(req, repo); err != nil {
//			fmt.Printf("Error initializing falcon before IPFS node construction: %v", err)
//			return err
//		}
//		//////////////////// Falcon ////////////////////
//
// 4. Locate node creation code serveHTTPApi(...) in daemonFunc() from
//    cmd/falcon/daemon.go and add the following code before it:
//
//		//////////////////// Falcon ////////////////////
//		falconErrc, err := falcon.InitFalconAfterNodeConstruction(req, cctx, node)
//		if err != nil {
//			fmt.Printf("Error initializing falcon after IPFS node construction: %v", err)
//			return err
//		}
//		//////////////////// Falcon ////////////////////
//
// 5. Update error channel merge loop to include falconErrc in daemonFunc()
//    from cmd/falcon/daemon.go:
//
//		for err := range merge(apiErrc, gwErrc, gcErrc, falconErrc) {
//		  ...
//		}
//
// 6. Disable default RPC API initialization in cmd/falcon/daemon.go by adding
//    the following code to the beginning of serveHTTPApi() from
//    from cmd/falcon/daemon.go:
//
//		//////////////////// Falcon ////////////////////
//      if true {
//			return nil, nil
// 		}
//		//////////////////// Falcon ////////////////////
//
// 7. Disable default gateway initialization in cmd/falcon/daemon.go by adding
//    the following code to the beginning of serveHTTPGateway() from
//    from cmd/falcon/daemon.go:
//
//		//////////////////// Falcon ////////////////////
//      if true {
//			return nil, nil
// 		}
//		//////////////////// Falcon ////////////////////
//
// 8. Disable debug handler in cmd/falcon/debug.go
//
// 9. Run `go mod tidy` to update go.mod with whats required by the new kubo
//    version. There might be a conflict with the otel package when building
//    falcon. Use `go get` to force the version used in kubo/go.mod.
//    For example:
//       go get go.opentelemetry.io/otel@v1.7.0
//
// Example command to run falcon daemon:
//    go run ./cmd/falcon/. daemon --falcon-config=./cmd/falcon/config/config_dev.yaml
const (
	falconConfigFile = "falcon-config"
)

var (
	ErrConfigMissing   = errors.New("falcon config file path not specified")
	ErrRcPinnerMissing = errors.New("falcon node is created without rc pinner")
)

func ConfigOption() cmds.Option {
	return cmds.StringOption(falconConfigFile, "Required path to a falcon config file.")
}

// InitFalconBeforeNodeConstruction setups falcon preparation work before
// core.IpfsNode is constructed.
func InitFalconBeforeNodeConstruction(
	req *cmds.Request,
	rpo repo.Repo,
) error {
	cfgPath, _ := req.Options[falconConfigFile].(string)
	if cfgPath == "" {
		return ErrConfigMissing
	}

	if err := ensureFalconPath(); err != nil {
		return err
	}

	if err := initConfig(cfgPath); err != nil {
		return err
	}

	if err := initLog(); err != nil {
		return err
	}

	if err := overrideIPFSConfig(rpo); err != nil {
		return err
	}

	// NOTE(kmax): it is a bit hacky to override a global var from kubo.
	corenode.Core = fx.Options(
		fx.Provide(corenode.BlockService),
		fx.Provide(corenode.Dag),
		fx.Provide(corenode.FetcherConfig),
		fx.Provide(corenode.Files),
		fx.Provide(RcPinning),
	)

	return nil
}

// InitFalconAfterNodeConstruction setups falcon initialization work after
// core.IpfsNode is constructed.
func InitFalconAfterNodeConstruction(
	req *cmds.Request,
	cctx *oldcmds.Context,
	nd *core.IpfsNode,
) (<-chan error, error) {
	// Sanity check.
	if _, ok := nd.Pinning.(*wrappedPinner); !ok {
		return nil, ErrRcPinnerMissing
	}

	log.Warn("IPFS node created", "ID", nd.Identity)

	if err := registerFalconNode(req.Context, nd); err != nil {
		return nil, err
	}

	initMetrics(req.Context, 9981)

	errc, err := initFalconGateway(req.Context, cctx, nd)
	if err != nil {
		return nil, err
	}

	return errc, nil
}
