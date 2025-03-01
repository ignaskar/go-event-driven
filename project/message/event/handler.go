package event

import (
	"context"
	"tickets/entities"
)

type Handler struct {
	spreadsheetsService SpreadsheetsService
	receiptsService     ReceiptsService
}

func NewHandler(
	spreadsheetsService SpreadsheetsService,
	receiptsService ReceiptsService,
) Handler {
	if spreadsheetsService == nil {
		panic("missing spreadsheetsService")
	}

	if receiptsService == nil {
		panic("missing receiptsService")
	}

	return Handler{
		receiptsService:     receiptsService,
		spreadsheetsService: spreadsheetsService,
	}
}

type SpreadsheetsService interface {
	AppendRow(ctx context.Context, sheetName string, row []string) error
}

type ReceiptsService interface {
	IssueReceipt(ctx context.Context, request entities.IssueReceiptRequest) (entities.IssueReceiptResponse, error)
}
