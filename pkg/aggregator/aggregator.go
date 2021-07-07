// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package aggregator

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"math"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/tellor-io/telliot/pkg/format"
	"github.com/tellor-io/telliot/pkg/logging"
	"github.com/tellor-io/telliot/pkg/tracker/index"
)

const ComponentName = "aggregator"

type IAggregator interface {
	TimeWeightedAvg(symbol string, start time.Time, lookBack time.Duration) (float64, float64, error)
}

type Config struct {
	LogLevel       string
	ManualDataFile string
}

type Aggregator struct {
	logger       log.Logger
	ctx          context.Context
	tsDB         storage.SampleAndChunkQueryable
	promqlEngine *promql.Engine
	cfg          Config
}

func New(
	logger log.Logger,
	ctx context.Context,
	cfg Config,
	tsDB storage.SampleAndChunkQueryable,
) (*Aggregator, error) {
	logger, err := logging.ApplyFilter(cfg.LogLevel, logger)
	if err != nil {
		return nil, errors.Wrap(err, "apply filter logger")
	}

	opts := promql.EngineOpts{
		Logger:               logger,
		Reg:                  nil,
		MaxSamples:           30000,
		Timeout:              10 * time.Second,
		LookbackDelta:        5 * time.Minute,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
	}
	engine := promql.NewEngine(opts)

	return &Aggregator{
		logger:       log.With(logger, "component", ComponentName),
		ctx:          ctx,
		tsDB:         tsDB,
		promqlEngine: engine,
		cfg:          cfg,
	}, nil
}

func (self *Aggregator) ManualValue(oracleName string, reqID int64, ts time.Time) (float64, error) {
	jsonFile, err := os.Open(self.cfg.ManualDataFile)
	if err != nil {
		return 0, errors.Wrapf(err, "manual data file read Error")
	}
	defer jsonFile.Close()
	byteValue, _ := ioutil.ReadAll(jsonFile)
	var result map[string]map[string]map[string]float64
	err = json.Unmarshal([]byte(byteValue), &result)
	if err != nil {
		return 0, errors.Wrap(err, "unmarshal manual data file")
	}

	oracleManualVals, ok := result[oracleName]
	if !ok {
		return 0, errors.Wrapf(err, "malformatted json file for oracle:%v", oracleName)
	}
	val := oracleManualVals[strconv.FormatInt(reqID, 10)]["VALUE"]
	if val != 0 {
		_timestamp := int64(oracleManualVals[strconv.FormatInt(reqID, 10)]["DATE"])
		timestamp := time.Unix(_timestamp, 0)
		if ts.After(timestamp) {
			return 0, errors.Errorf("manual entry value has expired:%v", ts)
		}
	}
	return val, nil
}

func (self *Aggregator) MedianAt(symbol string, at time.Time) (float64, float64, error) {
	vals, confidence, err := self.valsAtWithConfidence(symbol, at)
	if err != nil {
		return 0, 0, err
	}
	if len(vals) == 0 {
		return 0, 0, errors.Errorf("no vals at:%v", at)
	}
	median, confidenceM := self.median(vals)
	if confidenceM < confidence {
		confidence = confidenceM
	}

	return median, confidence, nil
}

func (self *Aggregator) MedianAtEOD(symbol string, at time.Time) (float64, float64, error) {
	d := 24 * time.Hour
	eod := time.Now().Truncate(d)
	return self.MedianAt(symbol, eod)
}

func (self *Aggregator) MeanAt(symbol string, at time.Time) (float64, float64, error) {
	vals, confidence, err := self.valsAtWithConfidence(symbol, at)
	if err != nil {
		return 0, 0, err
	}
	price, confidenceM := self.mean(vals)
	if confidenceM < confidence {
		confidence = confidenceM
	}
	return price, confidence * 100, nil
}

func (self *Aggregator) mean(vals []float64) (float64, float64) {
	if len(vals) == 1 {
		return vals[0], 100
	}

	priceSum := 0.0
	min, max := vals[0], vals[0]
	for _, val := range vals {
		priceSum += val
		if val < min {
			min = val
		}
		if val > max {
			max = val
		}
	}
	return priceSum / float64(len(vals)), confidenceInDifference(min, max)
}

