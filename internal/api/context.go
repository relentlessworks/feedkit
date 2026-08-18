package api

import (
	"context"

	"github.com/relentlessworks/feedkit/internal/model"
)

func contextWithWorkspace(ctx context.Context, ws *model.Workspace) context.Context {
	return context.WithValue(ctx, workspaceKey, ws)
}

func workspaceFromContext(ctx context.Context) *model.Workspace {
	v, _ := ctx.Value(workspaceKey).(*model.Workspace)
	return v
}
