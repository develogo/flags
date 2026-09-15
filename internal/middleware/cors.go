package middleware

import (
	"flags/internal/config"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func CORS(cfg *config.Config) echo.MiddlewareFunc {
	origins := cfg.App.CorsOrigins
	if len(origins) == 0 {
		origins = []string{"*"}
	}

	return middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: origins,
		AllowMethods: []string{echo.GET, echo.POST, echo.PUT, echo.DELETE, echo.OPTIONS},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
			"User-ID",
			"Device-ID",
			"Platform",
			"Platform-Version",
			"Device-Model",
			"Device-Architecture",
			"Device-Brand",
			"Mobile",
			"Device",
			"App-Name",
			"App-Version",
			"Package-Name",
			"Build-Number",
		},
		ExposeHeaders: []string{echo.HeaderContentLength, "X-Request-ID"},
		MaxAge:        3600,
	})
}
