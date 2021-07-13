---
description: Telliot tweaks and settings to keep your rig running smoothly.
---

# Configuration reference

## CLI reference

Telliot commands and config file options are as the following:

#### Required Flags <a id="docs-internal-guid-d1a57725-7fff-a753-9236-759dd3f42eed"></a>

* `--config` \(path to your config file.\)

#### Telliot Commands

* `accounts`

```
Usage: telliot accounts

Show accounts

Flags:
  -h, --help                  Show context-sensitive help.

      --config=CONFIG-PATH    path to config file

```

* `approve`

```
Usage: telliot approve --from=STRING --to=STRING <amount>

Approve tokens

Arguments:
  <amount>

Flags:
  -h, --help                  Show context-sensitive help.

      --config=CONFIG-PATH    path to config file
      --gas-price=INT         gas price to use when running the command
      --from=STRING
      --to=STRING

```

* `balance`

```
Usage: telliot balance [<address>]

Check the balance of an address

Arguments:
  [<address>]

Flags:
  -h, --help                  Show context-sensitive help.

      --config=CONFIG-PATH    path to config file

```

* `dataserver`

```
Usage: telliot dataserver

launch only a dataserver instance

Flags:
  -h, --help                  Show context-sensitive help.

      --config=CONFIG-PATH    path to config file

```

* `dispute`

```
Usage: telliot dispute <command>

Perform commands related to disputes

Flags:
  -h, --help    Show context-sensitive help.

Commands:
  dispute new <addr> <request-id> <timestamp> <miner-index>
    start a new dispute

  dispute vote <addr> <dispute-id> <support>
    vote on a open dispute

  dispute list <addr>
    list open disputes

  dispute tally <dispute-id>
    tally votes for a dispute ID

```

* `dispute list`

```
Usage: telliot dispute list <addr>

list open disputes

Arguments:
  <addr>

Flags:
  -h, --help                  Show context-sensitive help.

      --config=CONFIG-PATH    path to config file

```

* `dispute new`

```
Usage: telliot dispute new <addr> <request-id> <timestamp> <miner-index>

start a new dispute

Arguments:
  <addr>
  <request-id>     the request id to dispute it
  <timestamp>      the submitted timestamp to dispute
  <miner-index>    the miner index to dispute

Flags:
  -h, --help                  Show context-sensitive help.

      --config=CONFIG-PATH    path to config file
      --gas-price=INT         gas price to use when running the command

```

* `dispute tally`

```
Usage: telliot dispute tally <dispute-id>

tally votes for a dispute ID

Arguments:
  <dispute-id>    the dispute id

Flags:
  -h, --help                  Show context-sensitive help.

      --config=CONFIG-PATH    path to config file
      --gas-price=INT         gas price to use when running the command

```

* `dispute vote`

```
Usage: telliot dispute vote <addr> <dispute-id> <support>

vote on a open dispute

Arguments:
  <addr>
  <dispute-id>    the dispute id
  <support>       true or false

Flags:
  -h, --help                  Show context-sensitive help.

      --config=CONFIG-PATH    path to config file
      --gas-price=INT         gas price to use when running the command

```

* `mine`

```
Usage: telliot mine

Submit data to oracle contracts

Flags:
  -h, --help                  Show context-sensitive help.

      --config=CONFIG-PATH    path to config file

```

* `stake`

```
Usage: telliot stake <command>

Perform one of the stake operations

Flags:
  -h, --help    Show context-sensitive help.

Commands:
  stake deposit <addr>
    deposit a stake

  stake request <addr>
    request to withdraw stake

  stake withdraw <addr>
    withdraw stake

  stake status <addr>
    show stake status

```

* `stake deposit`

```
Usage: telliot stake deposit <addr>

deposit a stake

Arguments:
  <addr>

Flags:
  -h, --help                  Show context-sensitive help.

      --config=CONFIG-PATH    path to config file
      --gas-price=INT         gas price to use when running the command

```

* `stake request`

```
Usage: telliot stake request <addr>

request to withdraw stake

Arguments:
  <addr>

Flags:
  -h, --help                  Show context-sensitive help.

      --config=CONFIG-PATH    path to config file
      --gas-price=INT         gas price to use when running the command

```

* `stake status`

```
Usage: telliot stake status <addr>

show stake status

Arguments:
  <addr>

Flags:
  -h, --help                  Show context-sensitive help.

      --config=CONFIG-PATH    path to config file

```

* `stake withdraw`

```
Usage: telliot stake withdraw <addr>

withdraw stake

Arguments:
  <addr>

Flags:
  -h, --help                  Show context-sensitive help.

      --config=CONFIG-PATH    path to config file
      --gas-price=INT         gas price to use when running the command

```

* `transfer`

```
Usage: telliot transfer --from=STRING --to=STRING <amount>

Transfer tokens

Arguments:
  <amount>

Flags:
  -h, --help                  Show context-sensitive help.

      --config=CONFIG-PATH    path to config file
      --gas-price=INT         gas price to use when running the command
      --from=STRING
      --to=STRING

```

* `version`

```
Usage: telliot version

Show the CLI version information

Flags:
  -h, --help    Show context-sensitive help.

```

#### .env file options:


* `ETH_PRIVATE_KEYS` \(required\) - list of private keys separated by `,`

* `NODE_URL` \(required\) - websocket node URL \(e.g [wss://mainnet.infura.io/bbbb](wss://mainnet.infura.io/bbbb) or [wss://localhost:8546](ws://localhost:8546) if own node\)


#### Config file options:
```json
{
	"Aggregator": {
		"LogLevel": "Required:false, Default:info",
		"ManualDataFile": "Required:false, Default:configs/manualData.json"
	},
	"Db": {
		"LogLevel": "Required:false, Default:info",
		"Path": "Required:false, Default:db",
		"RemoteHost": "Required:false, Default:",
		"RemotePort": "Required:false, Default:0",
		"RemoteTimeout": {
			"Duration": "Required:false, Default:5s"
		}
	},
	"DisputeTracker": {
		"LogLevel": "Required:false, Default:info"
	},
	"GasStation": {
		"TimeWait": {
			"Duration": "Required:false, Default:1m0s"
		}
	},
	"IndexTracker": {
		"IndexFile": "Required:false, Default:configs/index.json",
		"Interval": {
			"Duration": "Required:false, Default:30s"
		},
		"LogLevel": "Required:false, Default:info"
	},
	"Mining": {
		"Heartbeat": "Required:false, Default:1m0s",
		"LogLevel": "Required:false, Default:info"
	},
	"ProfitTracker": {
		"LogLevel": "Required:false, Default:info"
	},
	"PsrTellor": {
		"MinConfidence": "Required:false, Default:70"
	},
	"PsrTellorMesosphere": {
		"MinConfidence": "Required:false, Default:0"
	},
	"RewardTracker": {
		"LogLevel": "Required:false, Default:info"
	},
	"SubmitterTellor": {
		"Enabled": "Required:false, Default:true",
		"LogLevel": "Required:false, Default:info",
		"MinSubmitPeriod": {
			"Duration": "Required:false, Default:15m1s"
		},
		"ProfitThreshold": "Required:false, Default:0, Description:Minimum percent of profit when submitting a solution. For example if the tx cost is 0.01 ETH and current reward is 0.02 ETH a ProfitThreshold of 200% or more will wait until the reward is increased or the gas cost is lowered a ProfitThreshold of 199% or less will submit."
	},
	"SubmitterTellorMesosphere": {
		"Enabled": "Required:false, Default:false",
		"LogLevel": "Required:false, Default:info",
		"MinSubmitPeriod": {
			"Duration": "Required:false, Default:15s"
		},
		"MinSubmitPriceChange": "Required:false, Default:0.05, Description: Submit only if that price changed at least that much percent."
	},
	"Tasker": {
		"LogLevel": "Required:false, Default:info"
	},
	"Transactor": {
		"GasMax": "Required:false, Default:10",
		"GasMultiplier": "Required:false, Default:1",
		"LogLevel": "Required:false, Default:info"
	},
	"Web": {
		"ListenHost": "Required:false, Default:",
		"ListenPort": "Required:false, Default:9090",
		"LogLevel": "Required:false, Default:info",
		"ReadTimeout": {
			"Duration": "Required:false, Default:0s"
		}
	},
	"envFile": "Required:false, Default:configs/.env"
}
```
Here are the config defaults in json format:
```json
{
	"Aggregator": {
		"LogLevel": "info",
		"ManualDataFile": "configs/manualData.json"
	},
	"Db": {
		"LogLevel": "info",
		"Path": "db",
		"RemoteHost": "",
		"RemotePort": 0,
		"RemoteTimeout": "5s"
	},
	"DisputeTracker": {
		"LogLevel": "info"
	},
	"GasStation": {
		"TimeWait": "1m0s"
	},
	"IndexTracker": {
		"IndexFile": "configs/index.json",
		"Interval": "30s",
		"LogLevel": "info"
	},
	"Mining": {
		"Heartbeat": 60000000000,
		"LogLevel": "info"
	},
	"ProfitTracker": {
		"LogLevel": "info"
	},
	"PsrTellor": {
		"MinConfidence": 70
	},
	"PsrTellorMesosphere": {
		"MinConfidence": 0
	},
	"RewardTracker": {
		"LogLevel": "info"
	},
	"SubmitterTellor": {
		"Enabled": true,
		"LogLevel": "info",
		"MinSubmitPeriod": "15m1s",
		"ProfitThreshold": 0
	},
	"SubmitterTellorMesosphere": {
		"Enabled": false,
		"LogLevel": "info",
		"MinSubmitPeriod": "15s",
		"MinSubmitPriceChange": 0.05
	},
	"Tasker": {
		"LogLevel": "info"
	},
	"Transactor": {
		"GasMax": 10,
		"GasMultiplier": 1,
		"LogLevel": "info"
	},
	"Web": {
		"ListenHost": "",
		"ListenPort": 9090,
		"LogLevel": "info",
		"ReadTimeout": "0s"
	},
	"envFile": "configs/.env"
}
```
### Log levels
Note the default level is "INFO", so to turn down the number of logs, enter "WARN" or "ERROR".

DEBUG - logs everything in INFO and additional developer logs

INFO - logs most information about the mining operation

WARN - logs all warnings and errors

ERROR - logs only serious errors
