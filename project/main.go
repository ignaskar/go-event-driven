package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/ThreeDotsLabs/go-event-driven/common/clients"
	"github.com/ThreeDotsLabs/go-event-driven/common/clients/receipts"
	"github.com/ThreeDotsLabs/go-event-driven/common/clients/spreadsheets"
	commonHTTP "github.com/ThreeDotsLabs/go-event-driven/common/http"
	"github.com/ThreeDotsLabs/go-event-driven/common/log"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

type TicketsConfirmationRequest struct {
	Tickets []string `json:"tickets"`
}

func main() {
	log.Init(logrus.InfoLevel)

	e := commonHTTP.NewEcho()

	w := NewWorker()
	go w.Run()

	e.POST("/tickets-confirmation", func(c echo.Context) error {
		var request TicketsConfirmationRequest
		err := c.Bind(&request)
		if err != nil {
			return err
		}

		for _, ticket := range request.Tickets {
			receiptMsg := Message{
				Task:     TaskIssueReceipt,
				TicketID: ticket,
			}

			trackerMsg := Message{
				Task:     TaskAppendToTracker,
				TicketID: ticket,
			}

			w.Send([]Message{receiptMsg, trackerMsg}...)
		}

		return c.NoContent(http.StatusOK)
	})

	logrus.Info("Server starting...")

	err := e.Start(":8080")
	if err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}

type Task int

const (
	TaskIssueReceipt Task = iota
	TaskAppendToTracker
)

type Message struct {
	Task     Task
	TicketID string
}

type Worker struct {
	queue chan Message
}

func NewWorker() *Worker {
	return &Worker{
		queue: make(chan Message, 100),
	}
}

func (w *Worker) Send(msgs ...Message) {
	for _, msg := range msgs {
		w.queue <- msg
	}
}

func (w *Worker) Run() {
	clients, err := clients.NewClients(os.Getenv("GATEWAY_ADDR"), nil)
	if err != nil {
		panic(err)
	}

	receiptsClient := NewReceiptsClient(clients)
	spreadsheetsClient := NewSpreadsheetsClient(clients)

	ctx := context.Background()

	for msg := range w.queue {
		switch msg.Task {
		case TaskIssueReceipt:
			err = receiptsClient.IssueReceipt(ctx, msg.TicketID)
			if err != nil {
				logrus.WithError(err).Errorf("failed to issue the receipt")
				w.Send(msg)
			}
		case TaskAppendToTracker:
			err = spreadsheetsClient.AppendRow(ctx, "tickets-to-print", []string{msg.TicketID})
			if err != nil {
				logrus.WithError(err).Errorf("failed to append to tracker")
				w.Send(msg)
			}
		}
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

func (c ReceiptsClient) IssueReceipt(ctx context.Context, ticketID string) error {
	body := receipts.PutReceiptsJSONRequestBody{
		TicketId: ticketID,
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
