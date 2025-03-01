package api

import (
	"context"
	"sync"
)

type SpreadsheetsMock struct {
	mu   sync.Mutex
	Rows map[string][][]string
}

func (s *SpreadsheetsMock) AppendRow(ctx context.Context, spreadsheetName string, row []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Rows == nil {
		s.Rows = make(map[string][][]string)
	}

	s.Rows[spreadsheetName] = append(s.Rows[spreadsheetName], row)

	return nil
}
