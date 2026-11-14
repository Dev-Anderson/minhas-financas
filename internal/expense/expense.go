package expense

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidName            = errors.New("expense name must contain between 1 and 150 characters")
	ErrInvalidAmount          = errors.New("expense amount must be a positive value in Brazilian currency format")
	ErrInvalidReferenceMonth  = errors.New("a valid reference month must be selected")
	ErrReferenceMonthNotFound = errors.New("reference month not found")
	ErrNotFound               = errors.New("expense not found")
	ErrCategoryNotFound       = errors.New("category not found")
	ErrInvalidSource          = errors.New("expense source must be manual or nubank")
)

const (
	SourceManual = "manual"
	SourceNubank = "nubank"
)

var amountPattern = regexp.MustCompile(`^(?:[0-9]+|[0-9]{1,3}(?:\.[0-9]{3})+)(?:,[0-9]{1,2})?$`)

type Expense struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Valor            string `json:"valor"`
	ReferenceMonthID int64  `json:"referenceMonthId,omitempty"`
	CategoryID       *int64 `json:"categoryId"`
	Source           string `json:"source"`
}

type Repository interface {
	CreateExpense(context.Context, int64, string, string) (Expense, error)
	ListExpenses(context.Context, *int64) ([]Expense, error)
	UpdateExpense(context.Context, int64, int64, string, string) (Expense, error)
	DeleteExpense(context.Context, int64) error
	AssignCategory(context.Context, int64, int64, string) (int64, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Create(ctx context.Context, referenceMonthID int64, name, amount string) (Expense, error) {
	if referenceMonthID < 1 {
		return Expense{}, ErrInvalidReferenceMonth
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 150 {
		return Expense{}, ErrInvalidName
	}

	normalizedAmount, err := normalizeAmount(amount)
	if err != nil {
		return Expense{}, err
	}
	return s.repository.CreateExpense(ctx, referenceMonthID, name, normalizedAmount)
}

func (s *Service) List(ctx context.Context, referenceMonthID *int64) ([]Expense, error) {
	if referenceMonthID != nil && *referenceMonthID < 1 {
		return nil, ErrInvalidReferenceMonth
	}
	return s.repository.ListExpenses(ctx, referenceMonthID)
}

func (s *Service) Update(ctx context.Context, expenseID, referenceMonthID int64, name, amount string) (Expense, error) {
	if expenseID < 1 {
		return Expense{}, ErrNotFound
	}
	if referenceMonthID < 1 {
		return Expense{}, ErrInvalidReferenceMonth
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 150 {
		return Expense{}, ErrInvalidName
	}
	normalizedAmount, err := normalizeAmount(amount)
	if err != nil {
		return Expense{}, err
	}
	return s.repository.UpdateExpense(ctx, expenseID, referenceMonthID, name, normalizedAmount)
}

func (s *Service) Delete(ctx context.Context, expenseID int64) error {
	if expenseID < 1 {
		return ErrNotFound
	}
	return s.repository.DeleteExpense(ctx, expenseID)
}

func (s *Service) AssignCategory(ctx context.Context, expenseID, categoryID int64, source string) (int64, error) {
	if expenseID < 1 || categoryID < 1 {
		return 0, ErrNotFound
	}
	if source != SourceManual && source != SourceNubank {
		return 0, ErrInvalidSource
	}
	return s.repository.AssignCategory(ctx, expenseID, categoryID, source)
}

func normalizeAmount(amount string) (string, error) {
	amount = strings.ReplaceAll(strings.TrimSpace(amount), " ", "")
	if !amountPattern.MatchString(amount) {
		return "", ErrInvalidAmount
	}

	normalized := strings.ReplaceAll(amount, ".", "")
	normalized = strings.ReplaceAll(normalized, ",", ".")
	value, err := strconv.ParseFloat(normalized, 64)
	if err != nil || value <= 0 {
		return "", ErrInvalidAmount
	}
	if !strings.Contains(normalized, ".") {
		normalized += ".00"
	} else if fractionLength := len(normalized) - strings.LastIndex(normalized, ".") - 1; fractionLength == 1 {
		normalized += "0"
	}
	return normalized, nil
}
