// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package rpc

import (
	"context"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	tellorCommon "github.com/tellor-io/telliot/pkg/common"
	"github.com/tellor-io/telliot/pkg/config"
	"github.com/tellor-io/telliot/pkg/contracts"
	"github.com/tellor-io/telliot/pkg/db"
)

// contractWrapper is internal wrapper of contract instance for calling common contract functions.
type contractWrapper struct {
	options     *bind.TransactOpts
	fromAddress common.Address

	*contracts.ITellor
}

func (c contractWrapper) AddTip(requestID *big.Int, amount *big.Int) (*types.Transaction, error) {
	return c.ITellor.AddTip(c.options, requestID, amount)
}

func (c contractWrapper) SubmitSolution(solution string, requestID [5]*big.Int, value [5]*big.Int) (*types.Transaction, error) {
	return c.ITellor.SubmitMiningSolution(c.options, solution, requestID, value)
}

func (c contractWrapper) DidMine(challenge [32]byte) (bool, error) {
	return c.ITellor.DidMine(nil, challenge, c.fromAddress)
}

func SubmitContractTxn(
	ctx context.Context,
	logger log.Logger,
	cfg *config.Config,
	proxy db.DataServerProxy,
	client contracts.ETHClient,
	tellor *contracts.ITellor,
	account *Account,
	ctxName string,
	callback tellorCommon.TransactionGeneratorFN,
) (*types.Transaction, error) {

	nonce, err := client.NonceAt(ctx, account.Address)
	if err != nil {
		return nil, errors.Wrap(err, "getting nonce for miner address")
	}

	// Use the same nonce in case there is a stuck transaction so that it submits with the current nonce but higher gas price.
	IntNonce := int64(nonce)
	keys := []string{
		db.GasKey,
	}
	m, err := proxy.BatchGet(keys)
	if err != nil {
		return nil, errors.Wrap(err, "getting data from the db")
	}
	gasPrice := getInt(m[db.GasKey])
	if gasPrice.Cmp(big.NewInt(0)) == 0 {
		level.Warn(logger).Log("msg", "no gas price from DB, falling back to client suggested gas price")
		gasPrice, err = client.SuggestGasPrice(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "determine gas price to submit txn")
		}
	}
	mul := cfg.GasMultiplier
	if mul > 0 {
		level.Info(logger).Log("msg", "settings gas price multiplier", "value", mul)
		gasPrice = gasPrice.Mul(gasPrice, big.NewInt(int64(mul)))
	}

	var finalError error
	for i := 0; i <= 5; i++ {
		balance, err := client.BalanceAt(ctx, account.Address, nil)
		if err != nil {
			finalError = err
			continue
		}

		cost := big.NewInt(1)
		cost = cost.Mul(gasPrice, big.NewInt(200000))
		if balance.Cmp(cost) < 0 {
			// FIXME: notify someone that we're out of funds!
			finalError = errors.Errorf("insufficient funds to send transaction: %v < %v", balance, cost)
			continue
		}

		netID, err := client.NetworkID(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "getting network id")
		}
		auth, err := bind.NewKeyedTransactorWithChainID(account.PrivateKey, netID)
		if err != nil {
			return nil, errors.Wrap(err, "creating transactor")
		}
		auth.Nonce = big.NewInt(IntNonce)
		auth.Value = big.NewInt(0)      // in weiF
		auth.GasLimit = uint64(3000000) // in units
		if gasPrice.Cmp(big.NewInt(0)) == 0 {
			gasPrice = big.NewInt(100)
		}
		if i > 1 {
			gasPrice1 := new(big.Int).Set(gasPrice)
			gasPrice1.Mul(gasPrice1, big.NewInt(int64(i*11))).Div(gasPrice1, big.NewInt(int64(100)))
			auth.GasPrice = gasPrice1.Add(gasPrice, gasPrice1)
		} else {
			// First time, try base gas price.
			auth.GasPrice = gasPrice
		}
		max := cfg.GasMax
		var maxGasPrice *big.Int
		gasPrice1 := big.NewInt(tellorCommon.GWEI)
		if max > 0 {
			maxGasPrice = gasPrice1.Mul(gasPrice1, big.NewInt(int64(max)))
		} else {
			maxGasPrice = gasPrice1.Mul(gasPrice1, big.NewInt(int64(100)))
		}

		if auth.GasPrice.Cmp(maxGasPrice) > 0 {
			level.Info(logger).Log("msg", "gas price too high, will default to the max price", "current", auth.GasPrice, "defaultMax", maxGasPrice)
			auth.GasPrice = maxGasPrice
		}

		wrapper := contractWrapper{auth, account.Address, tellor}
		tx, err := callback(ctx, wrapper)
		if err != nil {
			if strings.Contains(err.Error(), "nonce too low") { // Can't use error type matching because of the way the eth client is implemented.
				IntNonce = IntNonce + 1
				level.Debug(logger).Log("msg", "last transaction has been confirmed so will increase the nonce and resend the transaction.")

			} else if strings.Contains(err.Error(), "replacement transaction underpriced") { // Can't use error type matching because of the way the eth client is implemented.
				level.Debug(logger).Log("msg", "last transaction is stuck so will increase the gas price and try to resend")
				finalError = err
			} else {
				finalError = errors.Wrap(err, "callback")
			}

			delay := 15 * time.Second
			level.Debug(logger).Log("msg", "will retry a send", "retryDelay", delay)
			select {
			case <-ctx.Done():
				return nil, errors.New("the submit context was canceled")
			case <-time.After(delay):
				continue
			}
		}
		return tx, nil
	}
	return nil, errors.Wrapf(finalError, "submit txn after 5 attempts ctx:%v", ctxName)
}

func getInt(data []byte) *big.Int {
	if len(data) == 0 {
		return nil
	}

	val, err := hexutil.DecodeBig(string(data))
	if err != nil {
		return nil
	}
	return val
}

func Keccak256(input []byte) [32]byte {
	hash := crypto.Keccak256(input)
	var hashed [32]byte
	copy(hashed[:], hash)

	return hashed
}
