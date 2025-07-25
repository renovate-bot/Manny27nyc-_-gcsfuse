// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"

	"golang.org/x/oauth2"
)

// newProxyTokenSource returns a TokenSource that calls an external
// endpoint for authentication and access tokens.
func newProxyTokenSource(
	ctx context.Context,
	endpoint string,
	reuseTokenFromUrl bool,
) (ts oauth2.TokenSource, err error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		err = fmt.Errorf("newProxyTokenSource cannot parse endpoint %s: %w", endpoint, err)
		return nil, err
	}

	client := &http.Client{}
	if u.Scheme == "unix" {
		client.Transport = &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				dialer := net.Dialer{}
				return dialer.DialContext(ctx, u.Scheme, u.Path)
			},
		}
		endpoint = "http://unix?" + u.RawQuery
	}

	ts = proxyTokenSource{
		ctx:      ctx,
		endpoint: endpoint,
		client:   client,
	}
	if reuseTokenFromUrl {
		return oauth2.ReuseTokenSource(nil, ts), nil
	}
	return ts, nil
}

type proxyTokenSource struct {
	ctx      context.Context
	endpoint string
	client   *http.Client
}

func (ts proxyTokenSource) Token() (token *oauth2.Token, err error) {
	resp, err := ts.client.Get(ts.endpoint)
	if err != nil {
		err = fmt.Errorf("proxyTokenSource cannot fetch token: %w", err)
		return nil, err
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		err = fmt.Errorf("proxyTokenSource cannot load body: %w", err)
		return nil, err
	}

	if c := resp.StatusCode; c < 200 || c >= 300 {
		err = &oauth2.RetrieveError{
			Response: resp,
			Body:     body,
		}
		return nil, err
	}

	token = &oauth2.Token{}
	err = json.Unmarshal(body, token)
	if err != nil {
		err = fmt.Errorf("proxyTokenSource cannot decode body: %w", err)
		return nil, err
	}

	return token, nil
}

func NewTokenSourceFromURL(ctx context.Context, tokenUrl string, reuseTokenFromUrl bool) (tokenSrc oauth2.TokenSource, err error) {
	return newProxyTokenSource(ctx, tokenUrl, reuseTokenFromUrl)
}
