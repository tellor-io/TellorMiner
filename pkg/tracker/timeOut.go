// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package tracker

import (
	"context"
	"fmt"

	"encoding/hex"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	tellorCommon "github.com/tellor-io/TellorMiner/pkg/common"
	"github.com/tellor-io/TellorMiner/pkg/config"
	"github.com/tellor-io/TellorMiner/pkg/contracts/getter"
	"github.com/tellor-io/TellorMiner/pkg/db"
	"github.com/tellor-io/TellorMiner/pkg/rpc"
)

type TimeOutTracker struct {
	logger log.Logger
}

func (b *TimeOutTracker) String() string {
	return "TimeOutTracker"
}

func NewTimeOutTracker(logger log.Logger) *TimeOutTracker {
	return &TimeOutTracker{
		logger: log.With(logger, "component", "timeout tracker"),
	}
}

func (b *TimeOutTracker) Exec(ctx context.Context) error {
	//cast client using type assertion since context holds generic interface{}
	client := ctx.Value(tellorCommon.ClientContextKey).(rpc.ETHClient)
	DB := ctx.Value(tellorCommon.DBContextKey).(db.DB)

	//get the single config instance
	cfg := config.GetConfig()

	//get address from config
	_fromAddress := cfg.PublicAddress
	_conAddress := cfg.ContractAddress

	//convert to address
	contractAddress := common.HexToAddress(_conAddress)

	instance, err := getter.NewTellorGetters(contractAddress, client)
	if err != nil {
		return errors.Wrap(err, "creating instance")
	}
	address := "000000000000000000000000" + _fromAddress[2:]
	decoded, err := hex.DecodeString(address)
	if err != nil {
		return errors.Wrapf(err, "decoding address")
	}
	status, err := instance.GetUintVar(nil, rpc.Keccak256(decoded))

	if err != nil {
		return errors.Wrapf(err, "getting dispute status")
	}
	enc := hexutil.EncodeBig(status)
	err = DB.Put(db.TimeOutKey, []byte(enc))
	if err != nil {
		return errors.Wrapf(err, "storing dispute info")
	}
	// Issue #50, bail out of not able to mine
	// if status.Cmp(big.NewInt(1)) != 0 {
	//testutil.Ok(t, errors.New(fmt.Spintf("Miner is not able to mine with status %v. Stopping all mining immediately", status)))
	// }

	//add all whitelisted miner addresses as well since they will be coming in
	//asking for dispute status
	for _, addr := range cfg.ServerWhitelist {
		address := "000000000000000000000000" + addr[2:]
		decoded, err := hex.DecodeString(address)
		if err != nil {
			return errors.Wrapf(err, "decoding address")
		}
		status, err := instance.GetUintVar(nil, rpc.Keccak256(decoded))
		if err != nil {
			level.Error(b.logger).Log("msg", "getting staker timeOut status for miner", "address", addr, "err", err)
		}
		if status.Int64() > 0 {
			level.Info(b.logger).Log("msg", "whitelisted miner", "addr", addr, "lastTimeMined", time.Unix(status.Int64(), 0))
		}
		from := common.HexToAddress(addr)
		dbKey := fmt.Sprintf("%s-%s", strings.ToLower(from.Hex()), db.TimeOutKey)
		err = DB.Put(dbKey, []byte(hexutil.EncodeBig(status)))
		if err != nil {
			return errors.Wrapf(err, "storing last time mined")
		}
	}
	return nil
}
