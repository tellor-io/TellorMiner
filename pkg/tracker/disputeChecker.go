// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package tracker

import (
	"context"
	"fmt"
	"io/ioutil"
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
	"github.com/tellor-io/TellorMiner/abi/contracts1"
	tellorCommon "github.com/tellor-io/TellorMiner/pkg/common"
	"github.com/tellor-io/TellorMiner/pkg/config"
	"github.com/tellor-io/TellorMiner/pkg/rpc"
	"github.com/tellor-io/TellorMiner/pkg/util"
)

var disputeLogger = util.NewLogger("tracker", "disputeChecker")

type disputeChecker struct {
	lastCheckedBlock uint64
}

func (c *disputeChecker) String() string {
	return "DisputeChecker"
}

// ValueCheckResult holds the details regarding the disputed value.
type ValueCheckResult struct {
	High, Low   float64
	WithinRange bool
	Datapoints  []float64
	Times       []time.Time
}

// CheckValueAtTime queries for the details regarding the disputed value.
func CheckValueAtTime(reqID uint64, val *big.Int, at time.Time) *ValueCheckResult {
	cfg := config.GetConfig()
	//

	// check the value in 5 places, spread over cfg.DisputeTimeDelta.Duration.
	var datapoints []float64
	var times []time.Time
	for i := 0; i < 5; i++ {
		t := at.Add((time.Duration(i) - 2) * cfg.DisputeTimeDelta.Duration / 5)
		fval, confidence := PSRValueForTime(int(reqID), t)
		if confidence > 0.8 {
			datapoints = append(datapoints, fval)
			times = append(times, t)
		}
	}

	if len(datapoints) == 0 {
		return nil
	}

	min := math.MaxFloat64
	max := 0.0

	for _, dp := range datapoints {
		if dp > max {
			max = dp
		}
		if dp < min {
			min = dp
		}
	}
	min *= 1 - cfg.DisputeThreshold
	max *= 1 + cfg.DisputeThreshold

	bigF := new(big.Float)
	bigF.SetInt(val)
	floatVal, _ := bigF.Float64()

	withinRange := (floatVal > min) && (floatVal < max)

	return &ValueCheckResult{
		Low:         min,
		High:        max,
		WithinRange: withinRange,
		Datapoints:  datapoints,
		Times:       times,
	}
}

func (c *disputeChecker) Exec(ctx context.Context, logger log.Logger) error {

	client := ctx.Value(tellorCommon.ClientContextKey).(rpc.ETHClient)

	header, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get latest eth block header: %v", err)
	}
	if c.lastCheckedBlock == 0 {
		c.lastCheckedBlock = header.Number.Uint64()
	}

	toCheck := header.Number.Uint64()

	const blockDelay = 100
	if toCheck-c.lastCheckedBlock < blockDelay {
		return nil
	}

	tokenAbi, err := abi.JSON(strings.NewReader(contracts1.TellorLibraryABI))
	if err != nil {
		return fmt.Errorf("failed to parse abi: %v", err)
	}
	contractAddress := ctx.Value(tellorCommon.ContractAddress).(common.Address)

	//just use nil for most of the variables, only using this object to call UnpackLog which only uses the abi
	bar := bind.NewBoundContract(contractAddress, tokenAbi, nil, nil, nil)

	checkUntil := toCheck - blockDelay
	nonceSubmitID := tokenAbi.Events["NonceSubmitted"].ID()
	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(c.lastCheckedBlock)),
		ToBlock:   big.NewInt(int64(checkUntil)),
		Addresses: []common.Address{contractAddress},
		Topics:    [][]common.Hash{{nonceSubmitID}},
	}
	logs, err := client.FilterLogs(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to filter eth logs: %v", err)
	}
	blockTimes := make(map[uint64]time.Time)
	for _, l := range logs {
		nonceSubmit := contracts1.TellorLibraryNonceSubmitted{}
		err := bar.UnpackLog(&nonceSubmit, "NonceSubmitted", l)
		if err != nil {
			return fmt.Errorf("failed to unpack into object: %v", err)
		}
		blockTime, ok := blockTimes[l.BlockNumber]
		if !ok {
			header, err := client.HeaderByNumber(ctx, big.NewInt(int64(l.BlockNumber)))
			if err != nil {
				return fmt.Errorf("failed to get nonce block header: %v", err)
			}
			blockTime = time.Unix(int64(header.Time), 0)
			blockTimes[l.BlockNumber] = blockTime
		}
		reqID := nonceSubmit.RequestId.Uint64()
		result := CheckValueAtTime(reqID, nonceSubmit.Value, blockTime)
		if result == nil {
			level.Warn(logger).Log("msg", "no value data", "reqid", reqID, "blockTime", blockTime)
			continue
		}
		if !result.WithinRange {
			s := fmt.Sprintf("suspected incorrect value for requestID %d at %s:\n , nearest values:\n", reqID, blockTime)
			for i, pt := range result.Datapoints {
				s += fmt.Sprintf("\t%.0f, ", pt)
				delta := blockTime.Sub(result.Times[i])
				if delta > 0 {
					s += fmt.Sprintf("%s before\n", delta.String())
				} else {
					s += fmt.Sprintf("%s after\n", (-delta).String())
				}
			}
			s += fmt.Sprintf("value submitted by miner with address %s", nonceSubmit.Miner)
			disputeLogger.Error(s)
			filename := fmt.Sprintf("possible-dispute-%s.txt", blockTime)
			err := ioutil.WriteFile(filename, []byte(s), 0655)
			if err != nil {
				disputeLogger.Error("failed to save dispute data to %s: %v", filename, err)
			}
		} else {
			level.Info(logger).Log("msg", "value appears to be within expected range", "reqID", reqID, "value", nonceSubmit.Value, "blockTime", blockTime.String())
		}

	}
	c.lastCheckedBlock = checkUntil
	return nil
}
