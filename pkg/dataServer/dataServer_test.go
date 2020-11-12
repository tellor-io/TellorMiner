// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package dataServer

import (
	"fmt"

	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/tellor-io/TellorMiner/pkg/tcontext"
	"github.com/tellor-io/TellorMiner/pkg/util"
)

func TestDataServer(t *testing.T) {
	exitCh := make(chan int)
	logSetup := util.SetupLogger()
	logger := logSetup("debug")
	ctx, cfg, cleanup := tcontext.CreateTestContext(t)
	defer t.Cleanup(cleanup)

	ds, err := CreateServer(ctx, logger)
	if err != nil {
		t.Fatalf("error creating server in test: %s", err)
	}
	if err := ds.Start(ctx, exitCh); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)

	resp, err := http.Get("http://" + cfg.ServerHost + ":" + strconv.Itoa(int(cfg.ServerPort)) + "/balance")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	fmt.Printf("Finished: %+v", resp)
	exitCh <- 1
	time.Sleep(1 * time.Second)
	if !ds.Stopped {
		t.Fatal("Did not stop server")
	}
}
