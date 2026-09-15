package middleware_test

import (
	"flags/internal/middleware"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestID_GeneratesNew(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var capturedID string
	handler := middleware.RequestID()(func(c echo.Context) error {
		capturedID, _ = c.Get("request_id").(string)
		return c.NoContent(http.StatusOK)
	})

	err := handler(c)
	require.NoError(t, err)
	assert.NotEmpty(t, capturedID)
	assert.Equal(t, capturedID, rec.Header().Get("X-Request-ID"))
}

func TestRequestID_PropagatesExisting(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "existing-id-123")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var capturedID string
	handler := middleware.RequestID()(func(c echo.Context) error {
		capturedID, _ = c.Get("request_id").(string)
		return c.NoContent(http.StatusOK)
	})

	err := handler(c)
	require.NoError(t, err)
	assert.Equal(t, "existing-id-123", capturedID)
	assert.Equal(t, "existing-id-123", rec.Header().Get("X-Request-ID"))
}
