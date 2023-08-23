package node

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	gohttp "net/http"
	"testing"
	"time"

	"go.uber.org/atomic"

	"github.com/photon-storage/go-common/testing/require"
	"github.com/photon-storage/go-gw3/common/auth"
	"github.com/photon-storage/go-gw3/common/crypto"
	"github.com/photon-storage/go-gw3/common/http"
	"github.com/photon-storage/go-gw3/common/reporting"

	"github.com/photon-storage/falcon/node/config"
	"github.com/photon-storage/falcon/node/handlers"
)

type mockHttpClient struct {
	req  *gohttp.Request
	resp *gohttp.Response
}

func (c *mockHttpClient) Do(req *gohttp.Request) (*gohttp.Response, error) {
	c.req = req
	return c.resp, nil
}

func TestSendLog(t *testing.T) {
	sk0 := crypto.PregenEd25519(0)
	sk1 := crypto.PregenEd25519(1)
	mockCli := &mockHttpClient{
		resp: &gohttp.Response{
			StatusCode: gohttp.StatusOK,
		},
	}
	cfg := &config.Config{
		HttpClient: mockCli,
		SecretKey:  sk1,
	}
	cfg.ExternalServices.Spaceport = "http://127.0.0.1:9981"
	config.Mock(cfg)

	args := http.NewArgs().
		SetArg(
			http.ArgP3Unixtime,
			fmt.Sprintf("%v", time.Now().Unix()),
		).
		SetArg(http.ArgP3Node, "test.com").
		SetHeader("header0", "header_value0").
		SetHeader("header1", "header_value1").
		SetParam("param0", "param_value")
	req, err := gohttp.NewRequest(
		gohttp.MethodGet,
		"http://127.0.0.1:8080/api/v0/dag/get",
		nil,
	)
	require.NoError(t, err)
	require.NoError(t, auth.SignRequest(req, args, sk0))

	maxSize := 1 << 20
	httpIngr := newIngressCounter(nil, maxSize)
	httpIngr.sz = 100
	httpEgr := newEgressCounter(nil)
	httpEgr.sz = 8192
	mon := newMonitor(
		nil,
		req,
		httpIngr,
		httpEgr,
		atomic.NewUint64(100),
		maxSize,
		handlers.NewDagStats(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mon.run(ctx, cancel)

	logdata, err := ioutil.ReadAll(mockCli.req.Body)
	require.NoError(t, err)

	var log reporting.LogV1
	require.NoError(t, json.Unmarshal(logdata, &log))
	require.Equal(t, 1, log.Version)
	require.Equal(t, gohttp.MethodGet, log.Req.Method)
	require.Equal(t, "/api/v0/dag/get", log.Req.URI)
	require.Equal(t, args.Encode(), log.Req.Args)
	require.NoError(t, auth.VerifySigBase64(
		auth.GenStringToSign(
			log.Req.Method,
			log.Req.Host,
			log.Req.URI,
			log.Req.Args,
		),
		log.Req.Sig,
		sk0.GetPublic(),
	))
	require.Equal(t, 0, log.PinnedCount)
	require.Equal(t, 0, log.PinnedBytes)
	require.Equal(t, 200, log.Ingress)
	require.Equal(t, 8192, log.Egress)

	require.NoError(t, auth.VerifySigBase64(
		string(logdata),
		mockCli.req.Header.Get(http.HeaderAuthorization),
		sk1.GetPublic(),
	))
}
