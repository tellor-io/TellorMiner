// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package tellor

import (
	"math"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/tellor-io/telliot/pkg/aggregator"
)

const (
	ComponentName      = "psrTellor"
	DefaultGranularity = 1000000
)

func New(logger log.Logger, cfg Config, aggregator *aggregator.Aggregator) *Psr {
	return &Psr{
		logger:     log.With(logger, "component", ComponentName),
		aggregator: aggregator,
		cfg:        cfg,
	}
}

type Config struct {
	MinConfidence float64
}

type Psr struct {
	logger     log.Logger
	aggregator *aggregator.Aggregator
	cfg        Config
}

func (self *Psr) GetValue(reqID int64, ts time.Time) (int64, error) {
	val, err := self.getValue(reqID, ts)
	return int64(math.Round(val * DefaultGranularity)), err
}

func (self *Psr) getValue(reqID int64, ts time.Time) (float64, error) {
	val, err := self.aggregator.ManualValue("tellor", reqID, ts)
	if err != nil {
		level.Error(self.logger).Log("msg", "get manual value", "reqID", reqID, "err", err)
	}
	if val != 0 {
		level.Warn(self.logger).Log("msg", "USING MANUAL VALUE", "reqID", reqID, "val", val)
		return val, nil
	}

	var conf float64
	switch reqID {
	case 1:
		val, conf, err = self.aggregator.MedianAt("ETH/USD", ts)
	case 2:
		val, conf, err = self.aggregator.MedianAt("BTC/USD", ts)
	case 3:
		val, conf, err = self.aggregator.MedianAt("BNB/USD", ts)
	case 4:
		val, conf, err = self.aggregator.TimeWeightedAvg("BTC/USD", ts, 24*time.Hour)
	case 5:
		val, conf, err = self.aggregator.MedianAt("ETH/BTC", ts)
	case 6:
		val, conf, err = self.aggregator.MedianAt("BNB/BTC", ts)
	case 7:
		val, conf, err = self.aggregator.MedianAt("BNB/ETH", ts)
	case 8:
		val, conf, err = self.aggregator.TimeWeightedAvg("ETH/USD", ts, 24*time.Hour)
	case 9:
		val, conf, err = self.aggregator.MedianAtEOD("ETH/USD", ts)
	case 10: // For more details see https://docs.google.com/document/d/1RFCApk1PznMhSRVhiyFl_vBDPA4mP2n1dTmfqjvuTNw/edit
		// For now this uses third party APIs and don't do local aggregation.
		val, conf, err = self.aggregator.MedianAt("AMPL/USD/VWAP", ts)
	case 11:
		val, conf, err = self.aggregator.MedianAt("ZEC/ETH", ts)
	case 12:
		val, conf, err = self.aggregator.MedianAt("TRX/ETH", ts)
	case 13:
		val, conf, err = self.aggregator.MedianAt("XRP/USD", ts)
	case 14:
		val, conf, err = self.aggregator.MedianAt("XMR/ETH", ts)
	case 15:
		val, conf, err = self.aggregator.MedianAt("ATOM/USD", ts)
	case 16:
		val, conf, err = self.aggregator.MedianAt("LTC/USD", ts)
	case 17:
		val, conf, err = self.aggregator.MedianAt("WAVES/BTC", ts)
	case 18:
		val, conf, err = self.aggregator.MedianAt("REP/BTC", ts)
	case 19:
		val, conf, err = self.aggregator.MedianAt("TUSD/ETH", ts)
	case 20:
		val, conf, err = self.aggregator.MedianAt("EOS/USD", ts)
	case 21:
		val, conf, err = self.aggregator.MedianAt("IOTA/USD", ts)
	case 22:
		val, conf, err = self.aggregator.MedianAt("ETC/USD", ts)
	case 23:
		val, conf, err = self.aggregator.MedianAt("ETH/PAX", ts)
	case 24:
		val, conf, err = self.aggregator.TimeWeightedAvg("ETH/BTC", ts, time.Hour)
	case 25:
		val, conf, err = self.aggregator.MedianAt("USDC/USDT", ts)
	case 26:
		val, conf, err = self.aggregator.MedianAt("XTZ/USD", ts)
	case 27:
		val, conf, err = self.aggregator.MedianAt("LINK/USD", ts)
	case 28:
		val, conf, err = self.aggregator.MedianAt("ZRX/BNB", ts)
	case 29:
		val, conf, err = self.aggregator.MedianAt("ZEC/USD", ts)
	case 30:
		val, conf, err = self.aggregator.MedianAt("XAU/USD", ts)
	case 31:
		val, conf, err = self.aggregator.MedianAt("MATIC/USD", ts)
	case 32:
		val, conf, err = self.aggregator.MedianAt("BAT/USD", ts)
	case 33:
		val, conf, err = self.aggregator.MedianAt("ALGO/USD", ts)
	case 34:
		val, conf, err = self.aggregator.MedianAt("ZRX/USD", ts)
	case 35:
		val, conf, err = self.aggregator.MedianAt("COS/USD", ts)
	case 36:
		val, conf, err = self.aggregator.MedianAt("BCH/USD", ts)
	case 37:
		val, conf, err = self.aggregator.MedianAt("REP/USD", ts)
	case 38:
		val, conf, err = self.aggregator.MedianAt("GNO/USD", ts)
	case 39:
		val, conf, err = self.aggregator.MedianAt("DAI/USD", ts)
	case 40:
		val, conf, err = self.aggregator.MedianAt("STEEM/BTC", ts)
	case 41:
		// ID 41 is always manual so it sholud never get here.
		// It is three month average for US PCE (monthly levels): https://www.bea.gov/data/personal-consumption-expenditures-price-index-excluding-food-and-energy
		return 0, errors.New("no manual entry for request ID 41")
	case 42:
		val, conf, err = self.aggregator.MedianAtEOD("BTC/USD", ts)
	case 43:
		val, conf, err = self.aggregator.MedianAt("TRB/ETH", ts)
	case 44:
		val, conf, err = self.aggregator.TimeWeightedAvg("BTC/USD", ts, time.Hour)
	case 45:
		val, conf, err = self.aggregator.MedianAtEOD("TRB/USD", ts)
	case 46:
		val, conf, err = self.aggregator.TimeWeightedAvg("ETH/USD", ts, time.Hour)
	case 47:
		val, conf, err = self.aggregator.MedianAt("BSV/USD", ts)
	case 48:
		val, conf, err = self.aggregator.MedianAt("MAKER/USD", ts)
	case 49:
		val, conf, err = self.aggregator.TimeWeightedAvg("BCH/USD", ts, 24*time.Hour)
	case 50:
		val, conf, err = self.aggregator.MedianAt("TRB/USD", ts)
	case 51:
		val, conf, err = self.aggregator.MedianAt("XMR/USD", ts)
	case 52:
		val, conf, err = self.aggregator.MedianAt("XFT/USD", ts)
	case 53:
		val, conf, err = self.aggregator.MedianAt("BTCDOMINANCE", ts)
	case 54:
		val, conf, err = self.aggregator.MedianAt("WAVES/USD", ts)
	case 55:
		val, conf, err = self.aggregator.MedianAt("OGN/USD", ts)
	case 56:
		val, conf, err = self.aggregator.MedianAt("VIXEOD", ts)
	case 57:
		val, conf, err = self.aggregator.MedianAt("DEFITVL", ts)
	case 58:
		val, conf, err = self.aggregator.MeanAt("DEFIMCAP", ts)
	default:
		return 0, errors.Errorf("undeclared request ID:%v", reqID)
	}

	if err != nil {
		return 0, err
	}

	if conf < self.cfg.MinConfidence {
		return 0, errors.Errorf("not enough confidence - value:%v, conf:%v,confidence threshold:%v", val, conf, self.cfg.MinConfidence)
	}

	return val, err
}
