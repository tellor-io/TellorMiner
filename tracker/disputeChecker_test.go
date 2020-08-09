package tracker

import (
	"context"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	tellorCommon "github.com/tellor-io/TellorMiner/common"
	"github.com/tellor-io/TellorMiner/config"
	"github.com/tellor-io/TellorMiner/db"
	"github.com/tellor-io/TellorMiner/rpc"
)

func TestDisputeCheckerInRange(t *testing.T) {
	opts := &rpc.MockOptions{ETHBalance: big.NewInt(300000), Nonce: 1, GasPrice: big.NewInt(7000000000),
		TokenBalance: big.NewInt(0), Top50Requests: []*big.Int{}}
	disputeChecker := &disputeChecker{ lastCheckedBlock: 500}
	DB, err := db.Open(filepath.Join(os.TempDir(), "disputeChecker_test"))
	if err != nil {
		t.Fatal(err)
	}
	client := rpc.NewMockClientWithValues(opts)
	ctx := context.WithValue(context.Background(), tellorCommon.ClientContextKey, client)
	ctx = context.WithValue(ctx, tellorCommon.DBContextKey, DB)
	psrs, err := BuildIndexTrackers()
	execEthUsdPsrs(ctx, t, psrs)
	time.Sleep(2*time.Second)
	execEthUsdPsrs(ctx, t, psrs)
	ctx = context.WithValue(ctx, tellorCommon.ContractAddress, common.Address{0x0000000000000000000000000000000000000000})
	err = disputeChecker.Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}
	DB.Close()
}

func TestDisputeCheckerOutOfRange(t *testing.T) {
	cfg := config.GetConfig()
	cfg.DisputeThreshold = 0.000000001
	opts := &rpc.MockOptions{ETHBalance: big.NewInt(300000), Nonce: 1, GasPrice: big.NewInt(7000000000),
		TokenBalance: big.NewInt(0), Top50Requests: []*big.Int{}}
	disputeChecker := &disputeChecker{ lastCheckedBlock: 500}
	DB, err := db.Open(filepath.Join(os.TempDir(), "disputeChecker_test"))
	if err != nil {
		t.Fatal(err)
	}
	client := rpc.NewMockClientWithValues(opts)
	ctx := context.WithValue(context.Background(), tellorCommon.ClientContextKey, client)
	ctx = context.WithValue(ctx, tellorCommon.DBContextKey, DB)
	psrs, err := BuildIndexTrackers()
	execEthUsdPsrs(ctx, t, psrs)
	time.Sleep(2*time.Second)
	execEthUsdPsrs(ctx, t, psrs)
	ctx = context.WithValue(ctx, tellorCommon.ContractAddress, common.Address{0x0000000000000000000000000000000000000000})
	err = disputeChecker.Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}
	DB.Close()
}

func execEthUsdPsrs(ctx context.Context, t *testing.T, psrs []Tracker) {
	// These indexes represent all ETH api indexes tracked in tellor
	// The values are obtained through sorting all query endpoints in alphabetetical order
	indexes := [6]int{19, 20, 52, 68, 78, 84}
	for _, psrIdx  := range indexes {
		//fmt.Print("\nIndex: ", psrIdx, psrs[psrIdx])
		err := psrs[psrIdx].Exec(ctx)
		if err != nil {
			t.Fatalf("failed to execute psr: %v", err)
		}
	}
}
