package node

import (
	"context"
	"testing"

	"github.com/photon-storage/go-common/testing/require"
	"github.com/photon-storage/go-gw3/common/http"
)

func TestContext(t *testing.T) {
	ctx := context.Background()

	require.Nil(t, GetArgsFromCtx(ctx))

	args := http.NewArgs().SetParam(http.ParamIPFSArg, "mock_arg")
	ctx = WithArgs(ctx, args)
	gotArgs := GetArgsFromCtx(ctx)
	require.NotNil(t, gotArgs)
	require.Equal(t, "mock_arg", gotArgs.GetParam(http.ParamIPFSArg))

	require.False(t, GetNoAuthFromCtx(ctx))
	ctx = WithNoAuth(ctx)
	require.True(t, GetNoAuthFromCtx(ctx))

	require.False(t, GetNoReportFromCtx(ctx))
	ctx = WithNoReport(ctx)
	require.True(t, GetNoReportFromCtx(ctx))
}
