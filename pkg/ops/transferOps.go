// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package ops

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/tellor-io/telliot/pkg/config"
	"github.com/tellor-io/telliot/pkg/contracts"
	"github.com/tellor-io/telliot/pkg/util"
)

/**
 * This is the operational transfer component. Its purpose is to transfer tellor tokens
 */

func prepareTransfer(
	ctx context.Context,
	logger log.Logger,
	client contracts.ETHClient,
	tellor *contracts.ITellor,
	account *config.Account,
	amt *big.Int,
) (*bind.TransactOpts, error) {
	balance, err := tellor.BalanceOf(nil, account.Address)
	if err != nil {
		return nil, errors.Wrap(err, "get balance")
	}
	level.Info(logger).Log("msg", "check my balance", util.FormatERC20Balance(balance))
	if balance.Cmp(amt) < 0 {
		return nil, errors.Errorf("insufficient balance TRB actual: %v, requested: %v",
			util.FormatERC20Balance(balance),
			util.FormatERC20Balance(amt))
	}
	auth, err := util.PrepareEthTransaction(ctx, client, account)
	if err != nil {
		return nil, errors.Wrap(err, "preparing ethereum transaction")
	}
	return auth, nil
}

func Transfer(
	ctx context.Context,
	logger log.Logger,
	client contracts.ETHClient,
	tellor *contracts.ITellor,
	account *config.Account,
	toAddress common.Address,
	amt *big.Int,
) error {
	auth, err := prepareTransfer(ctx, logger, client, tellor, account, amt)
	if err != nil {
		return errors.Wrap(err, "preparing transfer")
	}

	tx, err := tellor.Transfer(auth, toAddress, amt)
	if err != nil {
		return errors.Wrap(err, "calling transfer")
	}
	level.Info(logger).Log(
		"msg", "transferred",
		"amount", util.FormatERC20Balance(amt),
		"to", toAddress.String()[:12],
		"tx Hash", tx.Hash().Hex(),
	)
	return nil
}

func Approve(
	ctx context.Context,
	logger log.Logger,
	client contracts.ETHClient,
	tellor *contracts.ITellor,
	account *config.Account,
	spender common.Address,
	amt *big.Int,
) error {
	auth, err := prepareTransfer(ctx, logger, client, tellor, account, amt)
	if err != nil {
		return errors.Wrap(err, "preparing transfer")
	}

	tx, err := tellor.Approve(auth, spender, amt)
	if err != nil {
		return errors.Wrap(err, "calling approve")
	}
	level.Info(logger).Log("msg", "approved", "amount", util.FormatERC20Balance(amt), "spender", spender.String()[:12], "tx Hash", tx.Hash().Hex())
	return nil
}

func Balance(ctx context.Context, logger log.Logger, client contracts.ETHClient, tellor *contracts.ITellor,
	addr common.Address) error {
	ethBalance, err := client.BalanceAt(ctx, addr, nil)
	if err != nil {
		return errors.Wrap(err, "get eth balance")
	}
	trbBalance, err := tellor.BalanceOf(nil, addr)
	if err != nil {
		return errors.Wrapf(err, "getting trb balance")
	}
	level.Info(logger).Log(
		"msg", "balance check",
		"address", addr.String(),
		"ETH", fmt.Sprintf("%10s", util.FormatERC20Balance(ethBalance)),
		"TRB", fmt.Sprintf("%10s", util.FormatERC20Balance(trbBalance)),
	)
	return nil
}
