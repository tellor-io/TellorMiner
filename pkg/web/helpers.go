// Copyright (c) The Tellor Authors.
// Licensed under the MIT License.

package web

import (
	"context"
	"crypto/tls"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

func Get(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := http.Client{Transport: tr}
	ticker := time.NewTicker(1 * time.Second)

	req, err := http.NewRequest("GET", ExpandTimeVars(url), nil)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Add(k, v)
	}

	var errFinal error
	for i := 0; i < 5; i++ {
		r, err := client.Do(req)
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

func ExpandTimeVars(url string) string {
	nowMillisecons := strconv.Itoa(int(time.Now().Unix() * 1000))
	url = strings.Replace(url, "$NOW", nowMillisecons, -1)

	millsIn1day := 86400000
	eodMillisecons := strconv.Itoa(int(time.Now().Unix()*1000) - millsIn1day)
	url = strings.Replace(url, "$EOD", eodMillisecons, -1)

	return url
}
