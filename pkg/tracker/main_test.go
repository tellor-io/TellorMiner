// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package tracker

import (
	"fmt"
	"io/ioutil"
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
	"Trackers": {"names":{}},
    "DbFile": "/tellorDB",
	"Logger": {"db.Db":"DEBUG"},
    "EnvFile": "` + filepath.Join("..", "..", "configs", ".env.example") + `",
    "ApiFile": "` + filepath.Join("..", "..", "configs", "api.json") + `",
    "ManualDataFile": "` + filepath.Join("..", "..", "configs", "manualData.json") + `"
}
`

func TestMain(m *testing.M) {
	mainConfigFile, err := ioutil.TempFile(os.TempDir(), "testing")
	if err != nil {
		log.Fatal("Cannot create temporary file", err)
	}
	defer os.Remove(mainConfigFile.Name())

	if _, err = mainConfigFile.Write([]byte(configJSON)); err != nil {
		log.Fatal("write the main config file", err)
	}
	if err := mainConfigFile.Close(); err != nil {
		log.Fatal(err)
	}
	cfg, err := config.ParseConfig(mainConfigFile.Name())
	if err != nil {
		log.Fatal(err)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "parse config %v\n", err)
		os.Exit(-1)
	}
	if err := apiOracle.EnsureValueOracle(logging.NewLogger(), cfg); err != nil {
		log.Fatal(err)
	}

	os.Exit(m.Run())
}
