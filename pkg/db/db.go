// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package db

import (
	"context"
	"sort"

	"net/url"
	"strconv"
	"time"

	"github.com/pkg/errors"
	promConfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/tellor-io/telliot/pkg/format"
)

const ComponentName = "db"

type Config struct {
	LogLevel string
	Path     string
	// Connect to this remote DB.
	RemoteHost    string
	RemotePort    uint
	RemoteTimeout format.Duration
}

func NewRemoteDB(cfg Config) (storage.SampleAndChunkQueryable, error) {

	url, err := url.Parse("http://" + cfg.RemoteHost + ":" + strconv.Itoa(int(cfg.RemotePort)) + "/api/v1/read")
	if err != nil {
		return nil, err
	}
	client, err := remote.NewReadClient("", &remote.ClientConfig{
		URL:     &promConfig.URL{URL: url},
		Timeout: model.Duration(cfg.RemoteTimeout.Duration),
		HTTPClientConfig: promConfig.HTTPClientConfig{
			FollowRedirects: true,
		},
	})
	if err != nil {
		return nil, err
	}
	return remote.NewSampleAndChunkQueryableClient(
		client,
		labels.Labels{},
		[]*labels.Matcher{},
		true,
		func() (i int64, err error) { return 0, nil },
	), nil
}

func Add(ctx context.Context, tsdb *tsdb.DB, lbls labels.Labels, value float64) error {
	var err error
	appender := tsdb.Appender(ctx)

	// Round up the time so that all appends happen with the same TS and
	// avoid out of order samples errors.
	ts = timestamp.FromTime(time.Now().Round(5 * time.Second))

	defer func() { // An appender always needs to be committed or rolled back.
		if err != nil {
			if errR := appender.Rollback(); errR != nil {
				err = errors.Wrap(err, "db rollback failed")
			}
			return
		}
		if errC := appender.Commit(); errC != nil {
			err = errors.Wrap(err, "db append commit failed")
		}
	}()
	sort.Sort(lbls) // This is important! The labels need to be sorted to avoid creating the same series with duplicate reference.

	_, err = appender.Append(0, lbls, ts, float64(value))
	if err != nil {
		return errors.Wrap(err, "append values to the DB")
	}
	return nil
}
