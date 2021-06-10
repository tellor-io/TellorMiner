// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package web

import (
	"context"
	"crypto/tls"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

func Fetch(ctx context.Context, url string) ([]byte, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := http.Client{Transport: tr}
	ticker := time.NewTicker(1 * time.Second)

	var errFinal error
	for i := 0; i < 5; i++ {
		r, err := client.Get(url)
		if err != nil {
			errFinal = errors.Wrap(err, "fetching data")
			select {
			case <-ticker.C:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			errFinal = errors.Wrap(err, "read response body")
			select {
			case <-ticker.C:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		r.Body.Close()

		if r.StatusCode/100 != 2 {
			errFinal = errors.Errorf("response status code not OK code:%v, payload:%v", r.StatusCode, string(data))
			select {
			case <-ticker.C:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return data, nil
	}

	return nil, errFinal

}
