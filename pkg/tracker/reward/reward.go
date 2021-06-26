// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package reward

import (
	"context"
	"math/big"
	"strings"
	"time"

	"github.com/bluele/gcache"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/event"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tellor-io/telliot/pkg/contracts"
	"github.com/tellor-io/telliot/pkg/contracts/tellor"
	"github.com/tellor-io/telliot/pkg/logging"
)

const ComponentName = "reward"
const DefaultRetry = 30 * time.Second

type Config struct {
	LogLevel string
}

type GasUsageTracker struct {
	netID            *big.Int
	client           *ethclient.Client
	logger           log.Logger
	contractInstance *contracts.ITellor
	abi              abi.ABI
	ctx              context.Context
	stop             context.CancelFunc
	addrs            []common.Address
	addrsMap         map[common.Address]struct{} // The same as above but used for quick matching.

	cacheTXsProfit     gcache.Cache
	cacheTXsCost       gcache.Cache
	cacheTXsCostFailed gcache.Cache
	lastFailedBlock    int64

	submitProfit *prometheus.GaugeVec
	submitCost   *prometheus.GaugeVec
	balances     *prometheus.GaugeVec
}

func NewGasUsageTracker(
	logger log.Logger,
	ctx context.Context,
	cfg Config,
	client *ethclient.Client,
	contractInstance *contracts.ITellor,
	addrs []common.Address,
) (*GasUsageTracker, error) {
	logger, err := logging.ApplyFilter(cfg.LogLevel, logger)
	if err != nil {
		return nil, errors.Wrap(err, "apply filter logger")
	}
	logger = log.With(logger, "component", ComponentName)

	abi, err := abi.JSON(strings.NewReader(tellor.TellorABI))
	if err != nil {
		return nil, errors.Wrap(err, "abi read")
	}

	addrsMap := make(map[common.Address]struct{})
	for _, addr := range addrs {
		addrsMap[addr] = struct{}{}
	}

	netID, err := client.NetworkID(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get nerwork ID")
	}

	ctx, cncl := context.WithCancel(ctx)

	return &GasUsageTracker{
		netID:            netID,
		client:           client,
		logger:           logger,
		contractInstance: contractInstance,
		abi:              abi,
		addrs:            addrs,
		addrsMap:         addrsMap,
		ctx:              ctx,
		stop:             cncl,

		cacheTXsProfit:     gcache.New(50).LRU().Build(),
		cacheTXsCost:       gcache.New(50).LRU().Build(),
		cacheTXsCostFailed: gcache.New(20).LRU().Build(),
	}, nil
}

func (self *GasUsageTracker) Start() error {
	level.Info(self.logger).Log("msg", "starting")

	go self.monitorGas()

	<-self.ctx.Done()
	return nil
}

func (self *GasUsageTracker) Stop() {
	self.stop()
}

func (self *GasUsageTracker) monitorGas() {
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
			self.recordGasData(event)
			self.recordGasEstimation()
		}
	}
}

func (self *GasUsageTracker) recordGasData(event *tellor.TellorNonceSubmitted) {
	ticker := time.NewTicker(DefaultRetry)
	defer ticker.Stop()
	for {
		select {
		case <-self.ctx.Done():
			level.Debug(self.logger).Log("msg", "context canceled, exiting...")
			return
		default:
		}
		receipt, err := self.client.TransactionReceipt(self.ctx, event.Raw.TxHash)
		if err != nil {
			level.Error(self.logger).Log("msg", "receipt retrieval", "err", err)
		} else if receipt != nil && receipt.Status == types.ReceiptStatusSuccessful { // Failed transactions cost is monitored in a different process.
			// tx, _, err := self.client.TransactionByHash(self.ctx, event.Raw.TxHash)
			// if err != nil {
			// 	level.Error(self.logger).Log("msg", "get transaction by hash", "err", err)
			// 	return
			// }
			level.Debug(self.logger).Log("msg", "adding gas used", "gasUsed", receipt.GasUsed)
			if err := self.addGasUsed(receipt.GasUsed); err != nil {
				level.Error(self.logger).Log("msg", "adding gas used", "err", err)
			}
			return
		}

		level.Debug(self.logger).Log("msg", "transaction not yet mined")
		<-ticker.C
		continue
	}
}

func (self *GasUsageTracker) addGasUsed(gasUsed uint64) error {
	// TODO:
	return nil
}

func (self *GasUsageTracker) recordGasEstimation() error {
	// TODO:
	return nil
}

func (self *GasUsageTracker) nonceSubmittedSub(output chan *tellor.TellorNonceSubmitted) (event.Subscription, error) {
	tellorFilterer, err := tellor.NewTellorFilterer(self.contractInstance.Address, self.client)
	if err != nil {
		return nil, errors.Wrap(err, "getting instance")
	}
	sub, err := tellorFilterer.WatchNonceSubmitted(&bind.WatchOpts{Context: self.ctx}, output, self.addrs, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting channel")
	}
	return sub, nil
}

func txIDNonceSubmit(event *tellor.TellorNonceSubmitted) string {
	return event.Raw.TxHash.String() + event.Miner.String()
}
