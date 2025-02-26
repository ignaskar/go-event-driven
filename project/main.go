package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/ThreeDotsLabs/go-event-driven/common/clients"
	"github.com/ThreeDotsLabs/go-event-driven/common/clients/receipts"
	"github.com/ThreeDotsLabs/go-event-driven/common/clients/spreadsheets"
	commonHTTP "github.com/ThreeDotsLabs/go-event-driven/common/http"
	"github.com/ThreeDotsLabs/go-event-driven/common/log"
	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/lithammer/shortuuid/v3"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type EventHeader struct {
	ID          string `json:"id"`
	PublishedAt string `json:"published_at"`
}

func NewHeader() EventHeader {
	return EventHeader{
		ID:          uuid.NewString(),
		PublishedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

type TicketBookingConfirmed struct {
	Header        EventHeader `json:"header"`
	TicketID      string      `json:"ticket_id"`
	CustomerEmail string      `json:"customer_email"`
	Price         Money       `json:"price"`
}

type TicketBookingCanceled struct {
	Header        EventHeader `json:"header"`
	TicketID      string      `json:"ticket_id"`
	CustomerEmail string      `json:"customer_email"`
	Price         Money       `json:"price"`
}

type Money struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type TicketStatus struct {
	TicketID      string `json:"ticket_id"`
	Status        string `json:"status"`
	CustomerEmail string `json:"customer_email"`
	Price         Money  `json:"price"`
}

type TicketsStatusRequest struct {
	Tickets []TicketStatus `json:"tickets"`
}

type IssueReceiptRequest struct {
	TicketID string
	Price    Money
}

func main() {
	log.Init(logrus.InfoLevel)

	e := commonHTTP.NewEcho()

	clients, err := clients.NewClients(
		os.Getenv("GATEWAY_ADDR"),
		func(ctx context.Context, req *http.Request) error {
			req.Header.Set("Correlation-ID", log.CorrelationIDFromContext(ctx))
			return nil
		},
	)
	if err != nil {
		panic(err)
	}

	receiptsClient := NewReceiptsClient(clients)
	spreadsheetsClient := NewSpreadsheetsClient(clients)

	logger := log.NewWatermill(logrus.NewEntry(logrus.StandardLogger()))

	rdb := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_ADDR"),
	})

	router, err := message.NewRouter(message.RouterConfig{}, logger)
	if err != nil {
		panic(err)
	}

	publisher, err := redisstream.NewPublisher(redisstream.PublisherConfig{
		Client: rdb,
	}, logger)
	if err != nil {
		panic(err)
	}

	receipt_subscriber, err := redisstream.NewSubscriber(redisstream.SubscriberConfig{
		Client:        rdb,
		ConsumerGroup: "issue-receipts",
	}, logger)
	if err != nil {
		panic(err)
	}

	tracker_subscriber, err := redisstream.NewSubscriber(redisstream.SubscriberConfig{
		Client:        rdb,
		ConsumerGroup: "append-to-tracker",
	}, logger)
	if err != nil {
		panic(err)
	}

	refund_subscriber, err := redisstream.NewSubscriber(redisstream.SubscriberConfig{
		Client:        rdb,
		ConsumerGroup: "refund-ticket",
	}, logger)
	if err != nil {
		panic(err)
	}

	router.AddMiddleware(CorrelationIDMiddleware)
	router.AddMiddleware(LoggingMiddleware)

	router.AddNoPublisherHandler(
		"issue-receipt-handler",
		"TicketBookingConfirmed",
		receipt_subscriber,
		func(msg *message.Message) error {
			var p TicketBookingConfirmed
			err := json.Unmarshal(msg.Payload, &p)
			if err != nil {
				return err
			}

			r := IssueReceiptRequest{
				TicketID: p.TicketID,
				Price: Money{
					Amount:   p.Price.Amount,
					Currency: p.Price.Currency,
				},
			}
			err = receiptsClient.IssueReceipt(msg.Context(), r)
			if err != nil {
				return err
			}

			return nil
		},
	)

	router.AddNoPublisherHandler(
		"append-to-tracker-handler",
		"TicketBookingConfirmed",
		tracker_subscriber,
		func(msg *message.Message) error {
			var p TicketBookingConfirmed
			err := json.Unmarshal(msg.Payload, &p)
			if err != nil {
				return err
			}

			err = spreadsheetsClient.AppendRow(msg.Context(), "tickets-to-print", []string{p.TicketID, p.CustomerEmail, p.Price.Amount, p.Price.Currency})
			if err != nil {
				return err
			}

			return nil
		},
	)

	router.AddNoPublisherHandler(
		"cancel-ticket-handler",
		"TicketBookingCanceled",
		refund_subscriber,
		func(msg *message.Message) error {
			var p TicketBookingCanceled
			err := json.Unmarshal(msg.Payload, &p)
			if err != nil {
				return err
			}

			err = spreadsheetsClient.AppendRow(msg.Context(), "tickets-to-refund", []string{p.TicketID, p.CustomerEmail, p.Price.Amount, p.Price.Currency})
			if err != nil {
				return err
			}

			return nil
		},
	)

	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return router.Run(ctx)
	})

	e.POST("/tickets-status", func(c echo.Context) error {
		var request TicketsStatusRequest
		err := c.Bind(&request)
		if err != nil {
			return err
		}

		correlation_id := c.Request().Header.Get("Correlation-ID")

		for _, ticket := range request.Tickets {
			if ticket.Status == "confirmed" {
				p := TicketBookingConfirmed{
					Header:        NewHeader(),
					TicketID:      ticket.TicketID,
					CustomerEmail: ticket.CustomerEmail,
					Price: Money{
						Amount:   ticket.Price.Amount,
						Currency: ticket.Price.Currency,
					},
				}

				evt, err := json.Marshal(p)
				if err != nil {
					return err
				}

				m := message.NewMessage(watermill.NewShortUUID(), evt)
				m.Metadata.Set("correlation_id", correlation_id)
				err = publisher.Publish("TicketBookingConfirmed", m)
				if err != nil {
					return err
				}

			} else if ticket.Status == "canceled" {
				p := TicketBookingCanceled{
					Header:        NewHeader(),
					TicketID:      ticket.TicketID,
					CustomerEmail: ticket.CustomerEmail,
					Price: Money{
						Amount:   ticket.Price.Amount,
						Currency: ticket.Price.Currency,
					},
				}

				evt, err := json.Marshal(p)
				if err != nil {
					return err
				}

				m := message.NewMessage(watermill.NewShortUUID(), evt)
				m.Metadata.Set("correlation_id", correlation_id)
				err = publisher.Publish("TicketBookingCanceled", m)
				if err != nil {
					return err
				}
			} else {
				return fmt.Errorf("unknown ticket status: %s", ticket.Status)
			}
		}

		return c.NoContent(http.StatusOK)
	})

	e.GET("/health", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	logrus.Info("Server starting...")
	g.Go(func() error {
		// we do not want to start HTTP server before router
		<-router.Running()

		err = e.Start(":8080")
		if err != nil && err != http.ErrServerClosed {
			return err
		}

		return nil
	})

	g.Go(func() error {
		<-ctx.Done()
		return e.Shutdown(ctx)
	})

	err = g.Wait()
	if err != nil {
		panic(err)
	}
}

