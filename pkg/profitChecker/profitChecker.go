// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package profitChecker

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	tellorCommon "github.com/tellor-io/telliot/pkg/common"
	"github.com/tellor-io/telliot/pkg/config"
	"github.com/tellor-io/telliot/pkg/contracts"
	"github.com/tellor-io/telliot/pkg/contracts/tellor"
	"github.com/tellor-io/telliot/pkg/db"
	"github.com/tellor-io/telliot/pkg/logging"
	"github.com/tellor-io/telliot/pkg/tracker"
)

const ComponentName = "profitChecker"

type ProfitChecker struct {
	client           contracts.ETHClient
	logger           log.Logger
	contractInstance *contracts.ITellor
	proxy            db.DataServerProxy
	ctx              context.Context
	stop             context.CancelFunc
	addrs            []common.Address

	submitProfit *prometheus.GaugeVec
	submitCost   *prometheus.GaugeVec
}

func NewProfitChecker(
	logger log.Logger,
	ctx context.Context,
	cfg *config.Config,
	client contracts.ETHClient,
	contractInstance *contracts.ITellor,
	proxy db.DataServerProxy,
	addrs []common.Address,
) (*ProfitChecker, error) {
	logger, err := logging.ApplyFilter(*cfg, ComponentName, logger)
	if err != nil {
		return nil, errors.Wrap(err, "apply filter logger")
	}
	logger = log.With(logger, "component", ComponentName)
	ctx, cncl := context.WithCancel(ctx)

	return &ProfitChecker{
		client:           client,
		logger:           logger,
		contractInstance: contractInstance,
		proxy:            proxy,
		addrs:            addrs,
		ctx:              ctx,
		stop:             cncl,

		submitProfit: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "telliot",
			Subsystem: "mining",
			Name:      "submit_profit",
			Help:      "Accumulated submit profit in ETH",
		},
			[]string{"addr"},
		),
		submitCost: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "telliot",
			Subsystem: "mining",
			Name:      "submit_cost",
			Help:      "Accumulated submit cost in ETH",
		},
			[]string{"addr"},
		),
	}, nil
}

// Current returns the profit in percents.
func (self *ProfitChecker) Current(slot *big.Int, gasPrice *big.Int) (int64, error) {
	gasUsed, err := self.gasUsed(slot)
	if err != nil {
		return 0, err
	}
	reward, err := self.currentReward()
	if err != nil {
		return 0, errors.Wrap(err, "getting current rewards")
	}

	txCost := big.NewInt(0).Mul(gasPrice, gasUsed)
	profit := big.NewInt(0).Sub(reward, txCost)
	profitPercentFloat := float64(profit.Int64()) / float64(txCost.Int64()) * 100
	profitPercent := int64(profitPercentFloat)

	level.Debug(self.logger).Log(
		"msg", "profit checking",
		"reward", fmt.Sprintf("%.2e", float64(reward.Int64())),
		"txCost", fmt.Sprintf("%.2e", float64(txCost.Int64())),
		"slot", slot,
		"gasUsed", gasUsed,
		"gasPrice", gasPrice,
		"profit", fmt.Sprintf("%.2e", float64(profit.Int64())),
		"profitMargin", profitPercent,
	)

	return profitPercent, nil
}

func (self *ProfitChecker) Start() error {
	level.Info(self.logger).Log("msg", "starting profit tracker")
	go self.monitorCost()
	go self.monitorReward()

	<-self.ctx.Done()
	return nil
}

func (self *ProfitChecker) Stop() {
	self.stop()
}
func (self *ProfitChecker) monitorCost() {
	var err error
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var sub event.Subscription
	events := make(chan *tellor.ITellorNonceSubmitted)

	for {
		sub, err = self.nonceSubmittedSub(events)
		if err != nil {
			level.Error(self.logger).Log("msg", "initial subscribing to NonceSubmitted events failed")
			select {
			case <-ticker.C:
				continue
			case <-self.ctx.Done():
				return
			}
		}
		break
	}

	for {
		select {
		case <-self.ctx.Done():
			return
		case err := <-sub.Err():
			if err != nil {
				level.Error(self.logger).Log(
					"msg",
					"NonceSubmitted subscription error",
					"err", err)
			}

			for {
				sub, err = self.nonceSubmittedSub(events)
				if err != nil {
					level.Error(self.logger).Log("msg", "re-subscribing to NonceSubmitted events failed")
					select {
					case <-ticker.C:
						continue
					case <-self.ctx.Done():
						return
					}
				}
				break
			}
			level.Info(self.logger).Log("msg", "re-subscribed to NonceSubmitted events")
		case event := <-events:
			logger := log.With(self.logger, "addr", event.Miner.String()[:6], "tx", event.Raw.TxHash)
			go self.setCostWhenConfirmed(logger, event)
			fmt.Println("vLog cost", event)
		}
	}
}

