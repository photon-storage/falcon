package node

import (
	"fmt"
	"net"
	gohttp "net/http"
	"os"
	"strconv"
	"time"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"gopkg.in/yaml.v2"

	"github.com/photon-storage/go-gw3/common/auth"
	gw3net "github.com/photon-storage/go-gw3/common/net"
)

type httpClient interface {
	Do(req *gohttp.Request) (*gohttp.Response, error)
}

type ListenAddress struct {
	Address string `yaml:"address"`
	UseTLS  bool   `yaml:"use_tls"`
}

// Config defines the config for falcon gateway.
type Config struct {
	// Log configuration.
	Log struct {
		// Log level, supported name:
		// "panic", "fatal", "error", "warn", "warning",
		// "info", "debug", "trace"
		Level string `yaml:"level"`
		// Comma separated list of IPFS subsystem names to apply log level.
		// "*" means set to all.
		// Empty means leave it to default.
		IpfsSubsystems string `yaml:"ipfs_subsystems"`
		// Log file path. Default to stdout if empty.
		FilePath string `yaml:"file_path"`
		// If enable ANSI color in log.
		Color bool `yaml:"color"`
	}

	// Auth configs API authentication.
	Auth struct {
		// Disable authentication for API requests.
		// This should only be enabled for testing purpose.
		NoAuth                  bool     `yaml:"no_auth"`
		RedirectOnFailure       bool     `yaml:"redirect_on_failure"`
		StarbasePublicKeyBase64 string   `yaml:"starbase_public_key_base64"`
		Whitelist               []string `yaml:"whitelist"`
	} `yaml:"auth"`

	// IPFSConfig provides a simple means to config Kubo node behavior.
	// For advanced IPFS config, either use Kubo CLI command or edit
	// ~/.ipfs/config directly.
	IPFSConfig struct {
		MaxMemMBytes       int           `yaml:"max_mem_mbytes"`
		MaxFileDescriptors int           `yaml:"max_file_descriptors"`
		ConnMgrLowWater    int           `yaml:"conn_mgr_low_water"`
		ConnMgrHighWater   int           `yaml:"conn_mgr_high_water"`
		ConnMgrGracePeriod time.Duration `yaml:"conn_mgr_grace_period"`
		DisableRelayClient bool          `yaml:"disable_relay_client"`
	} `yaml:"ipfs_config"`

	ListenAddresses []ListenAddress `yaml:"listen_addresses"`

	ExternalServices struct {
		Starbase  string `yaml:"starbase"`
		Spaceport string `yaml:"spaceport"`
	} `yaml:"extern_services"`

	Discovery struct {
		PublicHost string `yaml:"public_host"`
		PublicPort int    `yaml:"public_port"`
	} `yaml:"discovery"`

	// Read from env FALCON_SECRET_KEY if empty.
	SecretKeyBase64 string `yaml:"secret_key_base64"`

	// Derived fields
	SecretKey       libp2pcrypto.PrivKey
	PublicKeyBase64 string
	GW3Hostname     string
	HttpClient      httpClient
}

// Global Config instance read-only after init.
var _falconCfg *Config

func initConfig(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("error opening falcon config file: %w", err)
	}
	defer f.Close()

	var cfg Config
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return fmt.Errorf("error decoding falcon config file: %w", err)
	}

	if cfg.Discovery.PublicHost == "" {
		cfg.Discovery.PublicHost = os.Getenv("FALCON_PUBLIC_HOST")
	}
	if cfg.Discovery.PublicPort == 0 {
		val := os.Getenv("FALCON_PUBLIC_PORT")
		if val != "" {
			port, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return fmt.Errorf(
					"error parsing falcon public port from env: %w", err)
			}
			if port > 0 {
				cfg.Discovery.PublicPort = int(port)
			}
		}
		if cfg.Discovery.PublicPort == 0 {
			cfg.Discovery.PublicPort = 80
		}
	}

	if cfg.SecretKeyBase64 == "" {
		cfg.SecretKeyBase64 = os.Getenv("FALCON_SECRET_KEY")
	}
	if cfg.SecretKeyBase64 == "" {
		return fmt.Errorf("secret key is missing")
	}
	sk, err := auth.DecodeSk(cfg.SecretKeyBase64)
	if err != nil {
		return fmt.Errorf(
			"error unmarshaling secret key: %w", err)
	}
	cfg.SecretKey = sk
	pkStr, err := auth.EncodePk(sk.GetPublic())
	if err != nil {
		return fmt.Errorf(
			"error marshaling public key : %w", err)
	}
	cfg.PublicKeyBase64 = pkStr

	cfg.GW3Hostname = cfg.Discovery.PublicHost
	if addr := net.ParseIP(cfg.GW3Hostname); addr != nil {
		// Convert IP to expected hashed domain name
		cfg.GW3Hostname = gw3net.GW3Hostname(cfg.GW3Hostname)
	}

	cfg.HttpClient = gohttp.DefaultClient

	_falconCfg = &cfg
	return nil
}

func (c *Config) RequireTLSCert() bool {
	for _, la := range Cfg().ListenAddresses {
		if la.UseTLS {
			return true
		}
	}
	return false
}

func Cfg() *Config {
	if _falconCfg == nil {
		panic("Falcon config is not initialized")
	}
	return _falconCfg
}

func MockCfg(cfg *Config) {
	_falconCfg = cfg
}
