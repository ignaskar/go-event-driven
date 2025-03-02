package http

import (
	"github.com/ThreeDotsLabs/watermill/components/cqrs"
	"net/http"

	commonHTTP "github.com/ThreeDotsLabs/go-event-driven/common/http"
	"github.com/labstack/echo/v4"
)

func NewHttpRouter(eventBus *cqrs.EventBus) *echo.Echo {
	e := commonHTTP.NewEcho()

	e.GET("/health", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	handler := Handler{
		eventBus: eventBus,
	}

	e.POST("/tickets-status", handler.PostTicketsStatus)

	return e
}
