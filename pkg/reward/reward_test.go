// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package reward

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/params"
	"github.com/go-kit/kit/log"
	"github.com/tellor-io/telliot/pkg/testutil"
)

// TestProfitCalculation ensures correct calculation and pricision conversion of the reward tracker.
// With
// GAS usage of 1
// GAS price of 1ETH
// TRB price of 1ETH
// the rewards percent should equal the trb reward amount.
// Example
// half TRB reward or 5e17 should equal in -50% profit.
// 1 TRB reward or 1e18 should equal in 0% profit.
// 2 TRB reward or 2e18 should equal in 100% profit.
func TestProfitCalculation(t *testing.T) {
	logger := log.NewNopLogger()

	gasUsed := float64(1)
	gasCost := float64(params.Ether)
	costTotal := gasUsed * gasCost

	trbPrice := float64(1)

	aggregator := &MockAggr{TRBPrice: trbPrice}
	slotNum := big.NewInt(1)

	for _, rewardAmount := range []float64{5e17, 1e18, 2e18, 3e18} {
		contractCaller := &MockContractCaler{trbRewardAmount: big.NewInt(int64(rewardAmount))}
		reward := New(logger, aggregator, contractCaller, nil, nil)
		reward.SaveGasUsed(slotNum, uint64(gasUsed))

		rewardAct, err := reward.Current(context.Background(), slotNum, big.NewInt(int64(gasCost)))
		testutil.Ok(t, err)
		profit := rewardAmount*trbPrice - costTotal
		profitPercent := (profit / costTotal) * 100
		testutil.Equals(t, profitPercent, float64(rewardAct))
	}
}

type MockAggr struct {
	TRBPrice float64
}

func (self *MockAggr) TimeWeightedAvg(_ string, _ time.Time, _ time.Duration) (float64, float64, error) {
	return self.TRBPrice, 1, nil
}

type MockContractCaler struct {
	trbRewardAmount *big.Int
}

func (*MockContractCaler) GetUintVar(opts *bind.CallOpts, _data [32]byte) (*big.Int, error) {
	return nil, nil
}

func (self *MockContractCaler) CurrentReward(opts *bind.CallOpts) (*big.Int, error) {
	return self.trbRewardAmount, nil
}

func (self *MockContractCaler) GetNewCurrentVariables(opts *bind.CallOpts) (struct {
	Challenge  [32]byte
	RequestIds [5]*big.Int
	Difficutly *big.Int
	Tip        *big.Int
}, error) {
	return struct {
		Challenge  [32]byte
		RequestIds [5]*big.Int
		Difficutly *big.Int
		Tip        *big.Int
	}{}, nil
}
