package middleware

import (
	"flags/internal/models"

	"github.com/labstack/echo/v4"
)

const clientContextKey = "client_context"

// ClientContext monta o contexto de targeting a partir dos headers declarados
// pelo cliente. Nunca rejeita a requisição; Authorization é ignorado.
func ClientContext() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Request().Header
			c.Set(clientContextKey, &models.ClientContext{
				UserID:          h.Get("User-ID"),
				DeviceID:        h.Get("Device-ID"),
				Platform:        h.Get("Platform"),
				PlatformVersion: h.Get("Platform-Version"),
				DeviceModel:     h.Get("Device-Model"),
				Architecture:    h.Get("Device-Architecture"),
				DeviceBrand:     h.Get("Device-Brand"),
				Mobile:          h.Get("Mobile"),
				Device:          h.Get("Device"),
				AppName:         h.Get("App-Name"),
				AppVersion:      h.Get("App-Version"),
				PackageName:     h.Get("Package-Name"),
				BuildNumber:     h.Get("Build-Number"),
			})

			return next(c)
		}
	}
}

func GetClientContext(c echo.Context) *models.ClientContext {
	if ctx, ok := c.Get(clientContextKey).(*models.ClientContext); ok {
		return ctx
	}
	return &models.ClientContext{}
}
