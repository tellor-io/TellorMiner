// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package index

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/tellor-io/telliot/pkg/contracts"
	"github.com/tellor-io/telliot/pkg/ethereum"
	"github.com/tellor-io/telliot/pkg/format"
	"github.com/tellor-io/telliot/pkg/logging"
	"github.com/tellor-io/telliot/pkg/web"
	"github.com/yalp/jsonpath"
)

const (
	ComponentName      = "indexTracker"
	ValueSuffix        = "value"
	IntervalSuffix     = "interval"
	ValueMetricName    = ComponentName + "_" + ValueSuffix
	IntervalMetricName = ComponentName + "_" + IntervalSuffix
)

type Config struct {
	LogLevel  string
	Interval  format.Duration
	IndexFile string
}

type IndexTracker struct {
	logger      log.Logger
	ctx         context.Context
	stop        context.CancelFunc
	tsDB        *tsdb.DB
	cfg         Config
	dataSources map[string][]DataSource
	value       *prometheus.GaugeVec
	getErrors   *prometheus.CounterVec
}

func New(
	logger log.Logger,
	ctx context.Context,
	cfg Config,
	tsDB *tsdb.DB,
	client contracts.ETHClient,
) (*IndexTracker, error) {
	logger, err := logging.ApplyFilter(cfg.LogLevel, logger)
	if err != nil {
		return nil, errors.Wrap(err, "apply filter logger")
	}

	dataSources, err := createDataSources(ctx, cfg, client)
	if err != nil {
		return nil, errors.Wrap(err, "create data sources")
	}

	ctx, stop := context.WithCancel(ctx)

	return &IndexTracker{
		logger:      log.With(logger, "component", ComponentName),
		ctx:         ctx,
		stop:        stop,
		dataSources: dataSources,
		tsDB:        tsDB,
		cfg:         cfg,
		getErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "telliot",
			Subsystem: ComponentName,
			Name:      "errors_total",
			Help:      "The total number of get errors. Usually caused by API throtling.",
		}, []string{"source"}),
		value: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "telliot",
			Subsystem: ComponentName,
			Name:      ValueSuffix,
			Help:      "The current tracker value",
		},
			[]string{"symbol", "domain", "source"},
		),
	}, nil
}

func createDataSources(ctx context.Context, cfg Config, client contracts.ETHClient) (map[string][]DataSource, error) {
	// Load index file.
	byteValue, err := ioutil.ReadFile(cfg.IndexFile)
	if err != nil {
		return nil, errors.Wrapf(err, "read index file path:%s", cfg.IndexFile)
	}
	// Parse to json.
	indexes := make(map[string]Apis)
	err = json.Unmarshal(byteValue, &indexes)
	if err != nil {
		return nil, errors.Wrap(err, "parse index file")
	}

	dataSources := make(map[string][]DataSource)

	for symbol, api := range indexes {
		for _, endpoint := range api.Endpoints {
			endpoint.URL = os.Expand(endpoint.URL, func(key string) string {
				return os.Getenv(key)
			})

			var source DataSource

			// Default value for the api type.
			if endpoint.Type == "" {
				endpoint.Type = httpSource
			}

			// Default value for the parser.
			if endpoint.Parser == "" {
				endpoint.Parser = jsonPathParser
			}
			switch endpoint.Type {
			case httpSource:
				{
					source = NewJSONapi(api.Interval.Duration, endpoint.URL, NewParser(endpoint))
					if strings.Contains(strings.ToLower(symbol), "volume") {
						source = NewJSONapiVolume(api.Interval.Duration, endpoint.URL, NewParser(endpoint))
					}
				}
			case ethereumSource:
				{
					// Getting current network id from geth node.
					networkID, err := client.NetworkID(ctx)
					if err != nil {
						return nil, err
					}
					// Validate and pick an ethereum address for current network id.
					address, err := ethereum.GetAddressForNetwork(endpoint.URL, networkID.Int64())
					if err != nil {
						return nil, errors.Wrap(err, "getting address for network id")
					}
					if endpoint.Parser == uniswapParser {
						source = NewUniswap(symbol, address, api.Interval.Duration, client)

					} else if endpoint.Parser == balancerParser {
						source = NewBalancer(symbol, address, api.Interval.Duration, client)
					} else {
						return nil, errors.Wrapf(err, "unknown source for on-chain index tracker")
					}
				}
			default:
				return nil, errors.Errorf("unknown index type for index object:%v", endpoint.Type)
			}

			dataSources[symbol] = append(dataSources[symbol], source)
		}

	}
	return dataSources, nil

}

func (self *IndexTracker) Run() error {
	delay := time.Second
	for symbol, dataSources := range self.dataSources {
		for _, dataSource := range dataSources {
			// Use the default interval when not set.
			interval := dataSource.Interval()
			if int64(interval) == 0 {
				interval = self.cfg.Interval.Duration
			}

			go self.record(delay, symbol, interval, dataSource)
			delay += time.Second
		}
	}
	<-self.ctx.Done()
	return nil
}

