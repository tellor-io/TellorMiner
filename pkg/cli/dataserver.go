// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package cli

import (
	"context"
	"os"
	"syscall"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/oklog/run"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/tellor-io/telliot/pkg/aggregator"
	"github.com/tellor-io/telliot/pkg/config"
	"github.com/tellor-io/telliot/pkg/contracts"
	"github.com/tellor-io/telliot/pkg/ethereum"
	"github.com/tellor-io/telliot/pkg/logging"
	psrTellor "github.com/tellor-io/telliot/pkg/psr/tellor"
	"github.com/tellor-io/telliot/pkg/tracker/dispute"
	"github.com/tellor-io/telliot/pkg/tracker/index"
	"github.com/tellor-io/telliot/pkg/tracker/reward"
	"github.com/tellor-io/telliot/pkg/web"
)

type dataserverCmd struct {
	Config configPath `type:"existingfile" help:"path to config file"`
}

func (self dataserverCmd) Run() error {
	logger := logging.NewLogger()

	cfg, err := config.ParseConfig(logger, string(self.Config))
	if err != nil {
		return errors.Wrap(err, "creating config")
	}

	// Defining a global context for starting and stopping of components.
	ctx := context.Background()

	// We define our run groups here.
	var g run.Group
	// Run groups.
	{
		// Handle interupts.
		g.Add(run.SignalHandler(context.Background(), syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM))

		// Open the TSDB database.
		tsdbOptions := tsdb.DefaultOptions()
		// 5 days are enough as the aggregator needs data only 24 hours in the past.
		tsdbOptions.RetentionDuration = int64(5 * 24 * time.Hour / time.Millisecond)
		if err := os.MkdirAll(cfg.Db.Path, 0777); err != nil {
			return errors.Wrap(err, "creating tsdb DB folder")
		}
		tsDB, err := tsdb.Open(cfg.Db.Path, nil, nil, tsdbOptions)
		if err != nil {
			return errors.Wrap(err, "creating tsdb DB")
		}
		level.Info(logger).Log("msg", "opened local db", "path", cfg.Db.Path)

		defer func() {
			if err := tsDB.Close(); err != nil {
				level.Error(logger).Log("msg", "closing the tsdb", "err", err)
			}
		}()

		// Index tracker.

		// The client is needed when the api requests data from the blockchain.
		// TODO create an eth client only if the api config file has eth address.
		client, err := ethereum.NewClient(ctx, logger)
		if err != nil {
			return errors.Wrap(err, "creating ethereum client")
		}

		index, err := index.New(logger, ctx, cfg.IndexTracker, tsDB, client)
		if err != nil {
			return errors.Wrap(err, "creating index tracker")
		}

		g.Add(func() error {
			err := index.Run()
			level.Info(logger).Log("msg", "index shutdown complete")
			return err
		}, func(error) {
			index.Stop()
		})

		// Aggregator.
		aggregator, err := aggregator.New(logger, ctx, cfg.Aggregator, tsDB)
		if err != nil {
			return errors.Wrap(err, "creating aggregator")
		}

		contractTellor, err := contracts.NewITellor(client)
		if err != nil {
			return errors.Wrap(err, "create tellor contract instance")
		}

		// Reward tracker.
		accounts, err := ethereum.GetAccounts()
		if err != nil {
			return errors.Wrap(err, "getting accounts")
		}
		rewardTracker, err := reward.NewRewardTracker(logger, ctx, cfg.RewardTracker, tsDB, client, contractTellor, accounts[0].Address, aggregator)
		if err != nil {
			return errors.Wrap(err, "creating reward tracker")
		}
		g.Add(func() error {
			err := rewardTracker.Start()
			level.Info(logger).Log("msg", "reward tracker shutdown complete")
			return err
		}, func(error) {
			rewardTracker.Stop()
		})

		disputeTracker, err := dispute.New(
			logger,
			ctx,
			cfg.DisputeTracker,
			tsDB,
			client,
			contractTellor,
			psrTellor.New(logger, cfg.PsrTellor, aggregator),
		)
		if err != nil {
			return errors.Wrap(err, "creating profit tracker")
		}
		g.Add(func() error {
			disputeTracker.Start()
			level.Info(logger).Log("msg", "dispute tracker shutdown complete")
			return nil
		}, func(error) {
			disputeTracker.Stop()
		})

		// Web/Api server.
		{
			srv, err := web.New(logger, ctx, tsDB, cfg.Web)
			if err != nil {
				return errors.Wrap(err, "create web server")
			}
			g.Add(func() error {
				err := srv.Start()
				level.Info(logger).Log("msg", "web server shutdown complete")
				return err
			}, func(error) {
				srv.Stop()
			})
		}
	}

	if err := g.Run(); err != nil {
		level.Error(logger).Log("msg", "main exited with error", "err", err)
		return err
	}

	level.Info(logger).Log("msg", "main shutdown complete")
	return nil
}