// TimeWeightedAvg returns price and confidence level for a given symbol.
// Confidence is calculated based on maximum possible samples over the actual samples for a given period.
// avg(maxPossibleSamplesCount/actualSamplesCount)
// For example with 1h look back and source interval of 60sec maxPossibleSamplesCount = 36
// with actualSamplesCount = 18 this is 50% confidence.
// The same calculation is done for all source and the final value is the average of all.
//
// Example for 1h.
// maxDataPointCount is calculated by deviding the seconds in 1h by how often the tracker queries the APIs.
// avg(count_over_time(indexTracker_value{symbol="AMPL_USD"}[1h]) / (3.6e+12/indexTracker_interval)).
func (self *Aggregator) TimeWeightedAvg(
	symbol string,
	start time.Time,
	lookBack time.Duration,
) (float64, float64, error) {
	resolution, err := self.resolution(symbol, start)
	if err != nil {
		return 0, 0, err
	}

	// Avg value over the look back period.
	query, err := self.promqlEngine.NewInstantQuery(
		self.tsDB,
		`avg_over_time(
			`+index.ValueMetricName+`{symbol="`+format.SanitizeMetricName(symbol)+`"}
		[`+lookBack.String()+`])`,
		start,
	)
	if err != nil {
		return 0, 0, err
	}
	defer query.Close()
	_result := query.Exec(self.ctx)
	if _result.Err != nil {
		return 0, 0, errors.Wrapf(_result.Err, "error evaluating query:%v", query.Statement())
	}
	if len(_result.Value.(promql.Vector)) == 0 {
		return 0, 0, errors.Errorf("no result for TWAP vals query:%v", query.Statement())
	}

	result := _result.Value.(promql.Vector)[0].V

	// Confidence level.
	query, err = self.promqlEngine.NewInstantQuery(
		self.tsDB,
		`avg(
			count_over_time(
				`+index.ValueMetricName+`{ symbol="`+format.SanitizeMetricName(symbol)+`" }
			[`+lookBack.String()+`:`+resolution.String()+`])
			/
			(`+strconv.Itoa(int(lookBack.Nanoseconds()))+` / `+strconv.Itoa(int(resolution.Nanoseconds()))+`)
		)`,
		start,
	)

	if err != nil {
		return 0, 0, err
	}
	defer query.Close()
	confidence := query.Exec(self.ctx)
	if confidence.Err != nil {
		return 0, 0, errors.Wrapf(confidence.Err, "error evaluating query:%v", query.Statement())
	}

	if len(confidence.Value.(promql.Vector)) == 0 {
		return 0, 0, errors.Errorf("no result for TWAP confidence query:%v", query.Statement())
	}

	return result, confidence.Value.(promql.Vector)[0].V * 100, err
}

