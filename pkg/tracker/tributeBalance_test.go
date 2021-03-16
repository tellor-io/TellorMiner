// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package tracker

import (
	"context"
	"math/big"
	"testing"

	"github.com/pkg/errors"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/tellor-io/telliot/pkg/config"
	"github.com/tellor-io/telliot/pkg/contracts"
	"github.com/tellor-io/telliot/pkg/db"
	"github.com/tellor-io/telliot/pkg/logging"
	"github.com/tellor-io/telliot/pkg/rpc"
	"github.com/tellor-io/telliot/pkg/testutil"
)

func TestTributeBalance(t *testing.T) {
	cfg := config.OpenTestConfig(t)
	logger := logging.NewLogger()

	startBal := big.NewInt(456000)
	opts := &rpc.MockOptions{ETHBalance: startBal, Nonce: 1, GasPrice: big.NewInt(700000000),
		TokenBalance: startBal, Top50Requests: []*big.Int{}}
	client := rpc.NewMockClientWithValues(opts)

	DB, cleanup := db.OpenTestDB(t)
	defer t.Cleanup(cleanup)
	proxy, err := db.OpenLocal(logger, cfg, DB)
	testutil.Ok(t, err)
	contract, err := contracts.NewITellor(client)
	testutil.Ok(t, err)
	accounts, err := rpc.GetAccounts()
	testutil.Ok(t, err)
	for _, account := range accounts {
		tracker := NewTributeTracker(logger, proxy, contract, account)
		err = tracker.Exec(context.Background())
		testutil.Ok(t, err)
		v, err := proxy.Get(db.TributeBalanceKeyFor(account.Address))
		testutil.Ok(t, err)
		b, err := hexutil.DecodeBig(string(v))
		testutil.Ok(t, err)
		t.Logf("Tribute Balance stored: %v\n", b)
		if b.Cmp(startBal) != 0 {
			testutil.Ok(t, errors.Errorf("Balance from client did not match what should have been stored in DB. %s != %s", b, startBal))
		}
	}
}
