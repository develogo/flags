package middleware

import (
	"go.uber.org/fx"
)

var Module = fx.Module("middleware",
	fx.Provide(
		NewRateLimiter,
		fx.Annotate(
			ClientContext,
			fx.ResultTags(`name:"clientcontext"`),
		),
		fx.Annotate(
			CORS,
			fx.ResultTags(`name:"cors"`),
		),
		fx.Annotate(
			Logger,
			fx.ResultTags(`name:"logger"`),
		),
		fx.Annotate(
			RequestID,
			fx.ResultTags(`name:"requestid"`),
		),
	),
)
