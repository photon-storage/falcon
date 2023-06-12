package node

import (
	"bytes"
	"fmt"
	"time"

	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/repo"

	"github.com/photon-storage/go-common/log"
)

func overrideIPFSConfig(repo repo.Repo) error {
	rcfg, err := repo.Config()
	if err != nil {
		return err
	}

	origJson, err := config.Marshal(rcfg)
	if err != nil {
		return err
	}

	falconCfg := Cfg()
	// Resource Manager
	swarmRcMgrChanged := false
	maxMem := falconCfg.IPFSConfig.MaxMemMBytes
	if maxMem > 0 {
		rcfg.Swarm.ResourceMgr.MaxMemory =
			config.NewOptionalString(fmt.Sprintf("%d mib", maxMem))
		swarmRcMgrChanged = true
	}

	maxFd := falconCfg.IPFSConfig.MaxFileDescriptors
	if maxFd > 0 {
		rcfg.Swarm.ResourceMgr.MaxFileDescriptors =
			config.NewOptionalInteger(int64(maxFd))
		swarmRcMgrChanged = true
	}

	if swarmRcMgrChanged {
		rcfg.Swarm.ResourceMgr.Enabled = config.True
	}

	// Connection Manager
	swarmConnMgrChanged := false
	cmLow := falconCfg.IPFSConfig.ConnMgrLowWater
	if cmLow > 0 {
		rcfg.Swarm.ConnMgr.LowWater = config.NewOptionalInteger(int64(cmLow))
		swarmConnMgrChanged = true
	}
	cmHigh := falconCfg.IPFSConfig.ConnMgrHighWater
	if cmHigh > 0 {
		rcfg.Swarm.ConnMgr.HighWater = config.NewOptionalInteger(int64(cmHigh))
		swarmConnMgrChanged = true
	}
	cmGracePeriod := falconCfg.IPFSConfig.ConnMgrGracePeriod
	if cmGracePeriod > 0 {
		rcfg.Swarm.ConnMgr.GracePeriod =
			config.NewOptionalDuration(cmGracePeriod)
		swarmConnMgrChanged = true
	}
	if swarmConnMgrChanged {
		rcfg.Swarm.ConnMgr.Type =
			config.NewOptionalString(config.DefaultConnMgrType)
	}

	// Relay Client
	if falconCfg.IPFSConfig.DisableRelayClient {
		rcfg.Swarm.RelayClient.Enabled = config.False
	} else {
		rcfg.Swarm.RelayClient.Enabled = config.True
	}

	// Enforce Gateway CORs
	rcfg.Gateway.HTTPHeaders["Access-Control-Allow-Origin"] = []string{"*"}
	rcfg.Gateway.HTTPHeaders["Access-Control-Allow-Methods"] = []string{
		"GET",
		"POST",
		"DELETE",
		"PUT",
		"OPTIONS",
	}
	rcfg.Gateway.HTTPHeaders["Access-Control-Allow-Headers"] = []string{
		"Accept",
		"Content-Type",
		"Content-Length",
		"Accept-Encoding",
		"X-CSRF-Token",
		"Authorization",
	}
	rcfg.Gateway.HTTPHeaders["Access-Control-Expose-Headers"] = []string{
		"IPFS-Hash",
	}

	// Force IPNS pubsub
	rcfg.Ipns.UsePubsub = config.True

	// Check if config has been changed.
	updatedJson, err := config.Marshal(rcfg)
	if err != nil {
		return err
	}

	// No change.
	if bytes.Compare(origJson, updatedJson) == 0 {
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
