// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package config

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
	"github.com/tellor-io/telliot/pkg/aggregator"
	"github.com/tellor-io/telliot/pkg/db"
	"github.com/tellor-io/telliot/pkg/format"
	"github.com/tellor-io/telliot/pkg/gasPrice/gasStation"
	"github.com/tellor-io/telliot/pkg/mining"
	psrTellor "github.com/tellor-io/telliot/pkg/psr/tellor"
	psrTellorMesosphere "github.com/tellor-io/telliot/pkg/psr/tellorMesosphere"
	"github.com/tellor-io/telliot/pkg/submitter/tellor"
	"github.com/tellor-io/telliot/pkg/submitter/tellorMesosphere"
	"github.com/tellor-io/telliot/pkg/tasker"
	"github.com/tellor-io/telliot/pkg/tracker/dispute"
	"github.com/tellor-io/telliot/pkg/tracker/index"
	"github.com/tellor-io/telliot/pkg/tracker/profit"
	"github.com/tellor-io/telliot/pkg/tracker/reward"
	"github.com/tellor-io/telliot/pkg/transactor"
	"github.com/tellor-io/telliot/pkg/web"
)

// Config is the top-level configuration that holds configs for all components.
type Config struct {
	Web                       web.Config
	Mining                    mining.Config
	SubmitterTellor           tellor.Config
	SubmitterTellorMesosphere tellorMesosphere.Config
	ProfitTracker             profit.Config
	RewardTracker             reward.Config
	Tasker                    tasker.Config
	Transactor                transactor.Config
	IndexTracker              index.Config
	DisputeTracker            dispute.Config
	Aggregator                aggregator.Config
	PsrTellor                 psrTellor.Config
	PsrTellorMesosphere       psrTellorMesosphere.Config
	Db                        db.Config
	GasStation                gasStation.Config
	// EnvFile location that include all private details like private key etc.
	EnvFile string `json:"envFile"`
}

var DefaultConfig = Config{
	Mining: mining.Config{
		LogLevel:  "info",
		Heartbeat: time.Minute,
	},
	Web: web.Config{
		LogLevel:   "info",
		ListenHost: "", // Listen on all addresses.
		ListenPort: 9090,
	},
	Db: db.Config{
		LogLevel:      "info",
		Path:          "db",
		RemoteTimeout: format.Duration{Duration: 5 * time.Second},
	},
	Tasker: tasker.Config{
		LogLevel: "info",
	},
	ProfitTracker: profit.Config{
		LogLevel: "info",
	},
	RewardTracker: reward.Config{
		LogLevel: "info",
	},
	DisputeTracker: dispute.Config{
		LogLevel: "info",
	},
	Transactor: transactor.Config{
		LogLevel:      "info",
		GasMax:        10,
		GasMultiplier: 1,
	},
	SubmitterTellor: tellor.Config{
		Enabled:  true,
		LogLevel: "info",
		// With a 1 second delay here as a workaround to prevent a race condition in the oracle contract check.
		MinSubmitPeriod: format.Duration{Duration: 15*time.Minute + 1*time.Second},
	},
	SubmitterTellorMesosphere: tellorMesosphere.Config{
		LogLevel:             "info",
		MinSubmitPeriod:      format.Duration{Duration: 15 * time.Second},
		MinSubmitPriceChange: 0.05,
	},
	PsrTellor: psrTellor.Config{
		MinConfidence: 70,
	},
	Aggregator: aggregator.Config{
		LogLevel:       "info",
		ManualDataFile: "configs/manualData.json",
	},
	GasStation: gasStation.Config{
		TimeWait: format.Duration{Duration: time.Minute},
	},
	IndexTracker: index.Config{
		LogLevel:  "info",
		Interval:  format.Duration{Duration: 30 * time.Second},
		IndexFile: "configs/index.json",
	},
	EnvFile: "configs/.env",
}

func ParseConfig(logger log.Logger, path string) (*Config, error) {

	cfg := &Config{}

	cfgI, err := DeеpCopy(logger, path, cfg, DefaultConfig)
	cfg = cfgI.(*Config)

	if err := godotenv.Load(cfg.EnvFile); err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(err, "loading env vars from env file")
	}

	return cfg, err
}

func DeеpCopy(logger log.Logger, path string, cfg, cfgDefault interface{}) (interface{}, error) {
	if path == "" {
		path = filepath.Join("configs", "config.json")
	}

	// DeepCopy the default config into the final config.
	{
		b, err := json.Marshal(cfgDefault)
		if err != nil {
			return nil, errors.Wrap(err, "marshal default config")
		}

		if err := json.Unmarshal(b, cfg); err != nil {
			return nil, errors.Wrap(err, "copy default config")
		}
	}

	f, err := os.Open(path)
	var noConfigFile bool
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, errors.Wrap(err, "open config file")
		}
		noConfigFile = true
		level.Warn(logger).Log("msg", "no config file on disk so using defaults", "path", path)
	}

	if !noConfigFile {
		dec := json.NewDecoder(f)
		dec.DisallowUnknownFields()
		for {
			// Override defaults with the custom configs.
			if err := dec.Decode(cfg); err == io.EOF {
				break
			} else if err != nil {
				return nil, errors.Wrap(err, "parse config")
			}

		}
	}

	return cfg, nil
}
