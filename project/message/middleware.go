package message

import (
	"fmt"
	"time"

	"github.com/ThreeDotsLabs/go-event-driven/common/log"
	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/lithammer/shortuuid/v3"
	"github.com/sirupsen/logrus"
)

func useMiddlewares(router *message.Router, watermillLogger watermill.LoggerAdapter) {
	router.AddMiddleware(middleware.Recoverer)

	router.AddMiddleware(middleware.Retry{
		MaxRetries:      10,
		InitialInterval: time.Millisecond * 100,
		MaxInterval:     time.Second,
		Multiplier:      2,
		Logger:          watermillLogger,
	}.Middleware)

	router.AddMiddleware(correlationIDMiddleware)
	router.AddMiddleware(loggingMiddleware)
}

func loggingMiddleware(next message.HandlerFunc) message.HandlerFunc {
	return func(msg *message.Message) ([]*message.Message, error) {
		uuid := msg.UUID
		logger := log.FromContext(msg.Context())
		logger.WithFields(logrus.Fields{
			"message_uuid": uuid,
		}).Info("Handling a message")

		msgs, err := next(msg)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error":        err,
				"message_uuid": uuid,
			}).Error("Message handling error")

			return nil, err
		}

		return msgs, nil
	}
}

func correlationIDMiddleware(next message.HandlerFunc) message.HandlerFunc {
	return func(msg *message.Message) ([]*message.Message, error) {

		correlationId := msg.Metadata.Get("correlation_id")

		if correlationId == "" {
			correlationId = fmt.Sprintf("gen_%s", shortuuid.New())
		}

		ctx := log.ToContext(msg.Context(), logrus.WithField("correlation_id", correlationId))
		ctx = log.ContextWithCorrelationID(ctx, correlationId)

		msg.SetContext(ctx)
		return next(msg)
	}
}
