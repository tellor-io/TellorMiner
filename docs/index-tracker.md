---
description: The Index Tracker module collects and stores data from HTTP or Blockchain endpoints.
---

# Index Tracker

The index tracker module uses an `index.json` file.

The structure of the file is as follow:

```javascript
{
    "AMPL/USD/VWAP": {
        "interval": "10m",
        "endpoints": [
            {
                "URL": "https://api.anyblock.tools/market/AMPL_USD_via_ALL/daily-volume?roundDay=false&debug=false&access_token=$ANYBLOCK_KEY&start=$NOW&end=$EOD",
                "param": "$.overallVWAP"
            }
        ]
    },
    ...
    "AMPL/ETH": [
        "interval": "1m",
          "endpoints": [
              {
                "URL": "Mainnet:0xc5be99a02c6857f9eac67bbce58df5572498f40c,Rinkeby:0x7e62a502232f1feB77Adf8b8ca023cc9fB133418",
                "type": "ethereum",
                "parser": "Uniswap"
              }
          ]
    ],
    ...
}
```

Any env variable is substituted in the API URL. The example above uses an env variable to provide the api key required to access the endpoint.

There are some special env variables that can be added to the url and are dinamycly expanded before each request - `$NOW`, `$EOD`. This is required for some APIs that require specifying start and end timestamp like in the example above.


## Index Tracker types

### HTTP trackers

If not set the default type of an index tracker is `http` type.

### On-chain trackers

If the index tracker type was set to `ethereum` then it's an on-chain tracker that fetches data using on-chain calls on an Ethereum blockchain network.

Currently supported on-chain parsers are `Uniswap` and `Balancer` parsers.


## Parsers

### Jsonpath parser

When not set this is the default parser. It parses data from the JSON payload using the `param` as an instruction on how to parse the output.
[More info](http://goessner.net/articles/JsonPath/).

### Balancer parser

`Balancer` is a parser that fetches tracker info from a [Balancer pool](https://docs.balancer.finance/getting-started/faq#balancer-pools). Balancer pools are liquidity pools for pair of ERC20 tokens. a Balancer pool could exist on both Ethereum mainnet and testnets. for Balancer smart contract addresses see [here](https://docs.balancer.finance/smart-contracts/addresses).

There is an `on-chain registry` smart contract for Balancer that could be queried to get the best pools for an input pair of ERC20 tokens:

```javascript
getBestPools(address fromToken, address destToken)
```

This method will retrieve an array of pool addresses for a token pair that is ordered by liquidity value.

Sometimes there is no deployed testnet Balancer pool to use in the On-chain index tracker. Here are some steps to deploy a Balancer pool on the Rinkeby testnet and add some liquidity to it based on [this](https://docs.balancer.finance/guides/testing-on-kovan) Balancer doc.

**1- Deploy ERC20 tokens as required**

[Here](https://github.com/hhio618/simple-erc20-example) is a simple JavaScript project that could be used to deploy an ERC20 token on the Rinkeby testnet.

**2- Install and configure the seth client**

Install and configure `seth` client using [this Github repository](https://github.com/dapphub/dapptools#installation) guides.

**3- Pool creation**

Set address variables:

```javascript
export BFACTORY=0x9C84391B443ea3a48788079a5f98e2EaD55c9309
export TOKEN1=<token#1-address-from-the-first-step>
export TOKEN2=<token#2-address-from-the-first-step>
```

Create a new pool:

```text
seth send --gas 5000000 $BFACTORY "newBPool()"
```

Set `BPOOL` variable:

```text
export BPOOL=0x... (address returned from the previous command)
```

Approve ERC20 tokens on the BPool smart contract:

```text
amount=$(seth --to-uint256 $(seth --to-wei 1000000 ether))
seth send $TOKEN1 "approve(address,uint256)" $BPOOL $amount
seth send $TOKEN2 "approve(address,uint256)" $BPOOL $amount
```

Wait for confirmations. after that bind tokens using the `bind` method. for example:

```text
# Bind 1000000 TOKEN1 with a denormalized weight of 5
amount=$(seth --to-uint256 $(seth --to-wei 1000000 ether))
weight=$(seth --to-uint256 $(seth --to-wei 5 ether))
seth send $BPOOL "bind(address, uint256, uint256)" $TOKEN1 $amount $weight
```

Let's confirm that all the tokens were added by using some query method:

```text
seth call $BPOOL "getNumTokens()"
```

Set a swap fee and finalize:

```text
fee=$(seth --to-uint256 $(seth --to-wei 0.003 ether))
seth send $BPOOL "setSwapFee(uint256)" $fee
seth send $BPOOL "finalize()"
```

All done. Let's confirm that we received BPTs by calling `balanceOf` directly on the pool address \(since the pools themselves are ERC20 tokens\):

```text
seth --from-wei $(seth --to-dec $(seth call $BPOOL "balanceOf(address)" $ETH_FROM))
```

### Uniswap parser

`Uniswap` is a parser that fetches tracker info from a [UniswapV2 pair](https://uniswap.org/docs/v2/smart-contracts/pair/). the easiest way to add UniswapV2 testnet pair is to call the [addLiquidity](https://uniswap.org/docs/v2/smart-contracts/router02/#addliquidity) contract method on Uniswap RouterV2 smart contract on different Ethereum networks. see [addresses](https://uniswap.org/docs/v2/smart-contracts/router02/#addresshttps://uniswap.org/docs/v2/smart-contracts/router02/#address). the method is as follow:

```javascript
function addLiquidity(
  address tokenA,
  address tokenB,
  uint amountADesired,
  uint amountBDesired,
  uint amountAMin,
  uint amountBMin,
  address to,
  uint deadline
) external returns (uint amountA, uint amountB, uint liquidity);
```

This required to deploy some ERC20 token beforehand and will create a Uniswap V2 pair if already not exists for the provided pair. there is a factory method that could be used to get the pair address [here](https://uniswap.org/docs/v2/smart-contracts/factory/#getpair).