// VolumWeightedAvg returns price and confidence level for a given symbol.
// Confidence is calculated based on maximum possible samples over the actual samples for a given period.
// avg(maxPossibleSamplesCount/actualSamplesCount)
// For example with 1h look back and source interval of 60sec maxPossibleSamplesCount = 36
// with actualSamplesCount = 18 this is 50% confidence.
// The same calculation is done for all source and the final value is the average of all.
// maxDataPointCount is calculated by deviding the seconds in 1h by how often the tracker queries the APIs.
//
// Example for 1h.
// avg(count_over_time(indexTracker_value{symbol="AMPL_USD"}[1h]) / (3.6e+12/30s)).
//
// vals are calculated using the official VWAP formula from
// https://tradingtuitions.com/vwap-trading-strategy-excel-sheet/
func (self *Aggregator) VolumWeightedAvg(
	symbol string,
	start time.Time,
	end time.Time,
	aggrWindow time.Duration,
) (float64, float64, error) {
	_timeWindow := end.Sub(start).Round(time.Minute).Seconds()
	timeWindow := strconv.Itoa(int(_timeWindow)) + "s"

	resolution, err := self.resolution(symbol, end)
	if err != nil {
		return 0, 0, err
	}

	query, err := self.promqlEngine.NewInstantQuery(
		self.tsDB,
		`avg(
			sum_over_time(
				(
					sum_over_time( `+index.ValueMetricName+`{symbol="`+format.SanitizeMetricName(symbol)+`_VOLUME"}[`+aggrWindow.String()+`]
					) * on(domain)
					avg_over_time(`+index.ValueMetricName+`{symbol="`+format.SanitizeMetricName(symbol)+`"}[`+aggrWindow.String()+`])
				)
			[`+timeWindow+`:`+aggrWindow.String()+`])
			/ on(domain)
			sum_over_time(
					sum_over_time(`+index.ValueMetricName+`{symbol="`+format.SanitizeMetricName(symbol)+`_VOLUME"}[`+aggrWindow.String()+`])
			[`+timeWindow+`:`+aggrWindow.String()+`])
		)`,
		end,
	)

	if err != nil {
		return 0, 0, err
	}
	defer query.Close()

	// TODO: Add directly in the erros logs when this issues is fixed - https://github.com/prometheus/prometheus/issues/8949
	qStmt := query.Statement().String()

	_result := query.Exec(self.ctx)
	if _result.Err != nil {
		return 0, 0, errors.Wrapf(_result.Err, "error evaluating query:%v", qStmt)
	}
	result := _result.Value.(promql.Vector)
	if len(result) == 0 {
		return 0, 0, errors.Errorf("no result for VWAP vals query:%v", qStmt)
	}

	// Confidence level for prices.
	query, err = self.promqlEngine.NewInstantQuery(
		self.tsDB,
		`avg(
			count_over_time(`+index.ValueMetricName+`{symbol="`+format.SanitizeMetricName(symbol)+`"}[`+timeWindow+`])
			/
			(`+strconv.Itoa(int(end.Sub(start).Nanoseconds()))+` / `+strconv.Itoa(int(resolution.Nanoseconds()))+`)
		)`,
		end,
	)
	if err != nil {
		return 0, 0, err
	}
	defer query.Close()

	confidenceP := query.Exec(self.ctx)
	if confidenceP.Err != nil {
		return 0, 0, errors.Wrapf(confidenceP.Err, "error evaluating query:%v", query.Statement())
	}

	// Confidence level for volumes.
	resolution, err = self.resolution(symbol+"/VOLUME", end)
	if err != nil {
		return 0, 0, err
	}
	query, err = self.promqlEngine.NewInstantQuery(
		self.tsDB,
		`avg(
			count_over_time(`+index.ValueMetricName+`{symbol="`+format.SanitizeMetricName(symbol)+`_VOLUME"}[`+timeWindow+`])
			/
			(`+strconv.Itoa(int(end.Sub(start).Nanoseconds()))+` / `+strconv.Itoa(int(resolution.Nanoseconds()))+`)
		)`,
		end,
	)
	if err != nil {
		return 0, 0, err
	}
	defer query.Close()
	confidenceV := query.Exec(self.ctx)
	if confidenceV.Err != nil {
		return 0, 0, errors.Wrapf(confidenceV.Err, "error evaluating query:%v", query.Statement())
	}

	if len(confidenceP.Value.(promql.Vector)) == 0 || len(confidenceV.Value.(promql.Vector)) == 0 {
		return 0, 0, errors.Errorf("no result for VWAP confidence query:%v", query.Statement())
	}

	// Use the smaller confidence of volume or value.
	confidence := confidenceP.Value.(promql.Vector)[0].V
	if confidence > confidenceV.Value.(promql.Vector)[0].V {
		confidence = confidenceV.Value.(promql.Vector)[0].V
	}

	// Return the last VWAP price.
	return result[len(result)-1].V, confidence * 100, nil
}

func (self *Aggregator) median(vals []float64) (float64, float64) {
	if len(vals) == 1 {
		return vals[0], 100
	}
	sort.Slice(vals, func(i, j int) bool {
		return vals[i] < vals[j]
	})

	position := len(vals) / 2
	price := vals[position]

	// When number of vals is even need to use the mean
	// of the 2 middle vals.
	if len(vals)%2 == 0 {
		price = (vals[position-1] + vals[position]) / 2
	}

	return price, confidenceInDifference(vals[0], vals[len(vals)-1])
}

