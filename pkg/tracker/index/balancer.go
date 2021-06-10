// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package index

import (
	"context"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/tellor-io/telliot/pkg/contracts"
	balancer "github.com/tellor-io/telliot/pkg/contracts/balancer"
)

// BalancerPair to be fetched onchain.
type BalancerPair struct {
	token1Address  common.Address
	token2Address  common.Address
	token1Decimals uint64
	token2Decimals uint64
}

// Balancer implements DataSource interface.
type Balancer struct {
	address  string
	token1   string
	token2   string
	client   contracts.ETHClient
	interval time.Duration
}

func NewBalancer(pair, address string, interval time.Duration, client contracts.ETHClient) *Balancer {
	tokens := strings.Split(pair, "/")
	return &Balancer{
		interval: interval,
		address:  address,
		token1:   tokens[0],
		token2:   tokens[1],
		client:   client,
	}
}

func (b *Balancer) Get(ctx context.Context) (float64, error) {
	// Getting current pair info from input pool.
	pair, err := b.getPair()
	if err != nil {
		return 0, errors.Wrap(err, "getting pair info from balancer pool")
	}
	// Use balancer pool own GetSpotPrice to minimize onchain calls.
	price, err := b.getSpotPrice(ctx, pair)
	if err != nil {
		return 0, errors.Wrap(err, "getting price info from balancer pool")
	}
	return price, nil
}

func (b *Balancer) Interval() time.Duration {
	return b.interval
}

func (b *Balancer) Source() string {
	return b.address
}

func (b *Balancer) getPair() (*BalancerPair, error) {
	var poolCaller *balancer.BPoolCaller
	poolCaller, err := balancer.NewBPoolCaller(common.HexToAddress(b.address), b.client)
	if err != nil {
		return nil, err
	}
	currentTokens, err := poolCaller.GetCurrentTokens(&bind.CallOpts{})
	if err != nil {
		return nil, err
	}

	pair := &BalancerPair{}
	var token1Seen, token2Seen bool
	for _, token := range currentTokens {
		var tokenCaller *balancer.BTokenCaller
		tokenCaller, err = balancer.NewBTokenCaller(token, b.client)
		if err != nil {
			return nil, err
		}
		var symbol string
		var decimals uint8
		symbol, err = tokenCaller.Symbol(&bind.CallOpts{})
		if err != nil {
			return nil, err
		}
		decimals, err = tokenCaller.Decimals(&bind.CallOpts{})
		if err != nil {
			return nil, err
		}
		if symbol == b.token1 {
			pair.token1Address = token
			pair.token1Decimals = uint64(decimals)
			token1Seen = true
		} else if symbol == b.token2 {
			pair.token2Address = token
			pair.token2Decimals = uint64(decimals)
			token2Seen = true
		}
	}
	if !token1Seen || !token2Seen {
		return nil, errors.New("we expected this pool to have the provided tokens")
	}
	return pair, nil
}

func (b *Balancer) getSpotPrice(ctx context.Context, pair *BalancerPair) (float64, error) {
	var poolCaller *balancer.BPoolCaller
	poolCaller, err := balancer.NewBPoolCaller(common.HexToAddress(b.address), b.client)
	if err != nil {
		return 0, err
	}

	// Getting token1 price per token2.
	spotPrice, err := poolCaller.GetSpotPrice(&bind.CallOpts{Context: ctx}, pair.token2Address, pair.token1Address)
	if err != nil {
		return 0, err
	}
	decimals, err := poolCaller.Decimals(&bind.CallOpts{})
	if err != nil {
		return 0, err
	}

	_spotPrice := big.NewFloat(0).SetInt(spotPrice)
	price, _ := new(big.Float).Quo(_spotPrice, new(big.Float).SetFloat64(math.Pow10(int(uint64(decimals)+pair.token2Decimals-pair.token1Decimals)))).Float64()
	return price, nil
}
