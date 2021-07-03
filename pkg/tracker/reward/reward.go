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
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/params"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/tsdb"

	"github.com/tellor-io/telliot/pkg/aggregator"
	"github.com/tellor-io/telliot/pkg/contracts"
	"github.com/tellor-io/telliot/pkg/contracts/tellor"
	eth "github.com/tellor-io/telliot/pkg/ethereum"
	"github.com/tellor-io/telliot/pkg/logging"
)

const ComponentName = "rewardTracker"
const DefaultRetry = 30 * time.Second

type Config struct {
	LogLevel string
}

type RewardTracker struct {
	client           *ethclient.Client
	logger           log.Logger
	contractInstance *contracts.ITellor
	abi              abi.ABI
	ctx              context.Context
	stop             context.CancelFunc
	addr             common.Address

	tsDB   *tsdb.DB
	aggr   aggregator.IAggregator
	engine *promql.Engine
}

func NewRewardTracker(
	logger log.Logger,
	ctx context.Context,
	cfg Config,
	tsDB *tsdb.DB,
	client *ethclient.Client,
	contractInstance *contracts.ITellor,
	addr common.Address,
	aggr aggregator.IAggregator,
) (*RewardTracker, error) {
	logger, err := logging.ApplyFilter(cfg.LogLevel, logger)
	if err != nil {
		return nil, errors.Wrap(err, "apply filter logger")
	}
	logger = log.With(logger, "component", ComponentName)

	abi, err := abi.JSON(strings.NewReader(tellor.TellorABI))
	if err != nil {
		return nil, errors.Wrap(err, "abi read")
	}

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
	return &RewardTracker{
		client:           client,
		logger:           logger,
		contractInstance: contractInstance,
		abi:              abi,
		addr:             addr,
		ctx:              ctx,
		stop:             cncl,
		tsDB:             tsDB,
		engine:           engine,
		aggr:             aggr,
	}, nil
}

func (self *RewardTracker) Start() error {
	level.Info(self.logger).Log("msg", "starting")

	go self.monitorGas()

	<-self.ctx.Done()
	return nil
}

func (self *RewardTracker) Stop() {
	self.stop()
}

func (self *RewardTracker) monitorGas() {
	var err error
	ticker := time.NewTicker(DefaultRetry)
	defer ticker.Stop()

	logger := log.With(self.logger, "event", "NonceSubmitted")

	var sub event.Subscription
	events := make(chan *tellor.TellorNonceSubmitted)
	for {
		select {
		case <-self.ctx.Done():
			return
		default:
		}
		sub, err = self.nonceSubmittedSub(events)
		if err != nil {
			level.Error(logger).Log("msg", "initial subscribing to events failed")
			<-ticker.C
			continue
		}
		break
	}

	for {
		select {
		case <-self.ctx.Done():
			return
		case err := <-sub.Err():
			if err != nil {
				level.Error(logger).Log(
					"msg",
					"subscription error",
					"err", err)
			}

			// Trying to resubscribe until it succeeds.
			for {
				select {
				case <-self.ctx.Done():
					return
				default:
				}
				sub, err = self.nonceSubmittedSub(events)
				if err != nil {
					level.Error(logger).Log("msg", "re-subscribing to events failed", "err", err)
					<-ticker.C
					continue
				}
				break
			}
			level.Info(logger).Log("msg", "re-subscribed to events")
		case event := <-events:
			err := self.recordGasUsage(event)
			if err != nil {
				level.Error(self.logger).Log("msg", "record gas usage", "err", err)
			}
			err = self.recordGasUsageEstimated(self.ctx)
			if err != nil {
				level.Error(self.logger).Log("msg", "record gas usage estimation", "err", err)
			}
		}
	}
}

func (self *RewardTracker) recordGasUsage(event *tellor.TellorNonceSubmitted) error {
	receipt, err := self.client.TransactionReceipt(self.ctx, event.Raw.TxHash)
	if err != nil {
		return errors.Wrap(err, "receipt retrieval")
	} else if receipt != nil && receipt.Status == types.ReceiptStatusSuccessful { // Failed transactions cost is monitored in a different process.
		// tx, _, err := self.client.TransactionByHash(self.ctx, event.Raw.TxHash)
		// if err != nil {
		// 	level.Error(self.logger).Log("msg", "get transaction by hash", "err", err)
		// 	return
		// }
		level.Debug(self.logger).Log("msg", "adding gas used", "gasUsed", receipt.GasUsed)
		if err := self.addGasUsed(event.Slot.String(), receipt.GasUsed); err != nil {
			return errors.Wrap(err, "adding gas used")
		}
		return nil
	}
	return errors.New("transaction not yet mined")
}