// confidenceInDifference calculates the percentage difference between the max and min and subtract this from 100%.
// Example:
// min 1, max 2
// Difference is 1 which is 100% so the final confidence is 100-100 equals 0%.
func confidenceInDifference(min, max float64) float64 {
	return 100 - (math.Abs(min-max)/min)*100
}

// valsAtWithConfidence returns the value from all sources for a given symbol with the confidence level.
// 100% confidence is when all apis have returned a value within the last tracker interval.
// For every missing value the calculation subtracts some confidence level.
// Confidence is calculated actualDataPointCount/maxDataPointCount.
// maxDataPointCount = timeWindow/trackerCycle
//
// Example confidence for 1h.
// avg(count_over_time(indexTracker_value{symbol="AMPL_USD"}[1h]) / (3.6e+12/30s)).
func (self *Aggregator) valsAtWithConfidence(symbol string, at time.Time) ([]float64, float64, error) {
	resolution, err := self.resolution(symbol, at)
	if err != nil {
		return nil, 0, err
	}
	lookBack := time.Duration(resolution + 1e+9) // 1 sec more then the pull interval to make sure the tracker has added a value. Interval is in nanosecond granularity.
	var prices []float64
	pricesVector, err := self.valsAt(symbol, at, lookBack)
	if err != nil {
		return nil, 0, err
	}

	for _, price := range pricesVector {
		prices = append(prices, price.V)
	}

	// Confidence level.
	query, err := self.promqlEngine.NewInstantQuery(
		self.tsDB,
		`avg(
			count_over_time(`+index.ValueMetricName+`{ symbol="`+format.SanitizeMetricName(symbol)+`" }[`+lookBack.String()+`] )
			/
			(`+strconv.Itoa(int(lookBack.Nanoseconds()))+` / `+strconv.Itoa(int(resolution.Nanoseconds()))+`)
		)`,
		at,
	)
	if err != nil {
		return nil, 0, err
	}
	defer query.Close()
	confidence := query.Exec(self.ctx)
	if confidence.Err != nil {
		return nil, 0, errors.Wrapf(confidence.Err, "error evaluating query:%v", query.Statement())
	}
	if len(confidence.Value.(promql.Vector)) == 0 {
		return nil, 0, errors.Errorf("no vals for confidence at:%v, query:%v", at, query.Statement())
	}

	return prices, confidence.Value.(promql.Vector)[0].V * 100, nil
}

// valsAt returns all vals from all indexes at a given time.
func (self *Aggregator) valsAt(symbol string, at time.Time, lookBack time.Duration) (promql.Vector, error) {
	query, err := self.promqlEngine.NewInstantQuery(
		self.tsDB,
		`last_over_time( `+index.ValueMetricName+`{symbol="`+format.SanitizeMetricName(symbol)+`"} [`+lookBack.String()+`])`,
		at,
	)
	if err != nil {
		return nil, err
	}
	defer query.Close()
	result := query.Exec(self.ctx)
	if result.Err != nil {
		return nil, errors.Wrapf(result.Err, "error evaluating query:%v", query.Statement())
	}

	return result.Value.(promql.Vector), nil
}

func (self *Aggregator) resolution(symbol string, at time.Time) (time.Duration, error) {
	query, err := self.promqlEngine.NewInstantQuery(
		self.tsDB,
		`last_over_time(`+index.IntervalMetricName+`{symbol="`+format.SanitizeMetricName(symbol)+`"}[3h])`, // The interval is recorded on every index tracker cycle so this lookback should be sufficient.
		at,
	)
	if err != nil {
		return 0, err
	}
	defer query.Close()
	_trackerInterval := query.Exec(self.ctx)
	if _trackerInterval.Err != nil {
		return 0, errors.Wrapf(_trackerInterval.Err, "error evaluating query:%v", query.Statement())
	}
	if len(_trackerInterval.Value.(promql.Vector)) == 0 {
		return 0, errors.Errorf("no vals for tracker interval at:%v, query:%v", at, query.Statement())
	}

	return time.Duration(_trackerInterval.Value.(promql.Vector)[0].V), nil
}
