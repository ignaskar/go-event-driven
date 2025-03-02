package message

import (
	"github.com/ThreeDotsLabs/watermill/components/cqrs"
	"tickets/message/event"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
)

func NewWatermillRouter(handler event.Handler, processorConfig cqrs.EventProcessorConfig, watermillLogger watermill.LoggerAdapter) *message.Router {
	router, err := message.NewRouter(message.RouterConfig{}, watermillLogger)
	if err != nil {
		panic(err)
	}

	useMiddlewares(router, watermillLogger)

	eventProcessor, err := cqrs.NewEventProcessorWithConfig(router, processorConfig)
	if err != nil {
		panic(err)
	}

	err = eventProcessor.AddHandlers(
		cqrs.NewEventHandler("AppendToTracker", handler.AppendToTracker),
		cqrs.NewEventHandler("IssueReceipt", handler.IssueReceipt),
		cqrs.NewEventHandler("CancelTicket", handler.CancelTicket),
	)
	if err != nil {
		panic(err)
	}

	return router
}
