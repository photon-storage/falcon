package node

import (
	"strings"

	ipfslog "github.com/ipfs/go-log"
	"go.uber.org/zap/zapcore"

	"github.com/photon-storage/go-common/log"
)

func initLog() error {
	cfg := Cfg()

	logLvl, err := log.ParseLevel(cfg.Log.Level)
	if err != nil {
		return err
	}

	if err := log.Init(logLvl, log.TextFormat, true); err != nil {
		return err
	}

	if cfg.Log.FilePath != "" {
		if err := log.ConfigurePersistentLogging(
			cfg.Log.FilePath,
			false,
		); err != nil {
			return err
		}
	}
	if cfg.Log.Color {
		log.ForceColor()
	} else {
		log.DisableColor()
	}

	if cfg.Log.IpfsSubsystems != "" {
		lvl := toIpfsLogLevel(logLvl)
		for _, name := range strings.Split(cfg.Log.IpfsSubsystems, ",") {
			name = strings.TrimSpace(name)
			if name == "*" {
				ipfslog.SetAllLoggers(lvl)
			} else {
				ipfslog.SetLogLevel(name, zapcore.Level(lvl).String())
			}
		}
	}

	return nil
}

func toIpfsLogLevel(lvl log.Level) ipfslog.LogLevel {
	switch lvl {
	case log.PanicLevel:
		return ipfslog.LevelPanic
	case log.FatalLevel:
		return ipfslog.LevelFatal
	case log.ErrorLevel:
		return ipfslog.LevelError
	case log.WarnLevel:
		return ipfslog.LevelWarn
	case log.InfoLevel:
		return ipfslog.LevelInfo
	case log.DebugLevel:
		return ipfslog.LevelDebug
	case log.TraceLevel:
		return ipfslog.LevelDebug
	}
	return ipfslog.LevelInfo
}
