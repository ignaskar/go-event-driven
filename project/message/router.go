package message

import (
	"encoding/json"
	"tickets/entities"
	"tickets/message/event"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/redis/go-redis/v9"
)

const brokenMessageID = "2beaf5bc-d5e4-4653-b075-2b36bbf28949"

func NewWatermillRouter(receiptsService event.ReceiptsService, spreadsheetsService event.SpreadsheetsService, rdb *redis.Client, watermillLogger watermill.LoggerAdapter) *message.Router {
	router, err := message.NewRouter(message.RouterConfig{}, watermillLogger)
	if err != nil {
		panic(err)
	}

	handler := event.NewHandler(spreadsheetsService, receiptsService)

	useMiddlewares(router, watermillLogger)

	issueReceiptSub, err := redisstream.NewSubscriber(redisstream.SubscriberConfig{
		Client:        rdb,
		ConsumerGroup: "issue-receipts",
	}, watermillLogger)
	if err != nil {
		panic(err)
	}

	appendToTrackerSub, err := redisstream.NewSubscriber(redisstream.SubscriberConfig{
		Client:        rdb,
		ConsumerGroup: "append-to-tracker",
	}, watermillLogger)
	if err != nil {
		panic(err)
	}

	cancelTicketSub, err := redisstream.NewSubscriber(redisstream.SubscriberConfig{
		Client:        rdb,
		ConsumerGroup: "refund-ticket",
	}, watermillLogger)
	if err != nil {
		panic(err)
	}

	router.AddNoPublisherHandler(
		"issue-receipt-handler",
		"TicketBookingConfirmed",
		issueReceiptSub,
		func(msg *message.Message) error {
			if msg.UUID == brokenMessageID {
				return nil
			}

			if msg.Metadata.Get("type") != "TicketBookingConfirmed" {
				return nil
			}

			var p entities.TicketBookingConfirmed
			err := json.Unmarshal(msg.Payload, &p)
			if err != nil {
				return err
			}

			// Fixing a code bug: for some events, we didn't supply the currency, which was USD by default
			// Now some events are spinning
			// Add this if to default to USD for these events
			if p.Price.Currency == "" {
				p.Price.Currency = "USD"
			}

			return handler.IssueReceipt(msg.Context(), p)
		},
	)

	router.AddNoPublisherHandler(
		"append-to-tracker-handler",
		"TicketBookingConfirmed",
		appendToTrackerSub,
		func(msg *message.Message) error {
			if msg.UUID == brokenMessageID {
				return nil
			}

			if msg.Metadata.Get("type") != "TicketBookingConfirmed" {
				return nil
			}

			var p entities.TicketBookingConfirmed
			err := json.Unmarshal(msg.Payload, &p)
			if err != nil {
				return err
			}

			// Fixing a code bug: for some events, we didn't supply the currency, which was USD by default
			// Now some events are spinning
			// Add this if to default to USD for these events
			if p.Price.Currency == "" {
				p.Price.Currency = "USD"
			}

			return handler.AppendToTracker(msg.Context(), p)
		},
	)

	router.AddNoPublisherHandler(
		"cancel-ticket-handler",
		"TicketBookingCanceled",
		cancelTicketSub,
		func(msg *message.Message) error {
			if msg.Metadata.Get("type") != "TicketBookingCanceled" {
				return nil
			}

			var p entities.TicketBookingCanceled
			err := json.Unmarshal(msg.Payload, &p)
			if err != nil {
				return err
			}

			// Fixing a code bug: for some events, we didn't supply the currency, which was USD by default
			// Now some events are spinning
			// Add this if to default to USD for these events
			if p.Price.Currency == "" {
				p.Price.Currency = "USD"
			}

			return handler.CancelTicket(msg.Context(), p)
		},
	)

	return router
}
