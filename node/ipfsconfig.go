package node

import (
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	kuboconfig "github.com/ipfs/kubo/config"
	serialize "github.com/ipfs/kubo/config/serialize"
	"github.com/ipfs/kubo/repo"
	"github.com/libp2p/go-libp2p/core/peer"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/mitchellh/go-homedir"

	"github.com/photon-storage/go-common/log"

	"github.com/photon-storage/falcon/node/config"
)

func overrideIPFSConfig(repoPath string, repo repo.Repo) error {
	expPath, err := homedir.Expand(filepath.Clean(repoPath))
	if err != nil {
		return err
	}
	repoPath = expPath

	rcfg, err := repo.Config()
	if err != nil {
		return err
	}

	falconCfg := config.Get()

	// TODO(kmax): experiment with limit user overrides
	limitCfg, err := repo.UserResourceOverrides()
	if err != nil {
		return err
	}
	limitCfg.System.Conns = rcmgr.LimitVal(512)
	limitCfg.System.Streams = rcmgr.DefaultLimit
	limitCfg.System.FD = rcmgr.LimitVal(512)
	limitCfg.System.Memory = rcmgr.LimitVal64(512 << 20)
	limitCfg.Transient.Conns = rcmgr.LimitVal(768)
	limitCfg.Transient.Streams = rcmgr.DefaultLimit
	limitCfg.Transient.FD = rcmgr.LimitVal(768)
	limitCfg.Transient.Memory = rcmgr.LimitVal64(768 << 20)
	if err := serialize.WriteConfigFile(
		filepath.Join(repoPath, "libp2p-resource-limit-overrides.json"),
		limitCfg,
	); err != nil {
		return err
	}

	modified := false

	// Resource Manager
	swarmRcMgrChanged := false
	maxMem := falconCfg.IPFSConfig.MaxMemMBytes
	if maxMem > 0 {
		setOptString(
			&rcfg.Swarm.ResourceMgr.MaxMemory,
			fmt.Sprintf("%d mib", maxMem),
			&modified,
		)
		swarmRcMgrChanged = true
	}

	maxFd := falconCfg.IPFSConfig.MaxFileDescriptors
	if maxFd > 0 {
		setOptInt(
			&rcfg.Swarm.ResourceMgr.MaxFileDescriptors,
			int64(maxFd),
			&modified,
		)
		swarmRcMgrChanged = true
	}

	if true || swarmRcMgrChanged {
		setFlag(
			&rcfg.Swarm.ResourceMgr.Enabled,
			kuboconfig.True,
			&modified,
		)
	}

	// Connection Manager
	swarmConnMgrChanged := false
	cmLow := falconCfg.IPFSConfig.ConnMgrLowWater
	if cmLow > 0 {
		setOptInt(
			&rcfg.Swarm.ConnMgr.LowWater,
			int64(cmLow),
			&modified,
		)
		swarmConnMgrChanged = true
	}
	cmHigh := falconCfg.IPFSConfig.ConnMgrHighWater
	if cmHigh > 0 {
		setOptInt(
			&rcfg.Swarm.ConnMgr.HighWater,
			int64(cmHigh),
			&modified,
		)
		swarmConnMgrChanged = true
	}
	cmGracePeriod := falconCfg.IPFSConfig.ConnMgrGracePeriod
	if cmGracePeriod > 0 {
		setOptDuration(
			&rcfg.Swarm.ConnMgr.GracePeriod,
			cmGracePeriod,
			&modified,
		)
		swarmConnMgrChanged = true
	}
	if swarmConnMgrChanged {
		setOptString(
			&rcfg.Swarm.ConnMgr.Type,
			kuboconfig.DefaultConnMgrType,
			&modified,
		)
	}

	// Relay Client
	relayCli := kuboconfig.True
	if falconCfg.IPFSConfig.DisableRelayClient {
		relayCli = kuboconfig.False
	}
	setFlag(
		&rcfg.Swarm.RelayClient.Enabled,
		relayCli,
		&modified,
	)

	var peers []peer.AddrInfo
	for _, idStr := range falconCfg.IPFSConfig.Peers {
		id, err := peer.Decode(idStr)
		if err != nil {
			return err
		}
		peers = append(peers, peer.AddrInfo{
			ID: id,
		})
	}
	setPeers(
		&rcfg.Peering.Peers,
		peers,
		&modified,
	)

	// Enforce Gateway CORs
	setHeaders(
		rcfg.Gateway.HTTPHeaders,
		"Access-Control-Allow-Origin",
		[]string{"*"},
		&modified,
	)
	setHeaders(
		rcfg.Gateway.HTTPHeaders,
		"Access-Control-Allow-Methods",
		[]string{
			"GET",
			"POST",
			"DELETE",
			"PUT",
			"OPTIONS",
		},
		&modified,
	)
	setHeaders(
		rcfg.Gateway.HTTPHeaders,
		"Access-Control-Allow-Headers",
		[]string{
			"Accept",
			"Content-Type",
			"Content-Length",
			"Accept-Encoding",
			"X-CSRF-Token",
			"Authorization",
		},
		&modified,
	)
	setHeaders(
		rcfg.Gateway.HTTPHeaders,
		"Access-Control-Expose-Headers",
		[]string{
			"IPFS-Hash",
		},
		&modified,
	)

	// Force IPNS pubsub
	setFlag(
		&rcfg.Ipns.UsePubsub,
		kuboconfig.True,
		&modified,
	)

	// No change.
	if !modified {
		return nil
	}

	// Backup existing IPFS change and persist new config.
	prefix := fmt.Sprintf("falcon-%v", time.Now().Unix())
	if _, err := repo.BackupConfig(prefix); err != nil {
		return err
	}

	log.Warn("Falcon config change detected. Created IPFS config backup")

	//rcfg.Swarm.EnableHolePunching = config.False
	//rcfg.Swarm.DisableNatPortMap = true
	//rcfg.Swarm.RelayService.Enabled = config.False
	//rcfg.Swarm.RelayClient.Enabled = config.True

	//rcfg.Routing.Type = config.NewOptionalString("dhtclient")
	//rcfg.AutoNAT.ServiceMode = config.AutoNATServiceDisabled
	//rcfg.Reprovider.Interval = config.NewOptionalDuration(0)

	return repo.SetConfig(rcfg)
}