func (self *RewardTracker) addGasUsed(slot string, gasUsed uint64) error {
	var err error
	appender := self.tsDB.Appender(self.ctx)

	// Round up the time so that all appends happen with the same TS and
	// avoid out of order samples errors.
	ts := timestamp.FromTime(time.Now().Round(5 * time.Second))

	defer func() { // An appender always needs to be committed or rolled back.
		if err != nil {
			if err := appender.Rollback(); err != nil {
				level.Error(self.logger).Log("msg", "db rollback failed", "err", err)
			}
			return
		}
		if errC := appender.Commit(); errC != nil {
			err = errors.Wrap(err, "db append commit failed")
		}
	}()
	lbls := labels.Labels{
		labels.Label{Name: "__name__", Value: "gas_usage_actual"},
		labels.Label{Name: "slot", Value: slot},
	}

	_, err = appender.Append(0, lbls, ts, float64(gasUsed))
	if err != nil {
		return errors.Wrap(err, "append values to the DB")
	}
	return nil
}

func (self *RewardTracker) recordGasUsageEstimated(ctx context.Context) error {
	ctx, cncl := context.WithTimeout(ctx, 2*time.Second)
	defer cncl()
	// Getting abi.
	abi, err := abi.JSON(strings.NewReader(tellor.TellorABI))
	if err != nil {
		return errors.Wrap(err, "getting abi")
	}

	// Getting current slot.
	slot, err := self.Slot()
	if err != nil {
		return errors.Wrap(err, "getting current slot")
	}

	// Getting current challenge.
	vars, err := self.contractInstance.GetNewCurrentVariables(&bind.CallOpts{Context: ctx})
	if err != nil {
		return errors.Wrap(err, "call GetNewCurrentVariables")
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
		return errors.Wrap(err, "packing submitMiningSolution args")
	}
	data := ethereum.CallMsg{
		From: self.addr,
		To:   &self.contractInstance.Address,
		// Hardcoded gas price. Looks like it doesn't matter in the calculation.
		GasPrice: big.NewInt(0).Mul(big.NewInt(30), big.NewInt(params.GWei)),
		Data:     packed,
	}
	gasEstimation, err := self.client.EstimateGas(ctx, data)
	if err != nil {
		return errors.Wrap(err, "call client.EstimateGas")
	}
	err = self.addGasEstimation(slot, gasEstimation)
	if err != nil {
		return errors.Wrap(err, "add gas estimation to the db")
	}
	return nil
}

func (self *RewardTracker) addGasEstimation(slot *big.Int, gasEstimation uint64) error {
	var err error
	appender := self.tsDB.Appender(self.ctx)

	// Round up the time so that all appends happen with the same TS and
	// avoid out of order samples errors.
	ts := timestamp.FromTime(time.Now().Round(5 * time.Second))

	defer func() { // An appender always needs to be committed or rolled back.
		if err != nil {
			if err := appender.Rollback(); err != nil {
				level.Error(self.logger).Log("msg", "db rollback failed", "err", err)
			}
			return
		}
		if errC := appender.Commit(); errC != nil {
			err = errors.Wrap(err, "db append commit failed")
		}
	}()
	lbls := labels.Labels{
		labels.Label{Name: "__name__", Value: "gas_usage_estimated"},
		labels.Label{Name: "slot", Value: slot.String()},
	}

	_, err = appender.Append(0, lbls, ts, float64(gasEstimation))
	if err != nil {
		return errors.Wrap(err, "append values to the DB")
	}
	return nil
}

func (self *RewardTracker) nonceSubmittedSub(output chan *tellor.TellorNonceSubmitted) (event.Subscription, error) {
	tellorFilterer, err := tellor.NewTellorFilterer(self.contractInstance.Address, self.client)
	if err != nil {
		return nil, errors.Wrap(err, "getting instance")
	}
	sub, err := tellorFilterer.WatchNonceSubmitted(&bind.WatchOpts{Context: self.ctx}, output, []common.Address{}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting channel")
	}
	return sub, nil
}

// Current returns the profit in percents based on the current TRB price.
func (self *RewardTracker) Current(ctx context.Context, slot *big.Int, gasPriceEth1e18 *big.Int) (int64, error) {
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

// GasUsed estimates the gas needed by the transaction.
func (self *RewardTracker) GasUsed(ctx context.Context, slot *big.Int) (*big.Int, error) {
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
		return nil, errors.Errorf("no vals for refund interval query:%v", query.Statement())
	}

	return big.NewInt(int64(refund.Value.(promql.Vector)[0].V)), nil
}

func (self *RewardTracker) rewardInEth1e18() (*big.Int, error) {
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

func (self *RewardTracker) Slot() (*big.Int, error) {
	slot, err := self.contractInstance.GetUintVar(nil, eth.Keccak256([]byte("_SLOT_PROGRESS")))
	if err != nil {
		return nil, errors.Wrap(err, "getting _SLOT_PROGRESS")
	}
	return slot, nil
}
