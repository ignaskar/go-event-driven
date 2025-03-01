package tests_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/lithammer/shortuuid/v3"
	"net/http"
	"os"
	"testing"
	"tickets/api"
	"tickets/entities"
	"tickets/message"
	"tickets/service"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponent(t *testing.T) {
	// place for your tests!
	redisClient := message.NewRedisClient(os.Getenv("REDIS_ADDR"))
	defer redisClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	spreadsheetsService := &api.SpreadsheetsMock{}
	receiptsService := &api.ReceiptsMock{}

	go func() {
		svc := service.New(
			redisClient,
			spreadsheetsService,
			receiptsService,
		)
		assert.NoError(t, svc.Run(ctx))
	}()

	waitForHttpServer(t)
	testTicketsStatusConfirmed(t, receiptsService, spreadsheetsService)
	testTicketsStatusCanceled(t, spreadsheetsService)
}

func testTicketsStatusCanceled(t *testing.T, spreadsheetsService *api.SpreadsheetsMock) {
	ticket := getTestTicket("canceled")

	req := entities.TicketsStatusRequest{Tickets: []entities.Ticket{ticket}}

	sendTicketsStatus(t, req)
	assertRowsToSpreadsheetForTicketAppended(t, spreadsheetsService, ticket, "tickets-to-refund")
}

func testTicketsStatusConfirmed(t *testing.T, receiptsService *api.ReceiptsMock, spreadsheetsService *api.SpreadsheetsMock) {
	ticket := getTestTicket("confirmed")

	req := entities.TicketsStatusRequest{Tickets: []entities.Ticket{ticket}}

	sendTicketsStatus(t, req)
	assertReceiptForTicketIssued(t, receiptsService, ticket)
	assertRowsToSpreadsheetForTicketAppended(t, spreadsheetsService, ticket, "tickets-to-print")
}

func assertReceiptForTicketIssued(t *testing.T, receiptsService *api.ReceiptsMock, ticket entities.Ticket) {
	assert.EventuallyWithT(
		t,
		func(collectT *assert.CollectT) {
			issuedReceipts := len(receiptsService.IssuedReceipts)
			t.Log("issued receipts", issuedReceipts)

			assert.Greater(collectT, issuedReceipts, 0, "no receipts issued")
		},
		10*time.Second,
		100*time.Millisecond,
	)

	var receipt entities.IssueReceiptRequest
	var ok bool
	for _, issuedReceipt := range receiptsService.IssuedReceipts {
		if issuedReceipt.TicketID != ticket.TicketID {
			continue
		}
		receipt = issuedReceipt
		ok = true
		break
	}
	require.Truef(t, ok, "receipt for ticket %s is not found", ticket.TicketID)

	assert.Equal(t, ticket.TicketID, receipt.TicketID)
	assert.Equal(t, ticket.Price.Amount, receipt.Price.Amount)
	assert.Equal(t, ticket.Price.Currency, receipt.Price.Currency)
}

func assertRowsToSpreadsheetForTicketAppended(t *testing.T, spreadsheetsService *api.SpreadsheetsMock, ticket entities.Ticket, sheetName string) {
	assert.EventuallyWithT(
		t,
		func(collectT *assert.CollectT) {
			rows, ok := spreadsheetsService.Rows[sheetName]
			if !assert.True(t, ok, "sheet %s not found") {
			}

			var allValues []string

			for _, row := range rows {
				for _, col := range row {
					allValues = append(allValues, col)
				}
			}

			assert.Contains(t, allValues, ticket.TicketID, "ticket %s not found in sheet %s", ticket.TicketID, sheetName)
		},
		10*time.Second,
		100*time.Millisecond,
	)
}

func sendTicketsStatus(t *testing.T, req entities.TicketsStatusRequest) {
	t.Helper()

	payload, err := json.Marshal(req)
	require.NoError(t, err)

	correlationId := shortuuid.New()

	ticketIDs := make([]string, 0, len(req.Tickets))
	for _, ticket := range req.Tickets {
		ticketIDs = append(ticketIDs, ticket.TicketID)
	}

	httpReq, err := http.NewRequest(
		http.MethodPost,
		"http://localhost:8080/tickets-status",
		bytes.NewBuffer(payload),
	)
	require.NoError(t, err)

	httpReq.Header.Set("Correlation-ID", correlationId)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func getTestTicket(status string) entities.Ticket {
	return entities.Ticket{
		TicketID:      uuid.NewString(),
		Status:        status,
		CustomerEmail: "test@test.com",
		Price: entities.Money{
			Amount:   "50",
			Currency: "USD",
		},
	}
}

func waitForHttpServer(t *testing.T) {
	t.Helper()

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			resp, err := http.Get("http://localhost:8080/health")
			if !assert.NoError(t, err) {
				return
			}
			defer resp.Body.Close()

			if assert.Less(t, resp.StatusCode, 300, "API not ready, http status: %d", resp.StatusCode) {
				return
			}
		},
		time.Second*10,
		time.Millisecond*50,
	)
}