func setOptString(
	ptr **kuboconfig.OptionalString,
	val string,
	modified *bool,
) {
	if *ptr == nil || (*ptr).String() != val {
		*ptr = kuboconfig.NewOptionalString(val)
		*modified = true
	}
}

func setOptInt(
	ptr **kuboconfig.OptionalInteger,
	val int64,
	modified *bool,
) {
	if *ptr == nil || (*ptr).String() != fmt.Sprintf("%d", val) {
		*ptr = kuboconfig.NewOptionalInteger(val)
		*modified = true
	}
}

func setOptDuration(
	ptr **kuboconfig.OptionalDuration,
	val time.Duration,
	modified *bool,
) {
	if *ptr == nil || (*ptr).String() != val.String() {
		*ptr = kuboconfig.NewOptionalDuration(val)
		*modified = true
	}
}

func setFlag(
	ptr *kuboconfig.Flag,
	val kuboconfig.Flag,
	modified *bool,
) {
	if *ptr != val {
		*ptr = val
		*modified = true
	}
}

func setPeers(
	ptr *[]peer.AddrInfo,
	val []peer.AddrInfo,
	modified *bool,
) {
	matched := true
	if len(*ptr) == len(val) {
		for idx, p := range *ptr {
			if p.String() != val[idx].String() {
				matched = false
				break
			}
		}
	} else {
		matched = false
	}
	if matched {
		return
	}

	*ptr = val
	*modified = true
}

func setHeaders(
	m map[string][]string,
	k string,
	val []string,
	modified *bool,
) {
	if reflect.DeepEqual(m[k], val) {
		return
	}
	m[k] = val
	*modified = true
}