func (self *ProfitChecker) setCostWhenConfirmed(logger log.Logger, event *tellor.ITellorNonceSubmitted) {
	if event.Raw.Removed { // Ignore remove events due to reorg.
		level.Debug(logger).Log("msg", "cost reorg even ignored")
		return
	}
	receipt, err := self.waitMined(logger, event.Raw.TxHash)
	if err != nil {
		level.Error(logger).Log("msg", "wait confirmation for cost event", "err", err)
		return
	}
	tx, _, err := self.client.TransactionByHash(self.ctx, event.Raw.TxHash)
	if err != nil {
		level.Error(logger).Log("msg", "get transaction by hash", "err", err)
		return
	}
	cost, _ := big.NewFloat(0).Mul(big.NewFloat(float64(tx.GasPrice().Int64())), big.NewFloat(float64(receipt.GasUsed))).Float64()
	level.Debug(logger).Log("msg", "adding cost", "amount", cost/1e18)
	self.submitCost.With(prometheus.Labels{"addr": event.Miner.String()[:6]}).(prometheus.Gauge).Add(cost / 1e18)
}

func (self *ProfitChecker) setProfitWhenConfirmed(logger log.Logger, event *tellor.ITellorTransferred) {
	if event.Raw.Removed { // Ignore remove events due to reorg.
		level.Debug(logger).Log("msg", "profit reorg even ignored")
		return
	}
	receipt, err := self.waitMined(logger, event.Raw.TxHash)
	if receipt.Status != types.ReceiptStatusSuccessful {
		level.Error(logger).Log("msg", "profit event status not success so no profit added", "status", receipt.Status, "raw", event.Raw.TxHash)
		return
	}
	if err != nil {
		level.Error(logger).Log("msg", "wait confirmation for profit event", "err", err)
		return
	}

	trb, err := self.convertTRBtoETH(event.Value)
	if err != nil {
		level.Error(logger).Log("msg", "convert trb to eth", "err", err)
		return
	}
	profit, _ := big.NewFloat(float64(trb.Int64())).Float64()
	level.Debug(logger).Log("msg", "adding profit", "amount", profit/1e18)
	self.submitProfit.With(prometheus.Labels{"addr": event.To.String()}).(prometheus.Gauge).Add(profit / 1e18)
}

func (self *ProfitChecker) waitMined(logger log.Logger, txHash common.Hash) (*types.Receipt, error) {
	ctx, cncl := context.WithTimeout(self.ctx, 10*time.Minute)
	defer cncl()
	queryTicker := time.NewTicker(time.Second)
	defer queryTicker.Stop()

	for {
		receipt, err := self.client.TransactionReceipt(ctx, txHash)
		if receipt != nil {
			return receipt, nil // Even when the receipt is not success will cost some eth so need to record it.
		}

		if err != nil {
			level.Error(logger).Log("msg", "receipt retrieval failed", "err", err)
		} else {
			level.Debug(logger).Log("msg", "transaction not yet mined")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-queryTicker.C:
		}
	}
}

func (self *ProfitChecker) nonceSubmittedSub(output chan *tellor.ITellorNonceSubmitted) (event.Subscription, error) {
	var tellorFilterer *tellor.ITellorFilterer
	tellorFilterer, err := tellor.NewITellorFilterer(self.contractInstance.Address, self.client)
	if err != nil {
		return nil, errors.Wrap(err, "getting ITellorFilterer instance")
	}
	sub, err := tellorFilterer.WatchNonceSubmitted(&bind.WatchOpts{}, output, nil, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting NonceSubmitted channel")
	}
	return sub, nil
}

