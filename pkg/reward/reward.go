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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tellor-io/telliot/pkg/aggregator"
	"github.com/tellor-io/telliot/pkg/contracts"
	"github.com/tellor-io/telliot/pkg/contracts/tellor"
	eth "github.com/tellor-io/telliot/pkg/ethereum"
)

const ComponentName = "reward"

var tellorAddress = common.HexToAddress(contracts.TellorAddress)

// TODO: Refund gas value.
var refund uint64 = 0

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

func New(logger log.Logger, aggr aggregator.IAggregator, contractCaller ContractCaller) *Reward {
	return &Reward{
		aggr:           aggr,
		logger:         log.With(logger, "component", ComponentName),
		contractCaller: contractCaller,
		gasUsed:        make(map[int64]*big.Int),
		estimatedGasCostValue: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "telliot",
			Subsystem: ComponentName,
			Name:      "estimated_gas_value",
			Help:      "The estimated gas value",
		},
			[]string{"slot"},
		),
	}
}

type Reward struct {
	logger                log.Logger
	aggr                  aggregator.IAggregator
	contractCaller        ContractCaller
	gasUsed               map[int64]*big.Int
	estimatedGasCostValue *prometheus.GaugeVec
}

// Current returns the profit in percents based on the current TRB price.
func (self *Reward) Current(ctx context.Context, slot *big.Int, gasPriceEth1e18 *big.Int, client contracts.ETHClient, account *eth.Account) (int64, error) {
	gasUsed, err := self.GasUsed(ctx, slot, client, account)
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

// GasUsed estimates the gas needed by the transaction.
func (self *Reward) GasUsed(ctx context.Context, slot *big.Int, client contracts.ETHClient, account *eth.Account) (*big.Int, error) {
	// Getting abi.
	abi, _ := abi.JSON(strings.NewReader(tellor.TellorABI))

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
	vars, err = self.contractCaller.GetNewCurrentVariables(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, errors.Wrap(err, "calling GetNewCurrentVariables")
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}
	// We don't have actual values here, so we'll use math.MaxInt64 as our values.
	// As we tested, this won't make any significant difference in the estimated gas value!
	reqVals := [5]*big.Int{
		big.NewInt(math.MaxInt64),
		big.NewInt(math.MaxInt64),
		big.NewInt(math.MaxInt64),
		big.NewInt(math.MaxInt64),
		big.NewInt(math.MaxInt64),
	}
	packed, err := abi.Pack("submitMiningSolution", "", vars.RequestIds, reqVals)
	if err != nil {
		return nil, errors.Wrap(err, "packing submitMiningSolution args")
	}
	data := ethereum.CallMsg{
		From:     account.Address,
		To:       &tellorAddress,
		GasPrice: gasPrice,
		Data:     packed,
	}
	gasUsed, err := client.EstimateGas(ctx, data)
	if err != nil {
		return nil, errors.Wrap(err, "calling EstimateGas")
	}
	estimation := gasUsed - refund
	self.estimatedGasCostValue.With(
		prometheus.Labels{
			"slot": slot.String(),
		},
	).(prometheus.Gauge).Set(float64(estimation))
	return big.NewInt(int64(estimation)), nil
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
