// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package tracker

import (
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/tellor-io/TellorMiner/pkg/apiOracle"
	"github.com/tellor-io/TellorMiner/pkg/config"
	"github.com/tellor-io/TellorMiner/pkg/util"
)

// TODO: Set threshold low and test the  "out of range" failure.
var configJSON = `{
	"publicAddress":"0000000000000000000000000000000000000000",
	"privateKey":"1111111111111111111111111111111111111111111111111111111111111111",
	"contractAddress":"0x724D1B69a7Ba352F11D73fDBdEB7fF869cB22E19",
	"trackers": {"disputeChecker": true},
	"ConfigFolder": "..",
	"disputeThreshold": 1.0,
	"disputeTimeDelta": "50s"
}
`

func TestMain(m *testing.M) {
	err := config.ParseConfigBytes([]byte(configJSON))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse mock config: %v\n", err)
		os.Exit(-1)
	}
	if err := util.ParseLoggingConfig(""); err != nil {
		log.Fatal(err)
	}
	if err := apiOracle.EnsureValueOracle(); err != nil {
		log.Fatal(err)
	}

	os.Exit(m.Run())
}
