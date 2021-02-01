// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package ops

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tellor-io/telliot/pkg/contracts/master"
	"github.com/tellor-io/telliot/pkg/contracts/proxy"
	"github.com/tellor-io/telliot/pkg/testutil"
)

// TestMain check for goroutine leaks.
// It ensures that at the end of the tests there are no remaining go routines.
func TestMain(m *testing.M) {
	testutil.TolerantVerifyLeakMain(m)
}

func TestSubmission(t *testing.T) {

	backend, transactor := getbackendBackend(t)
	// Deploy a token contract on the backendulated blockchain
	_, tx1, _, err := master.DeployTellor(transactor, backend)
	testutil.Ok(t, err)
	backend.Commit()

	addr, err := bind.WaitDeployed(context.Background(), backend, tx1)
	testutil.Ok(t, err)

	_, tx2, proxy, err := proxy.DeployTellorMaster(transactor, backend, addr)
	testutil.Ok(t, err)
	backend.Commit()

	_, err = bind.WaitDeployed(context.Background(), backend, tx2)
	testutil.Ok(t, err)
	backend.Commit()

	name, err := proxy.GetName(&bind.CallOpts{Pending: true})
	testutil.Ok(t, err)

	fmt.Println("name", name)

	a, b, c, d, e, f, err := proxy.GetCurrentVariables(nil)
	testutil.Ok(t, err)
	fmt.Println(a, b, c, d, e, f)

}

func getbackendBackend(t *testing.T) (*backends.SimulatedBackend, *bind.TransactOpts) {
	sk, err := crypto.GenerateKey()
	testutil.Ok(t, err)
	faucetAddr := crypto.PubkeyToAddress(sk.PublicKey)
	addr := map[common.Address]core.GenesisAccount{
		common.BytesToAddress([]byte{1}): {Balance: big.NewInt(1)}, // ECRecover
		common.BytesToAddress([]byte{2}): {Balance: big.NewInt(1)}, // SHA256
		common.BytesToAddress([]byte{3}): {Balance: big.NewInt(1)}, // RIPEMD
		common.BytesToAddress([]byte{4}): {Balance: big.NewInt(1)}, // Identity
		common.BytesToAddress([]byte{5}): {Balance: big.NewInt(1)}, // ModExp
		common.BytesToAddress([]byte{6}): {Balance: big.NewInt(1)}, // ECAdd
		common.BytesToAddress([]byte{7}): {Balance: big.NewInt(1)}, // ECScalarMul
		common.BytesToAddress([]byte{8}): {Balance: big.NewInt(1)}, // ECPairing
		faucetAddr:                       {Balance: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(9))},
	}
	alloc := core.GenesisAlloc(addr)
	transactor, err := bind.NewKeyedTransactorWithChainID(sk, big.NewInt(1337))
	testutil.Ok(t, err)
	return backends.NewSimulatedBackend(alloc, 80000000), transactor
}
