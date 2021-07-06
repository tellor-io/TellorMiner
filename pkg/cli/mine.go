// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package cli

import (
	"context"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/oklog/run"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/tellor-io/telliot/pkg/aggregator"
	"github.com/tellor-io/telliot/pkg/config"
	"github.com/tellor-io/telliot/pkg/contracts"
	"github.com/tellor-io/telliot/pkg/db"
	"github.com/tellor-io/telliot/pkg/ethereum"
	"github.com/tellor-io/telliot/pkg/gasPrice/gasStation"
	"github.com/tellor-io/telliot/pkg/logging"
	"github.com/tellor-io/telliot/pkg/mining"
	psrTellor "github.com/tellor-io/telliot/pkg/psr/tellor"
	psrTellorMesosphere "github.com/tellor-io/telliot/pkg/psr/tellorMesosphere"
	"github.com/tellor-io/telliot/pkg/submitter/tellor"
	"github.com/tellor-io/telliot/pkg/submitter/tellorMesosphere"
	"github.com/tellor-io/telliot/pkg/tasker"
	"github.com/tellor-io/telliot/pkg/tracker/dispute"
	"github.com/tellor-io/telliot/pkg/tracker/index"
	"github.com/tellor-io/telliot/pkg/tracker/profit"
	"github.com/tellor-io/telliot/pkg/tracker/reward"
	"github.com/tellor-io/telliot/pkg/transactor"
	"github.com/tellor-io/telliot/pkg/web"
)

type mineCmd struct {
	Config configPath `type:"existingfile" help:"path to config file"`
}

