package dashboard

import (
	"context"
	"errors"
)

var ErrInvalidReferenceMonth = errors.New("a valid reference month must be selected")

type Summary struct {
	Income        string          `json:"income"`
	Expenses      string          `json:"expenses"`
	Balance       string          `json:"balance"`
	ExpenseCount  int             `json:"expenseCount"`
	CategoryCount int             `json:"categoryCount"`
	Categories    []CategoryGroup `json:"categories"`
}

type CategoryGroup struct {
	Name     string        `json:"name"`
	Total    string        `json:"total"`
	Expenses []ExpenseLine `json:"expenses"`
}

type ExpenseLine struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Amount string `json:"amount"`
	Source string `json:"source"`
}

type Repository interface {
	Summary(context.Context, int64) (Summary, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) GetSummary(ctx context.Context, referenceMonthID int64) (Summary, error) {
	if referenceMonthID < 1 {
		return Summary{}, ErrInvalidReferenceMonth
	}
	return s.repository.Summary(ctx, referenceMonthID)
}