type ReceiptsClient struct {
	clients *clients.Clients
}

func NewReceiptsClient(clients *clients.Clients) ReceiptsClient {
	return ReceiptsClient{
		clients: clients,
	}
}

func (c ReceiptsClient) IssueReceipt(ctx context.Context, request IssueReceiptRequest) error {
	body := receipts.PutReceiptsJSONRequestBody{
		TicketId: request.TicketID,
		Price: receipts.Money{
			MoneyAmount:   request.Price.Amount,
			MoneyCurrency: request.Price.Currency,
		},
	}

	receiptsResp, err := c.clients.Receipts.PutReceiptsWithResponse(ctx, body)
	if err != nil {
		return err
	}
	if receiptsResp.StatusCode() != http.StatusOK {
		return fmt.Errorf("unexpected status code: %v", receiptsResp.StatusCode())
	}

	return nil
}

type SpreadsheetsClient struct {
	clients *clients.Clients
}

func NewSpreadsheetsClient(clients *clients.Clients) SpreadsheetsClient {
	return SpreadsheetsClient{
		clients: clients,
	}
}

func (c SpreadsheetsClient) AppendRow(ctx context.Context, spreadsheetName string, row []string) error {
	request := spreadsheets.PostSheetsSheetRowsJSONRequestBody{
		Columns: row,
	}

	sheetsResp, err := c.clients.Spreadsheets.PostSheetsSheetRowsWithResponse(ctx, spreadsheetName, request)
	if err != nil {
		return err
	}
	if sheetsResp.StatusCode() != http.StatusOK {
		return fmt.Errorf("unexpected status code: %v", sheetsResp.StatusCode())
	}

	return nil
}

func LoggingMiddleware(next message.HandlerFunc) message.HandlerFunc {
	return func(msg *message.Message) ([]*message.Message, error) {
		uuid := msg.UUID
		logger := log.FromContext(msg.Context())
		logger.WithField("message_uuid", uuid).Info("Handling a message")
		return next(msg)
	}
}

func CorrelationIDMiddleware(next message.HandlerFunc) message.HandlerFunc {
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
