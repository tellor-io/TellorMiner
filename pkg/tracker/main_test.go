// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package tracker

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/tellor-io/telliot/pkg/apiOracle"
	"github.com/tellor-io/telliot/pkg/config"
	"github.com/tellor-io/telliot/pkg/logging"
)

// TODO: Set threshold low and test the  "out of range" failure.
var configJSON = `{
    "publicAddress": "92f91500e105e3051f3cf94616831b58f6bce1e8",
    "trackerCycle": 1,
    "trackers": {},
    "dbFile": "/tellorDB",
	"requestTips": 1,
	"logger": {"db.Db":"DEBUG"},
    "configFolder": "` + filepath.Join("..", "..", "configs") + `",
    "envFile": "` + filepath.Join("..", "..", "configs", ".env.example") + `"
}
`

func TestMain(m *testing.M) {
	err := config.ParseConfigBytes([]byte(configJSON))
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse mock config: %v\n", err)
		os.Exit(-1)
	}
	if err := apiOracle.EnsureValueOracle(logging.NewLogger(), config.GetConfig()); err != nil {
		log.Fatal(err)
	}

	os.Exit(m.Run())
}
