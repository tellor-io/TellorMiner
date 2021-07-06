// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package reward

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/tsdb"

	"github.com/tellor-io/telliot/pkg/aggregator"
	"github.com/tellor-io/telliot/pkg/contracts"
	eth "github.com/tellor-io/telliot/pkg/ethereum"
	"github.com/tellor-io/telliot/pkg/logging"
)

type RewardQuerier struct {
	client           *ethclient.Client
	logger           log.Logger
	contractInstance *contracts.ITellor
	ctx              context.Context
	stop             context.CancelFunc

	tsDB   *tsdb.DB
	aggr   aggregator.IAggregator
	engine *promql.Engine
}

func NewRewardQuerier(
	logger log.Logger,
	ctx context.Context,
	cfg Config,
	tsDB *tsdb.DB,
	client *ethclient.Client,
	contractInstance *contracts.ITellor,
	addr common.Address,
	aggr aggregator.IAggregator,
) (*RewardQuerier, error) {
	logger, err := logging.ApplyFilter(cfg.LogLevel, logger)
	if err != nil {
		return nil, errors.Wrap(err, "apply filter logger")
	}
	logger = log.With(logger, "component", ComponentName)

	opts := promql.EngineOpts{
		Logger:               logger,
		Reg:                  nil,
		MaxSamples:           30000,
		Timeout:              10 * time.Second,
		LookbackDelta:        5 * time.Minute,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
	}
	engine := promql.NewEngine(opts)

	ctx, cncl := context.WithCancel(ctx)
	return &RewardQuerier{
		client:           client,
		logger:           logger,
		contractInstance: contractInstance,
		ctx:              ctx,
		stop:             cncl,
		tsDB:             tsDB,
		engine:           engine,
		aggr:             aggr,
	}, nil
}

// Current returns the profit in percents based on the current TRB price.
func (self *RewardQuerier) Current(ctx context.Context, slot *big.Int, gasPriceEth1e18 *big.Int) (int64, error) {
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

type ErrNoDataForSlot struct {
	slot string
}

func (e ErrNoDataForSlot) Error() string {
	return "no data for gas used for slot:" + e.slot
}

// GasUsed estimates the gas needed by the transaction.
func (self *RewardQuerier) GasUsed(ctx context.Context, slot *big.Int) (*big.Int, error) {
	query, err := self.engine.NewInstantQuery(
		self.tsDB,
		`last_over_time(gas_usage_estimated{slot="`+slot.String()+`"}[1d]) - `+`last_over_time(gas_usage_actual{slot="`+slot.String()+`"}[1d])`,
		time.Now(),
	)
	if err != nil {
		return nil, err
	}
	defer query.Close()
	refund := query.Exec(self.ctx)
	if refund.Err != nil {
		return nil, errors.Wrapf(refund.Err, "error evaluating query:%v", query.Statement())
	}
	if len(refund.Value.(promql.Vector)) == 0 {
		return nil, ErrNoDataForSlot{slot: slot.String()}
	}
	return big.NewInt(int64(refund.Value.(promql.Vector)[0].V)), nil
}

func (self *RewardQuerier) rewardInEth1e18() (*big.Int, error) {
	trbAmount1e18, err := self.contractInstance.CurrentReward(nil)
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

func (self *RewardQuerier) Slot() (*big.Int, error) {
	slot, err := self.contractInstance.GetUintVar(nil, eth.Keccak256([]byte("_SLOT_PROGRESS")))
	if err != nil {
		return nil, errors.Wrap(err, "getting _SLOT_PROGRESS")
	}
	return slot, nil
}
