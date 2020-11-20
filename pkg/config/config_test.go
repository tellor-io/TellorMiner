// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package config

import (
	"os"
	"testing"

	"github.com/tellor-io/TellorMiner/pkg/testutil"
)

func createEnvFile(t *testing.T) func() {
	f, err := os.Create(".env")
	testutil.Ok(t, err)

	_, err = f.WriteString("ETH_PRIVATE_KEY=\"0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\"")
	testutil.Ok(t, err)
	testutil.Ok(t, f.Close())

	return func() {
		os.Remove(".env")
	}
}

func TestConfig(t *testing.T) {
	//Creating a mock .ENV file to go around this issue with godotenv:
	//https://github.com/joho/godotenv/issues/43
	cleanup := createEnvFile(t)
	defer t.Cleanup(cleanup)

	cfg := OpenTestConfig(t)

	//Asserting Default Values
	testutil.Assert(t, cfg.GasMax > 0, "GasMax should have value")
	testutil.Assert(t, cfg.GasMultiplier > 0, "GasMultiplier should have value")
	testutil.Assert(t, cfg.MinConfidence > 0, "MinConfidence should have value")
	testutil.Assert(t, cfg.DisputeThreshold > 0, "DisputeThreshold should have value")

}
