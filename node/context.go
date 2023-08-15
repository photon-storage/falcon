package node

import (
	"context"

	"go.uber.org/atomic"

	"github.com/photon-storage/go-gw3/common/http"
	rcpinner "github.com/photon-storage/go-rc-pinner"

	"github.com/photon-storage/falcon/node/handlers"
)

const (
	ctxArgsKey  = "ctx_args"
	ctxNoAuth   = "ctx_noauth"
	ctxNoReport = "ctx_noreport"
)

func SetArgsFromCtx(ctx context.Context, args *http.Args) context.Context {
	return context.WithValue(ctx, ctxArgsKey, args)
}

func GetArgsFromCtx(ctx context.Context) *http.Args {
	v := ctx.Value(ctxArgsKey)
	if v == nil {
		return nil
	}
	args, ok := v.(*http.Args)
	if !ok {
		return nil
	}
	return args
}

func SetNoAuthFromCtx(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxNoAuth, true)
}

func GetNoAuthFromCtx(ctx context.Context) bool {
	v := ctx.Value(ctxNoAuth)
	if v == nil {
		return false
	}
	noauth, ok := v.(bool)
	if !ok {
		return false
	}
	return noauth
}

func SetNoReportFromCtx(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxNoReport, true)
}

func GetNoReportFromCtx(ctx context.Context) bool {
	v := ctx.Value(ctxNoReport)
	if v == nil {
		return false
	}
	noreport, ok := v.(bool)
	if !ok {
		return false
	}
	return noreport
}

func SetFetchSizeFromCtx(
	ctx context.Context,
	v *atomic.Uint64,
) context.Context {
	return context.WithValue(ctx, rcpinner.DagSizeContextKey, v)
}

func SetDagStatFromCtx(
	ctx context.Context,
	v *handlers.DagStats,
) context.Context {
	return context.WithValue(ctx, handlers.DagStatsCtxKey, v)
}
