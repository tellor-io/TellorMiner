// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package tracker

import (
	"context"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/tellor-io/TellorMiner/pkg/common"
	"github.com/tellor-io/TellorMiner/pkg/db"
	"github.com/tellor-io/TellorMiner/pkg/rpc"
	"github.com/tellor-io/TellorMiner/pkg/util"
)

func TestTimeOutString(t *testing.T) {
	tracker := NewTimeOutTracker(util.SetupLogger("debug"))
	res := tracker.String()
	if res != "TimeOutTracker" {
		t.Fatalf("should return 'TimeOutTracker' string")
	}
}

func TestTimeOutTracker(t *testing.T) {

	db, err := db.Open(filepath.Join(os.TempDir(), "test_timeOut"))
	if err != nil {
		log.Fatal(err)
		panic(err.Error())
	}

	startBal := big.NewInt(456000)
	opts := &rpc.MockOptions{ETHBalance: startBal, Nonce: 1, GasPrice: big.NewInt(700000000),
		TokenBalance: startBal, Top50Requests: []*big.Int{}}
	client := rpc.NewMockClientWithValues(opts)
	ctx := context.Background()
	ctx = context.WithValue(ctx, common.ClientContextKey, client)
	ctx = context.WithValue(ctx, common.DBContextKey, db)

	tracker := NewTimeOutTracker(util.SetupLogger("debug"))
	if err := tracker.Exec(ctx); err != nil {
		log.Fatal(err)
	}
}
