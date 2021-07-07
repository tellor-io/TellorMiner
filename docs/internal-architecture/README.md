---
description: Internal architecture of the project.
---

# Internal architecture

## Tasker

Monitors the oracle contract for new data requests(aka "oracle blocks").
As soon as a new challenge is emitted it is sent to the miner module to find a solution.

## Miner

It simply solves the PoW challenge which is required by the oracle contract when submitting values.

## Submitter

Submits the currently requested data IDs to the oracle contract.
It supports submitting to different oracle contracts(see the setup page for more details).
It makes all the necessary checks to prepare the data accordingly to avoid failed transactions.
The data is taken from the PSR module.

## PSR

It defines all DATA ids for the oracle contract.
For example DATA is 10 in the tellor oracle contract is 24h VWAP of the AMPL/USD price.
It uses the aggregator to get the required aggregated data.

## Aggregator

Aggregates data to expose median, mean, TWAP, VWAP etc.
It uses the data from the local/remote db.
The db is populated by the index tracker.

## Trackers

A tracker is module that runs at a given interval and collects and records data.
The most important tracked it the [Index Tracker](index-tracker.md). It gets and parses data from an HTTP API or a Blockchain smart contract for later usage or aggregation.
Another tracker is the profit tracker. It monitors and records all profit and cost for a one or multiple addresses.

## API

The cli exposes an api to query all collected data from the trackers.
The api is an exact copy of the [Prometheus API](https://prometheus.io/docs/prometheus/latest/querying/api/) which uses the [promql query language](https://prometheus.io/docs/prometheus/latest/querying/basics).


