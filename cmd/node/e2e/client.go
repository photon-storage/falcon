package main

import (
	"fmt"
	gohttp "net/http"
	"strings"
	"time"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"

	"github.com/photon-storage/go-gw3/common/auth"
	"github.com/photon-storage/go-gw3/common/http"
)

type authClient struct {
	*gohttp.Client
	sk libp2pcrypto.PrivKey
}

func newAuthClient(sk libp2pcrypto.PrivKey) *authClient {
	return &authClient{
		Client: gohttp.DefaultClient,
		sk:     sk,
	}
}

func (c *authClient) Do(req *gohttp.Request) (*gohttp.Response, error) {
	if c.sk != nil {
		args := http.NewArgs().
			SetArg(http.ArgP3Unixtime, fmt.Sprintf("%v", time.Now().Unix())).
			SetArg(http.ArgP3Node, "localhost")
		for k, v := range req.URL.Query() {
			args.SetParam(k, strings.Join(v, ";;;"))
		}
		req.URL.RawQuery = ""
		for k, v := range req.Header {
			args.SetHeader(k, strings.Join(v, ";;;"))
		}
		req.Header = gohttp.Header{}

		if err := auth.SignRequest(req, args, c.sk); err != nil {
			return nil, err
		}
	}

	return c.Client.Do(req)
}