func (self *ProfitChecker) monitorReward() {
	var err error
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var sub event.Subscription
	events := make(chan *tellor.ITellorTransferred)

	for {
		sub, err = self.transferSub(events)
		if err != nil {
			level.Error(self.logger).Log("msg", "initial subscribing to Transferred events failed")
			select {
			case <-ticker.C:
				continue
			case <-self.ctx.Done():
				return
			}
		}
		break
	}

	for {
		select {
		case <-self.ctx.Done():
			return
		case err := <-sub.Err():
			if err != nil {
				level.Error(self.logger).Log(
					"msg",
					"Transferred subscription error",
					"err", err)
			}

			// Trying to resubscribe until it succeeds.
			for {
				sub, err = self.transferSub(events)
				if err != nil {
					level.Error(self.logger).Log("msg", "re-subscribing to Transferred events failed")
					select {
					case <-ticker.C:
						continue
					case <-self.ctx.Done():
						return
					}
				}
				break
			}
			level.Info(self.logger).Log("msg", "re-subscribed to Transferred events")
		case event := <-events:
			fmt.Println("vLog tranfer", event)
			logger := log.With(self.logger, "addr", event.To.String()[:6], "tx", event.Raw.TxHash)
			go self.setProfitWhenConfirmed(logger, event)
		}
	}
}

func (self *ProfitChecker) transferSub(output chan *tellor.ITellorTransferred) (event.Subscription, error) {
	var tellorFilterer *tellor.ITellorFilterer
	tellorFilterer, err := tellor.NewITellorFilterer(self.contractInstance.Address, self.client)
	if err != nil {
		return nil, errors.Wrap(err, "getting ITellorFilterer instance")
	}
	sub, err := tellorFilterer.WatchTransferred(
		&bind.WatchOpts{},
		output,
		[]common.Address{
			common.HexToAddress("0x0000000000000000000000000000000000000000"),
		},
		self.addrs,
	)
	if err != nil {
		return nil, errors.Wrap(err, "getting Transferred channel")
	}
	return sub, nil
}

func (self *ProfitChecker) gasUsed(slot *big.Int) (*big.Int, error) {
	txID := tellorCommon.PriceTXs + slot.String()
	gas, err := self.proxy.Get(txID)
	if err != nil {
		return nil, errors.New("getting the tx eth cost from the db")
	}

	if gas == nil {
		return nil, ErrNoDataForSlot{slot: slot.String()}
	}

	return big.NewInt(0).SetBytes(gas), nil
}

// currentReward returns the current TRB rewards converted to ETH.
func (self *ProfitChecker) currentReward() (*big.Int, error) {
	reward, err := self.contractInstance.CurrentReward(nil)
	if err != nil {
		return nil, errors.New("getting currentReward from the chain")
	}
	return self.convertTRBtoETH(reward)
}

func (self *ProfitChecker) convertTRBtoETH(trb *big.Int) (*big.Int, error) {
	// val, err := self.proxy.Get(db.QueriedValuePrefix + strconv.Itoa(tracker.RequestID_TRB_ETH))
	// if err != nil {
	// 	return nil, errors.New("getting the trb price from the db")
	// }
	// if len(val) == 0 {
	// 	return nil, errors.New("the db doesn't have the trb price")
	// }
	// priceTRB, err := hexutil.DecodeBig(string(val))
	// if err != nil {
	// 	return nil, errors.New("decoding trb price from the db")
	// }

	priceTRB := big.NewInt(215510)
	wei := big.NewInt(tellorCommon.WEI)
	precisionUpscale := big.NewInt(0).Div(wei, big.NewInt(tracker.PSRs[tracker.RequestID_TRB_ETH].Granularity()))
	priceTRB.Mul(priceTRB, precisionUpscale)

	eth := big.NewInt(0).Mul(priceTRB, trb)
	eth.Div(eth, big.NewInt(1e18))
	return eth, nil
}

type ErrNoDataForSlot struct {
	slot string
}

func (e ErrNoDataForSlot) Error() string {
	return "no data for gas used for slot:" + e.slot
}

// SaveGasUsed calculates the price for a given slot.
func (self *ProfitChecker) SaveGasUsed(receipt *types.Receipt, slot *big.Int) {
	gasUsed := big.NewInt(int64(receipt.GasUsed))

	txID := tellorCommon.PriceTXs + slot.String()
	err := self.proxy.Put(txID, gasUsed.Bytes())
	if err != nil {
		level.Error(self.logger).Log("msg", "saving transaction cost", "err", err)
	}
	level.Info(self.logger).Log("msg", "saved transaction gas used", "txHash", receipt.TxHash.String(), "amount", gasUsed.Int64(), "slot", slot.Int64())
}