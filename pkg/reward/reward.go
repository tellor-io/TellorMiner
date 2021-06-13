// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package reward

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/tellor-io/telliot/pkg/aggregator"
	"github.com/tellor-io/telliot/pkg/contracts"
	"github.com/tellor-io/telliot/pkg/contracts/tellor"
	eth "github.com/tellor-io/telliot/pkg/ethereum"
)

const ComponentName = "reward"

type ContractCaller interface {
	GetUintVar(opts *bind.CallOpts, _data [32]byte) (*big.Int, error)
	CurrentReward(opts *bind.CallOpts) (*big.Int, error)
	GetNewCurrentVariables(opts *bind.CallOpts) (struct {
		Challenge  [32]byte
		RequestIds [5]*big.Int
		Difficutly *big.Int
		Tip        *big.Int
	}, error)
}

func New(logger log.Logger, aggr aggregator.IAggregator, contractCaller ContractCaller, client contracts.ETHClient, account *eth.Account) *Reward {
	// Getting abi.
	abi, _ := abi.JSON(strings.NewReader(tellor.TellorABI))
	return &Reward{
		aggr:           aggr,
		logger:         log.With(logger, "component", ComponentName),
		contractCaller: contractCaller,
		gasUsed:        make(map[int64]*big.Int),
		client:         client,
		abi:            abi,
		account:        account,
	}
}

type Reward struct {
	logger         log.Logger
	aggr           aggregator.IAggregator
	contractCaller ContractCaller
	client         contracts.ETHClient
	gasUsed        map[int64]*big.Int
	abi            abi.ABI
	account        *eth.Account
}

// Current returns the profit in percents based on the current TRB price.
func (self *Reward) Current(ctx context.Context, slot *big.Int, gasPriceEth1e18 *big.Int) (int64, error) {
	gasUsed, err := self.GasUsed(ctx, slot)
	if err != nil {
		return 0, err
	}

	rewardEth1e18, err := self.rewardInEth1e18()
	if err != nil {
		return 0, errors.New("getting trb current TRB price")
	}

	txCostEth1e18 := big.NewInt(0).Mul(gasPriceEth1e18, gasUsed)
	profit := big.NewInt(0).Sub(rewardEth1e18, txCostEth1e18)
	profitPercentFloat := float64(profit.Int64()) / float64(txCostEth1e18.Int64()) * 100
	profitPercent := int64(profitPercentFloat)

	level.Debug(self.logger).Log(
		"msg", "profit checking",
		"reward", fmt.Sprintf("%.2e", float64(rewardEth1e18.Int64())),
		"txCost", fmt.Sprintf("%.2e", float64(txCostEth1e18.Int64())),
		"slot", slot,
		"gasUsed", gasUsed,
		"gasPrice", gasPriceEth1e18,
		"profit", fmt.Sprintf("%.2e", float64(profit.Int64())),
		"profitMargin", profitPercent,
	)

	return profitPercent, nil
}

var tellorAddress = common.HexToAddress(contracts.TellorAddress)

func (self *Reward) GasUsed(ctx context.Context, slot *big.Int) (*big.Int, error) {
	ticker := time.NewTicker(1 * time.Second)

	var (
		vars struct {
			Challenge  [32]byte
			RequestIds [5]*big.Int
			Difficutly *big.Int
			Tip        *big.Int
		}
		err error
	)
	// Getting current challenge.
	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("context cancelled")
		default:
		}
		vars, err = self.contractCaller.GetNewCurrentVariables(nil)
		if err != nil {
			level.Debug(self.logger).Log("msg", "calling GetNewCurrentVariables", "err", err)
			<-ticker.C
			continue
		}
		break
	}

	i := 0
	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("context cancelled")
		default:
		}
		gasPrice, err := self.client.SuggestGasPrice(ctx)
		if err != nil {
			level.Debug(self.logger).Log("msg", "calling SuggestGasPrice", "err", err)
			<-ticker.C
			continue
		}
		reqVals := [5]*big.Int{
			big.NewInt(math.MaxInt64),
			big.NewInt(math.MaxInt64),
			big.NewInt(math.MaxInt64),
			big.NewInt(math.MaxInt64),
			big.NewInt(math.MaxInt64),
		}

		packed, err := self.abi.Pack("submitMiningSolution", "", vars.RequestIds, reqVals)
		if err != nil {
			level.Debug(self.logger).Log("msg", "packing submitMiningSolution args", "err", err)
			return nil, errors.Wrap(err, "packing submitMiningSolution args")
		}
		data := ethereum.CallMsg{
			From:     self.account.Address,
			To:       &tellorAddress,
			GasPrice: gasPrice,
			Data:     packed,
		}
		gasUsed, err := self.client.EstimateGas(ctx, data)
		if err != nil {
			level.Debug(self.logger).Log("msg", "calling EstimateGas", "err", err)
			continue
		}
		if i%2 == 0 {
			fmt.Println("gasLimit(random)", gasUsed)
		} else {
			fmt.Println("gasLimit(maxInt)", gasUsed)
		}
		i++
		if i == 5 {
			return nil, ErrNoDataForSlot{slot: slot.String()}
		}
		<-ticker.C
	}

}

type ErrNoDataForSlot struct {
	slot string
}

func (e ErrNoDataForSlot) Error() string {
	return "no data for gas used for slot:" + e.slot
}

// SaveGasUsed calculates the price for a given slot.
func (self *Reward) SaveGasUsed(slot *big.Int, _gasUsed uint64) {
	gasUsed := big.NewInt(int64(_gasUsed))

	self.gasUsed[slot.Int64()] = gasUsed
	level.Info(self.logger).Log("msg", "saved transaction gas used", "amount", gasUsed.Int64(), "slot", slot.Int64())
}

func (self *Reward) rewardInEth1e18() (*big.Int, error) {
	trbAmount1e18, err := self.contractCaller.CurrentReward(nil)
	if err != nil {
		return nil, errors.New("getting currentReward from the chain")
	}

	trbPrice, confidence, err := self.aggr.TimeWeightedAvg("TRB/ETH", time.Now(), time.Hour)
	if err != nil {
		return nil, errors.New("getting the trb price from the aggregator")
	}

	if confidence < 0.5 {
		return nil, errors.New("trb price confidence too low")

	}

	rewardEth1e18 := big.NewFloat(0).Mul(big.NewFloat(0).SetInt(trbAmount1e18), big.NewFloat(trbPrice))

	rewardEth1e18Int, accuracy := rewardEth1e18.Int64()
	if accuracy != big.Exact {
		return nil, errors.New("conversion precision loss")
	}

	return big.NewInt(rewardEth1e18Int), nil
}

func (s *Reward) Slot() (*big.Int, error) {
	slot, err := s.contractCaller.GetUintVar(nil, eth.Keccak256([]byte("_SLOT_PROGRESS")))
	if err != nil {
		return nil, errors.Wrap(err, "getting _SLOT_PROGRESS")
	}
	return slot, nil
}
