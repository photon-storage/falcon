package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"

	"github.com/enescakir/emoji"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multibase"

	"github.com/photon-storage/go-common/log"
)

func gatewayPost(
	ctx context.Context,
	cfg config,
	path string,
	header http.Header,
	body io.Reader,
) (int, http.Header, []byte, error) {
	return httpCall(ctx, cfg, http.MethodPost, path, header, body)
}

func gatewayPut(
	ctx context.Context,
	cfg config,
	path string,
	header http.Header,
	body io.Reader,
) (int, http.Header, []byte, error) {
	return httpCall(ctx, cfg, http.MethodPut, path, header, body)
}

func gatewayDel(
	ctx context.Context,
	cfg config,
	path string,
	header http.Header,
) (int, http.Header, []byte, error) {
	return httpCall(ctx, cfg, http.MethodDelete, path, header, nil)
}

func gatewayGet(
	ctx context.Context,
	cfg config,
	path string,
	header http.Header,
) (int, http.Header, []byte, error) {
	return httpCall(ctx, cfg, http.MethodGet, path, header, nil)
}

func httpCall(
	ctx context.Context,
	cfg config,
	method string,
	path string,
	header http.Header,
	body io.Reader,
) (int, http.Header, []byte, error) {
	url := fmt.Sprintf("http://%s:%d", cfg.host, cfg.port)
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	if len(path) > 0 {
		url = url + "/" + path
	}
	req, err := http.NewRequestWithContext(
		ctx,
		method,
		url,
		body,
	)
	if err != nil {
		return 0, nil, nil, err
	}

	for k := range header {
		req.Header.Set(k, header.Get(k))
	}

	fmt.Printf("URL: %v\n", req.URL.String())

	resp, err := cfg.cli.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, err
	}

	return resp.StatusCode, resp.Header, data, nil
}

func logStep(format string, a ...any) {
	fmt.Printf(
		"%v  %s\n",
		emoji.RightArrow,
		log.White(fmt.Sprintf(format, a...)),
	)
}

func logResp(code int, header http.Header, data []byte, err error) error {
	if err != nil {
		fmt.Printf("Request Error: %v\n", err)
	} else {
		fmt.Printf("Response Code: %v (%v)\n", code, http.StatusText(code))
		if len(header) == 0 {
			fmt.Printf("Response Header: <empty>\n")
		} else {
			var keys []string
			for k, _ := range header {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for i, k := range keys {
				vals := header.Get(k)
				if i == 0 {
					fmt.Printf("Response Header: %v = %v\n", k, vals)
				} else {
					fmt.Printf("                 %v = %v\n", k, vals)
				}
			}
		}
		if len(data) != 0 {
			fmt.Printf("Response Body: %s\n", data)
		}
	}

	if err != nil {
		return err
	}

	if code != 200 && code != 201 {
		return fmt.Errorf("unexpected http code: %v (%v)",
			code,
			http.StatusText(code),
		)
	}

	return nil
}

func toB36(k string) (string, error) {
	c, err := cid.Decode(k)
	if err != nil {
		return "", err
	}
	c = cid.NewCidV1(c.Type(), c.Hash())
	return c.StringOfBase(multibase.Base32)
}
