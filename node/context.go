package node

import (
	"context"

	"github.com/photon-storage/go-gw3/common/http"
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