// record from all API calls.
// The request delay is used to avoid rate limiting at startup
// for when all API calls try to happen at the same time.
func (self *IndexTracker) record(delay time.Duration, symbol string, interval time.Duration, dataSource DataSource) {
	time.Sleep(delay)

	ticker := time.NewTicker(interval)
	logger := log.With(self.logger, "source", dataSource.Source())

	for {
		ts := timestamp.FromTime(time.Now())

		// Record the source interval to use it for the confidence calculation.
		// Confidence = avg(actualSamplesCount/expectedMaxSamplesCount) for a given period.
		source, err := url.Parse(dataSource.Source())
		if err != nil {
			level.Error(logger).Log("msg", "parsing url from data source", "err", err)
			select {
			case <-self.ctx.Done():
				level.Debug(self.logger).Log("msg", "values record loop exited")
				return
			case <-ticker.C:
				continue
			}
		}
		{
			appender := self.tsDB.Appender(self.ctx)

			lbls := labels.Labels{
				labels.Label{Name: "__name__", Value: IntervalMetricName},
				labels.Label{Name: "source", Value: dataSource.Source()},
				labels.Label{Name: "domain", Value: source.Host},
				labels.Label{Name: "symbol", Value: format.SanitizeMetricName(symbol)},
			}

			sort.Sort(lbls) // This is important! The labels need to be sorted to avoid creating the same series with duplicate reference.

			if _, err = appender.Append(0,
				lbls,
				ts,
				float64(interval),
			); err != nil {
				level.Error(logger).Log("msg", "append values to the DB", "err", err)
				select {
				case <-self.ctx.Done():
					level.Debug(self.logger).Log("msg", "values record loop exited")
					return
				case <-ticker.C:
					continue
				}
			}

			if err := appender.Commit(); err != nil {
				level.Error(logger).Log("msg", "adding values to the DB", "err", err)
				if err := appender.Rollback(); err != nil {
					level.Error(logger).Log("msg", "rollback DB append", "err", err)
				}

				select {
				case <-self.ctx.Done():
					level.Debug(self.logger).Log("msg", "values record loop exited")
					return
				case <-ticker.C:
					continue
				}
			}
		}

		value, err := dataSource.Get(self.ctx)
		if err != nil {
			level.Error(logger).Log("msg", "getting values from data source", "err", err)
			self.getErrors.With(prometheus.Labels{"source": dataSource.Source()}).(prometheus.Counter).Inc()
			select {
			case <-self.ctx.Done():
				level.Debug(self.logger).Log("msg", "values record loop exited")
				return
			case <-ticker.C:
				continue
			}
		}

		// Record the actual value.
		{
			appender := self.tsDB.Appender(self.ctx)
			lbls := labels.Labels{
				labels.Label{Name: "__name__", Value: ValueMetricName},
				labels.Label{Name: "source", Value: dataSource.Source()},
				labels.Label{Name: "domain", Value: source.Host},
				labels.Label{Name: "symbol", Value: format.SanitizeMetricName(symbol)},
			}
			sort.Sort(lbls) // This is important! The labels need to be sorted to avoid creating the same series with duplicate reference.

			level.Debug(logger).Log("msg", "adding value to db", "source", dataSource.Source(), "host", source.Host, "symbol", format.SanitizeMetricName(symbol), "value", value, "interval", interval)
			if _, err = appender.Append(0,
				lbls,
				ts,
				value,
			); err != nil {
				level.Error(logger).Log("msg", "append values to the DB", "err", err)
				if err := appender.Rollback(); err != nil {
					level.Error(logger).Log("msg", "rollback DB append", "err", err)
				}
				select {
				case <-self.ctx.Done():
					level.Debug(logger).Log("msg", "values record loop exited")
					return
				case <-ticker.C:
					continue
				}
			}

			if err := appender.Commit(); err != nil {
				level.Error(logger).Log("msg", "adding values to the DB", "err", err)
				select {
				case <-self.ctx.Done():
					level.Debug(logger).Log("msg", "values record loop exited")
					return
				case <-ticker.C:
					continue
				}
			}

			self.value.With(
				prometheus.Labels{
					"source": dataSource.Source(),
					"domain": source.Host,
					"symbol": format.SanitizeMetricName(symbol),
				},
			).(prometheus.Gauge).Set(value)

		}

		select {
		case <-self.ctx.Done():
			level.Debug(logger).Log("msg", "values record loop exited")
			return
		case <-ticker.C:
			continue
		}
	}
}

func (self *IndexTracker) Stop() {
	self.stop()
}

// IndexType -> index type for Api.
type IndexType string

const (
	httpSource     IndexType = "http"
	ethereumSource IndexType = "ethereum"
)

