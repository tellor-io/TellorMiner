// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package rest

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/pkg/errors"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/phayes/freeport"
	"github.com/tellor-io/TellorMiner/pkg/common"
	"github.com/tellor-io/TellorMiner/pkg/db"
	"github.com/tellor-io/TellorMiner/pkg/tcontext"
	"github.com/tellor-io/TellorMiner/pkg/testutil"
)

func TestServer(t *testing.T) {
	ctx, cfg, cleanup := tcontext.CreateTestContext(t)
	defer t.Cleanup(cleanup)
	cfg.ServerWhitelist = []string{"0x053b09e98ede40997546e8bb812cd838f18bb146"}

	DB := ctx.Value(common.DBContextKey).(db.DB)

	balInt := big.NewInt(350000)
	err := DB.Put(db.BalanceKey, []byte(hexutil.EncodeBig(balInt)))
	testutil.Ok(t, err)

	port, err := freeport.GetFreePort()
	testutil.Ok(t, err)

	srv, err := Create(ctx, cfg.ServerHost, uint(port))
	testutil.Ok(t, err)

	srv.Start()
	defer func() {
		if err := srv.Stop(); err != nil {
			fmt.Println("error stoping the server", err)
		}
	}()

	proxy := ctx.Value(common.DataProxyKey).(db.DataServerProxy)
	data, err := proxy.Get(db.BalanceKey)
	if err != nil {
		testutil.Ok(t, err)
	}
	if len(data) == 0 {
		testutil.Ok(t, errors.New("Expected data to be returned"))
	}
	asInt, err := hexutil.DecodeBig(string(data))
	if err != nil {
		testutil.Ok(t, err)
	}
	if asInt.Cmp(balInt) != 0 {
		testutil.Ok(t, errors.Errorf("Expected %v but received %v as balance", balInt, asInt))
	}

	t.Logf("Retrieved balance from server: %+v\n", asInt)

	/***
	ctx := context.WithValue(context.Background(), common.DBContextKey, DB)
	srv, err := Create(ctx, "localhost", 5000)
	if err != nil {
		testutil.Ok(t, err)
	}
	srv.Start()
	defer srv.Stop()

	resp, err := http.Get("http://localhost:5000/balance")
	if err != nil {
		testutil.Ok(t, err)
	}
	var bal BalTest
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&bal)
	if !strings.Contains(bal.Balance, "0x") {
		testutil.Ok(t, errors.New("Missing balance in response"))
	} else {
		t.Logf("Retrieved balance from server: %+v\n", bal)
	}
	**/

}
