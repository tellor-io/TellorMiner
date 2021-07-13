---
description: Here are the nuts and bolts for using the CLI
---

# Setup and usage

## Get the CLI

The CLI support only linux and is provided as a pre-built binary with every release and also as a docker image.

[Github releases](https://github.com/tellor-io/telliot/releases)

[https://hub.docker.com/u/tellor](https://hub.docker.com/u/tellor)

## Config files.
 - `.env` - keeps private information(private keys, api keys etc.). Most commands require some secrets and these are kept in this file as a precaution against accidental exposure. For a working setup it is required to at least add one private key in your `"ETH_PRIVATE_KEYS"` environment variable. Multiple private keys are supported separated by `,`.
 - `index.json` - all api endpoint for data providers. The cli uses these provider endpoints to gather data which is then used to submit to the onchain oracle.
 - `manualdata.json` - for providing data manually. There is currently one data point which must be manually created. The rolling 3 month average of the US PCE . It is updated monthly. _Make sure to keep this file up to date._
 For testing purposes, or if you want to hardcode in a specific value, you can use the file to add manual data for a given requestID. Add the request ID, a given value \(with granularity\), and a date on which the manual data expires.
The following example shows request ID 4, inputting a value of 9000 with 6 digits granularity. Note the date is a unix timestamp.
```bash
"4":{
    "VALUE":9000.123456,
    "DATE":1596153600
}
```
 - `config.json` - optional config file to override any of the defaults. See the [configuration page](configuration.md) for full reference.


> by default the cli looks for these in the `./configs` folder relative to the cli folder.

### Here is a quick reference how to run the cli with the default configs.

```
mkdir ./configs
cd ./configs
wget https://raw.githubusercontent.com/tellor-io/telliot/master/configs/index.json
wget https://raw.githubusercontent.com/tellor-io/telliot/master/configs/manualData.json
wget https://raw.githubusercontent.com/tellor-io/telliot/master/configs/.env.example
mv .env.example .env
cd ../
wget https://github.com/tellor-io/telliot/releases/latest/download/telliot
chmod +x telliot
```

## Deposit or withdraw a stake

As of now, mining requires you to deposit 500 TRB to be allowed to submit values to the oracle and earn rewards. This is a security deposit. If you are a malicious actor \(aka submit a bad value\), the community can vote to slash your 500 tokens.
Your stake is locked for a minimum of 7 days after you run the command to request withdrawal.

Run the following command to deposit your stake:

```bash
./telliot stake deposit
```

To unstake your tokens, you need to request a withdraw:

```bash
./telliot stake request
```

One week after the request, the tokens are free to move at your discretion after running the command:

```bash
./telliot stake withdraw
```

## Start mining.
{% hint style="info" %}
The same instance can be used with multiple private keys in the `.env` file separated by a comma.
{% endhint %}

```bash
./telliot mine
```

Telliot supports submiting data to different contracts and the config folder contains examples for that.
For example:
```bash
./telliot mine --config=configs/configTellorMesosphere.json
```

## DataServer - a shared data API feeds.

{% hint style="info" %}
Advanced usage! If you are setting up a Tellor miner for the first time, it might be a good idea to skip this section and come back after you're up and running with one miner. See the [configuration page](configuration.md) for the required configs.
{% endhint %}

Some oracle feeds require 24h avarages and for these enough historical data is needed. Running a dataserver is the solution to always have enough historical data to generate these averages.

The network topology of this setup looks like the diagram below.
One ore more miners are connected to the same data server for fetching current or historical data to submit to the oracle.
The data server pulls data from the API providers, the 5 staked miners pull data from the data server and submit on-chain to the Tellor Core smart contracts.

```bash
                            /(0xE037)\
                Miner      | (0xcdd8) |
Tellor     <-> (multiple   | (0xb9dD) | <-> Data Server <-> Data APIs
(on chain)      keys)      | (0x2305) |
                            \(0x3233)/
```


## Run with Docker - [https://hub.docker.com/u/tellor](https://hub.docker.com/u/tellor)

```bash
cp configs/.env.example configs/.env # Edit the file after the copy.
docker run -v $(pwd)/configs:/configs tellor/telliot:master mine
```

## Run on Kubernetes with Helm Chart

A Helm chart for installing telliot on Kubernetes

[A guide for installing helm can be found here](https://helm.sh/docs/intro/install/)

## telliot Configuration

Include telliot configuration files in the files directory of this chart.

These files should be:

- .env
- index.json
- manualData.json

Run the following to use the default configuration files:

```bash
cp configs/index.json configs/manualData.json configs/helm/files/
cp configs/.env.example configs/helm/files/.env # you will need to edit this file with your own secrets after copying
```

Optionally you can also include config.json if you would like to override any default config values.

## Usage

After you have moved your configuration files to config/helm/files, you can install this chart using the following command:

```bash
export INSTANCE_NAME=lat
helm install $INSTANCE_NAME configs/helm/ \
    --namespace tellor --create-namespace 
```

INSTANCE_NAME being a string you would use to denote this instance of telliot.

Keep in mind this command is using all default values.

## Values

The default helm values will install a mining and dataServer instance of telliot. Grafana, Alertmanager, and Prometheus are disabled by default.

To override these values during installation include `--set $key=$value` in the helm upgrade command.

For example, to run a dataserver instance of telliot using a custom image with 5Gi of storage and the monitoring/alerting stack:

```bash
export INSTANCE_NAME=lat
helm install $INSTANCE_NAME configs/helm/ \
    --namespace tellor --create-namespace \
    --set "telliot.container.image=mytelliot:01" \
    --set "telliot.storage=5Gi" \
    --set "telliot.modes={dataServer}" \
    --set "grafana.enabled=true" \
    --set "alertmanager.enabled=true"
    --set "prometheus.enabled=true"
```

If I instead only wanted to run a mining instance of telliot:

```bash
export INSTANCE_NAME=lat
helm install $INSTANCE_NAME configs/helm/ \
    --namespace tellor --create-namespace \
    --set "telliot.modes={mine}" \
```

You can also modify these values directly in values.yaml before installation.

| Key                              | Type   | Default                               | Description                                                                                  |
| -------------------------------- | ------ | ------------------------------------- | -------------------------------------------------------------------------------------------- |
| alertmanager.bot.container.image | string | `"metalmatze/alertmanager-bot:0.4.3"` | Docker image for alertmanager bot                                                            |
| alertmanager.bot.container.port  | int    | `8080`                                |                                                                                              |
| alertmanager.bot.enabled         | bool   | `false`                               | Whether to enable alertmanager bot                                                           |
| alertmanager.bot.service.port    | int    | `8080`                                |                                                                                              |
| alertmanager.bot.storage         | string | `"1Gi"`                               |                                                                                              |
| alertmanager.bot.telegram.admin  | string | `nil`                                 | Telegram admin username                                                                      |
| alertmanager.bot.telegram.token  | string | `nil`                                 | Telegram token                                                                               |
| alertmanager.container.image     | string | `"prom/alertmanager:v0.19.0"`         | Docker image for alertmanager                                                                |
| alertmanager.container.port      | int    | `9093`                                |                                                                                              |
| alertmanager.enabled             | bool   | `false`                               |                                                                                              |
| alertmanager.service.port        | int    | `9093`                                |                                                                                              |
| grafana.container.image          | string | `"grafana/grafana:7.3.6"`             | Docker image for grafana                                                                     |
| grafana.container.port           | int    | `3000`                                |                                                                                              |
| grafana.enabled                  | bool   | `false`                               |                                                                                              |
| grafana.ingress.class            | string | `"nginx"`                             | Ingress class to use for grafana                                                             |
| grafana.ingress.hostname         | string | `"monitor.tellor.io"`                 | Hostname to use for accessing grafana                                                        |
| grafana.ingress.path             | string | `"/"`                                 | Subpath to access grafana                                                                    |
| grafana.ingress.tls.enabled      | bool   | `false`                               | Enable/Disable TLS for grafana                                                               |
| grafana.ingress.tls.secret       | string | `"grafana-tls-secret"`                | Name of TLS secret to use for grafana                                                        |
| grafana.persist                  | bool   | `true`                                | Enable persistance for grafana configuration                                                 |
| grafana.service.port             | int    | `80`                                  |                                                                                              |
| grafana.storage                  | string | `"5Gi"`                               | Grafana persistent storage size                                                              |
| prometheus.container.image       | string | `"prom/prometheus:v2.24.0"`           | Docker image for prometheus                                                                  |
| prometheus.container.port        | int    | `9090`                                |                                                                                              |
| prometheus.enabled               | bool   | `false`                               |                                                                                              |
| prometheus.persist               | bool   | `true`                                | Enable persistance for prometheus configuration                                              |
| prometheus.service.port          | int    | `9090`                                |                                                                                              |
| prometheus.storage               | int    | `50`                                  | Prometheus storage size in GB                                                                |
| telliot.uniqueConfiguration      | bool   | `false`                               | Whether to utilize unique configurations for dataServer and mine. <sup>[1](#footnote1)</sup> |
| telliot.container.image          | string | `"tellor/telliot:master"`             | Docker image for telliot                                                                     |
| telliot.container.port           | int    | `9090`                                |                                                                                              |
| telliot.modes                    | string | `"{dataServer,mine}"`                 | Array of commands to spawn separate instances of telliot instances with                      |
| telliot.service.port             | int    | `9090`                                |                                                                                              |
| telliot.storage                  | string | `"2Gi"`                               | telliot persistent storage size                                                              |

<a name="footnote1">1</a>: If this value is set to true, you will need to provide configurations for both modes under config/helm/files/mine and config/helm/files/dataServer.

## Upgrade

To upgrade your instance simply run

```bash
helm upgrade $INSTANCE_NAME configs/helm/ --namespace tellor --set $key=$value
```

With `$key=$value` being the desired value changes you would like to make.

If you would like to make any config changes, move the updated configuration file to configs/helm/files/ and run

```bash
helm upgrade $INSTANCE_NAME configs/helm/ --namespace tellor
```

## Removal

Uninstalling an instance of telliot using helm is as simple as

```bash
helm uninstall $INSTANCE_NAME --namespace tellor
```

Where instance name was the name for the release you specified during installation. You can find this value by running

```bash
helm list --namespace tellor
```