// ParserType -> index parser for Api.
type ParserType string

const (
	jsonPathParser ParserType = "jsonPath"
	uniswapParser  ParserType = "Uniswap"
	balancerParser ParserType = "Balancer"
)

type Endpoint struct {
	URL    string
	Type   IndexType
	Parser ParserType
	Param  string
}

// Apis will be used in parsing index file.
type Apis struct {
	// The recommended interval for calling the Get method.
	// Some APIs will return an error if called more often
	// Due to API rate limiting of the provider.
	Interval  format.Duration
	Endpoints []Endpoint
}

// NewJSONapiVolume are treated differently and return 0 values when the api returns the same timestamp.
// This is to avoid double counting volumes for the same time period.
// Another way is to skip adding the data, but this messes up the confidence calculations
// which counts total added data points.
func NewJSONapiVolume(interval time.Duration, url string, parser Parser) *JSONapiVolume {
	return &JSONapiVolume{
		JSONapi: &JSONapi{
			url:      url,
			interval: interval,
			Parser:   parser,
		},
	}
}

type JSONapiVolume struct {
	*JSONapi
	lastTS time.Time
}

func (self *JSONapiVolume) Get(ctx context.Context) (float64, error) {
	vals, err := web.Fetch(ctx, self.url)
	if err != nil {
		return 0, errors.Wrapf(err, "fetching data from API url:%v", self.url)
	}
	val, ts, err := self.Parse(vals)
	if err != nil {
		return 0, errors.Wrapf(err, "parsing data from API url:%v", self.url)
	}

	// Use 0 value for the volume as this has already been requested.
	if self.lastTS.Equal(ts) {
		val = 0
	}
	self.lastTS = ts

	return val, nil

}

func NewJSONapi(interval time.Duration, url string, parser Parser) *JSONapi {
	return &JSONapi{
		url:      url,
		interval: interval,
		Parser:   parser,
	}
}

type JSONapi struct {
	url      string
	interval time.Duration
	Parser
}

func (self *JSONapi) Get(ctx context.Context) (float64, error) {
	vals, err := web.Fetch(ctx, self.url)
	if err != nil {
		return 0, errors.Wrapf(err, "fetching data from API url:%v", self.url)
	}
	val, _, err := self.Parse(vals)
	return val, err
}

func (self *JSONapi) Interval() time.Duration {
	return self.interval
}

func (self *JSONapi) Source() string {
	return self.url
}

type DataSource interface {
	// Source returns the data source.
	Source() string
	// Get returns current api value.
	Get(context.Context) (float64, error)
	// The recommended interval for calling the Get method.
	// Some APIs will return an error if called more often
	// Due to API rate limiting of the provider.
	Interval() time.Duration
}

type Parser interface {
	Parse([]byte) (value float64, timestamp time.Time, err error)
}

type JsonPathParser struct {
	param string
}

func (self *JsonPathParser) Parse(input []byte) (float64, time.Time, error) {
	var output interface{}

	maxErrL := len(string(input)) - 1
	if maxErrL > 200 {
		maxErrL = 200
	}

	timestamp := time.Now()

	err := json.Unmarshal(input, &output)
	if err != nil {
		return 0, timestamp, errors.Wrapf(err, "json marshal:%v", string(input)[:maxErrL])
	}

	output, err = jsonpath.Read(output, self.param)
	if err != nil {
		return 0, timestamp, errors.Wrapf(err, "json path read:%v", string(input)[:maxErrL])
	}

	// Expect result to be a slice of float or a single float value.
	var resultList []interface{}
	switch result := output.(type) {
	case []interface{}:
		resultList = result
	default:
		resultList = []interface{}{result}
	}
	// Parse each item of slice to a float.
	var value float64
	for i, a := range resultList {
		strValue := fmt.Sprintf("%v", a)
		// Normalize based on american locale.
		strValue = strings.Replace(strValue, ",", "", -1)

		switch i {
		case 0:
			val, err := strconv.ParseFloat(strValue, 64)
			if err != nil {
				return 0, timestamp, errors.Wrapf(err, "value needs to be a valid float:%v", strValue)
			}
			value = val
		case 1:
			val, err := strconv.ParseFloat(strValue, 64)
			if err != nil {
				return 0, timestamp, errors.Wrapf(err, "timestamp needs to be a valid float:%v", strValue)
			}
			timestamp = time.Unix(int64(val), 0)
			if int64(val) > 9999999999 { // The TS is with Millisecond granularity.
				timestamp = time.Unix(0, int64(val)*int64(time.Millisecond))
			}
		}
	}
	return value, timestamp, nil
}

func NewParser(t Endpoint) Parser {
	switch t.Parser {
	case jsonPathParser:
		return &JsonPathParser{
			param: t.Param,
		}
	default:
		return nil
	}
}
