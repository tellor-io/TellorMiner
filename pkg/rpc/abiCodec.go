// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package rpc

import (
	"encoding/json"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	balancer "github.com/tellor-io/telliot/pkg/contracts/balancer"
	"github.com/tellor-io/telliot/pkg/contracts/tellorCurrent"
	"github.com/tellor-io/telliot/pkg/contracts/tellorMaster"
	uniswap "github.com/tellor-io/telliot/pkg/contracts/uniswap"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

// ABICodec holds abi definitions for encoding/decoding contract methods and events.
type ABICodec struct {
	abiStruct abi.ABI
	methods   map[string]*abi.Method
	Events    map[string]*abi.Event
}

// BuildCodec constructs a merged abi structure representing all methods/events for Tellor tellor. This is primarily
// used for mock encoding/decoding parameters but could also be used for manual RPC operations that do not rely on geth's contract impl.
func BuildCodec(logger log.Logger) (*ABICodec, error) {
	all := []string{
		tellorCurrent.TellorDisputeABI,
		tellorCurrent.TellorLibraryABI,
		tellorCurrent.TellorGettersLibraryABI,
		tellorCurrent.TellorStakeABI,
		tellorCurrent.TellorTransferABI,
		tellorMaster.TellorGettersABI,
		balancer.BPoolABI,
		balancer.BTokenABI,
		uniswap.IERC20ABI,
		uniswap.IUniswapV2PairABI,
	}

	parsed := make([]interface{}, 0)
	for _, abi := range all {
		var f interface{}
		if err := json.Unmarshal([]byte(abi), &f); err != nil {
			return nil, err
		}
		asList := f.([]interface{})
		parsed = append(parsed, asList...)
	}
	j, err := json.Marshal(parsed)
	if err != nil {
		return nil, err
	}
	abiStruct, err := abi.JSON(strings.NewReader(string(j)))
	if err != nil {
		return nil, err
	}
	methodMap := make(map[string]*abi.Method)
	eventMap := make(map[string]*abi.Event)
	for _, a := range abiStruct.Methods {
		sig := hexutil.Encode(a.ID)
		level.Debug(logger).Log("msg", "mapping method sig", "sig", sig, "method", a.Name)
		methodMap[sig] = &abi.Method{Name: a.Name, Constant: a.Constant, Inputs: a.Inputs, Outputs: a.Outputs}
	}
	for _, e := range abiStruct.Events {
		sig := hexutil.Encode(e.ID.Bytes())
		//abiCodecLog.Debug("Mapping event sig: %s to event %s", sig, e.Name)
		level.Debug(logger).Log("msg", "mapping method sig", "sig", sig, "method", e.Name)
		eventMap[sig] = &abi.Event{Name: e.Name, Anonymous: e.Anonymous, Inputs: e.Inputs}
	}

	return &ABICodec{abiStruct, methodMap, eventMap}, nil
}

// AllEventsthis lets you quickly find the type of each event. It is helpful for debugging.
func AllEvents() (map[[32]byte]abi.Event, error) {
	all := []string{
		tellorCurrent.TellorABI,
		tellorCurrent.TellorDisputeABI,
		tellorCurrent.TellorGettersLibraryABI,
		tellorCurrent.TellorLibraryABI,
		tellorCurrent.TellorStakeABI,
		tellorCurrent.TellorStorageABI,
		tellorCurrent.TellorTransferABI,
		balancer.BPoolABI,
		balancer.BTokenABI,
		uniswap.IERC20ABI,
		uniswap.IUniswapV2PairABI,
	}

	parsed := make([]interface{}, 0)
	for _, abi := range all {
		var f interface{}
		if err := json.Unmarshal([]byte(abi), &f); err != nil {
			return nil, err
		}
		asList := f.([]interface{})
		parsed = append(parsed, asList...)
	}
	j, err := json.Marshal(parsed)
	if err != nil {
		return nil, err
	}
	abiStruct, err := abi.JSON(strings.NewReader(string(j)))
	if err != nil {
		return nil, err
	}
	eventMap := make(map[[32]byte]abi.Event)
	for _, e := range abiStruct.Events {
		eventMap[e.ID] = e
	}

	return eventMap, nil
}