func (self mineCmd) Run() error {
	logger := logging.NewLogger()

	cfg, err := config.ParseConfig(logger, string(self.Config))
	if err != nil {
		return errors.Wrap(err, "creating config")
	}

	// Defining a global context for starting and stopping of components.
	ctx := context.Background()

	client, err := ethereum.NewClient(ctx, logger)
	if err != nil {
		return errors.Wrap(err, "creating ethereum client")
	}

	accounts, err := ethereum.GetAccounts()
	if err != nil {
		return errors.Wrap(err, "getting accounts")
	}

	// We define our run groups here.
	var g run.Group
	// Run groups.
	{
		// Handle interupts.
		g.Add(run.SignalHandler(context.Background(), syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM))

		// Open a local or remote instance of the TSDB database.
		var tsDB storage.SampleAndChunkQueryable
		if cfg.Db.RemoteHost != "" {
			tsDB, err = db.NewRemoteDB(cfg.Db)
			if err != nil {
				return errors.Wrap(err, "opening remote tsdb DB")
			}
			level.Info(logger).Log("msg", "connected to remote db", "host", cfg.Db.RemoteHost, "port", cfg.Db.RemotePort)
		} else {
			// Open the TSDB database.
			tsdbOptions := tsdb.DefaultOptions()
			// 5 days are enough as the aggregator needs data only 24 hours in the past.
			tsdbOptions.RetentionDuration = int64(5 * 24 * time.Hour)
			_tsDB, err := tsdb.Open(cfg.Db.Path, nil, nil, tsdbOptions)
			if err != nil {
				return errors.Wrap(err, "opening local tsdb DB")
			}
			defer func() {
				if err := _tsDB.Close(); err != nil {
					level.Error(logger).Log("msg", "closing the tsdb", "err", err)
				}
			}()
			tsDB = _tsDB
			level.Info(logger).Log("msg", "opened local db", "path", cfg.Db.Path)
			level.Warn(logger).Log("msg", "FOR NEW DB INSTANCES IT IS NORMAL TO SEE SOME QUERY ERRORS AS THE DATABASE IS NOT YET POPULATED WITH VALUES")
		}

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

		// Aggregator.
		aggregator, err := aggregator.New(logger, ctx, cfg.Aggregator, tsDB)
		if err != nil {
			return errors.Wrap(err, "creating aggregator")
		}

		// Index tracker.
		// Run only when not using remote DB as it needs to write to the local db.
		if cfg.Db.RemoteHost == "" {
			_tsDB, ok := tsDB.(*tsdb.DB)
			if !ok {
				return errors.New("tsdb is not a writable DB instance")
			}

			// Index Tracker.
			index, err := index.New(logger, ctx, cfg.IndexTracker, _tsDB, client)
			if err != nil {
				return errors.Wrapf(err, "creating index tracker")
			}

			g.Add(func() error {
				err := index.Run()
				level.Info(logger).Log("msg", "index shutdown complete")
				return err
			}, func(error) {
				index.Stop()
			})

			_netID, err := client.NetworkID(ctx)
			if err != nil {
				return errors.Wrap(err, "getting network ID")
			}
			netID := _netID.Int64()

			// Dispute tracker.
			// Run it only when not connected to a remote DB.
			// A remote DB already runs a dispute tracker so no need to run another one.
			// Also run and only for mainnet or rinkeby as the tellor oracle exists only on those networks.
			if netID == 1 || netID == 4 {
				contractTellor, err := contracts.NewITellor(client)
				if err != nil {
					return errors.Wrap(err, "create tellor contract instance")
				}

				// Reward tracker.
				rewardTracker, err := reward.NewRewardTracker(logger, ctx, cfg.RewardTracker, _tsDB, client, contractTellor, accounts[0].Address, aggregator)
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
					_tsDB,
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
			}

		}

		gasPriceQuerier, err := gasStation.New(logger, cfg.GasStation, client)
		if err != nil {
			return errors.Wrap(err, "creating gas price tracker")
		}

		if cfg.SubmitterTellor.Enabled {
			// Profit tracker.
			var accountAddrs []common.Address
			for _, acc := range accounts {
				accountAddrs = append(accountAddrs, acc.Address)
			}

			contractTellor, err := contracts.NewITellor(client)
			if err != nil {
				return errors.Wrap(err, "create tellor contract instance")
			}

			profitTracker, err := profit.NewProfitTracker(logger, ctx, cfg.ProfitTracker, client, contractTellor, accountAddrs)
			if err != nil {
				return errors.Wrap(err, "creating profit tracker")
			}
			g.Add(func() error {
				err := profitTracker.Start()
				level.Info(logger).Log("msg", "profit tracker shutdown complete")
				return err
			}, func(error) {
				profitTracker.Stop()
			})

			// Event tasker.
			tasker, taskerChs, err := tasker.New(ctx, logger, cfg.Tasker, client, contractTellor, accounts)
			if err != nil {
				return errors.Wrap(err, "creating tasker")
			}
			g.Add(func() error {
				err := tasker.Start()
				level.Info(logger).Log("msg", "tasker shutdown complete")
				return err
			}, func(error) {
				tasker.Stop()
			})

			// Create a submitter for each account.
			for _, account := range accounts {
				loggerWithAddr := log.With(logger, "addr", account.Address.String()[:6])

				transactor, err := transactor.New(loggerWithAddr, cfg.Transactor, gasPriceQuerier, client, account)
				if err != nil {
					return errors.Wrap(err, "creating transactor")
				}

				psr := psrTellor.New(loggerWithAddr, cfg.PsrTellor, aggregator)

				_tsDB, ok := tsDB.(*tsdb.DB)
				if !ok {
					return errors.New("tsdb is not a writable DB instance")
				}
				// Reward tracker instance for the query.
				rewardQuerier, err := reward.NewRewardQuerier(logger, ctx, cfg.RewardTracker, _tsDB, client, contractTellor, accounts[0].Address, aggregator)
				if err != nil {
					return errors.Wrap(err, "creating reward tracker")
				}
				// Get a channel on which it listens for new data to submit.
				submitter, submitterCh, err := tellor.New(
					ctx,
					loggerWithAddr,
					cfg.SubmitterTellor,
					client,
					contractTellor,
					account,
					rewardQuerier,
					transactor,
					gasPriceQuerier,
					psr,
				)
				if err != nil {
					return errors.Wrap(err, "creating tellor submitter")
				}
				g.Add(func() error {
					err := submitter.Start()
					level.Info(loggerWithAddr).Log("msg", "tellor submitter shutdown complete")
					return err
				}, func(error) {
					submitter.Stop()
				})

				// Will be used to cancel pending submissions.
				tasker.AddSubmitCanceler(submitter)

				// The Miner component.
				miner, err := mining.NewMiningManager(loggerWithAddr, ctx, cfg.Mining, contractTellor, taskerChs[account.Address.String()], submitterCh, client)
				if err != nil {
					return errors.Wrap(err, "creating miner")
				}
				g.Add(func() error {
					err := miner.Start()
					level.Info(loggerWithAddr).Log("msg", "miner shutdown complete")
					return err
				}, func(error) {
					miner.Stop()
				})
			}
		}

		if cfg.SubmitterTellorMesosphere.Enabled {
			contract, err := contracts.NewITellorMesosphere(client)
			if err != nil {
				return errors.Wrap(err, "create contract instance")
			}

			// Create a submitter for each account.
			for _, account := range accounts {
				loggerWithAddr := log.With(logger, "addr", account.Address.String()[:6])
				psr := psrTellorMesosphere.New(loggerWithAddr, cfg.PsrTellorMesosphere, aggregator)
				transactor, err := transactor.New(loggerWithAddr, cfg.Transactor, gasPriceQuerier, client, account)
				if err != nil {
					return errors.Wrap(err, "creating transactor")
				}

				submitter, err := tellorMesosphere.New(
					ctx,
					loggerWithAddr,
					cfg.SubmitterTellorMesosphere,
					client,
					contract,
					account,
					transactor,
					psr,
				)
				if err != nil {
					return errors.Wrap(err, "creating tellor mesosphere submitter")
				}
				g.Add(func() error {
					err := submitter.Start()
					level.Info(loggerWithAddr).Log("msg", "tellor mesosphere submitter shutdown complete")
					return err
				}, func(error) {
					submitter.Stop()
				})
			}
		}

	}

	if err := g.Run(); err != nil {
		level.Error(logger).Log("msg", "main exited with error", "err", err)
		return err
	}

	level.Info(logger).Log("msg", "main shutdown complete")
	return nil
}
