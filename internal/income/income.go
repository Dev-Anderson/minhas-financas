package income

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidName            = errors.New("income name must contain between 1 and 150 characters")
	ErrInvalidAmount          = errors.New("income amount must be a positive value in Brazilian currency format")
	ErrInvalidReferenceMonth  = errors.New("a valid reference month must be selected")
	ErrReferenceMonthNotFound = errors.New("reference month not found")
)

var amountPattern = regexp.MustCompile(`^(?:[0-9]+|[0-9]{1,3}(?:\.[0-9]{3})+)(?:,[0-9]{1,2})?$`)

type Income struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Amount           string `json:"amount"`
	ReferenceMonthID int64  `json:"referenceMonthId"`
}

type Repository interface {
	CreateIncome(context.Context, int64, string, string) (Income, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Create(ctx context.Context, referenceMonthID int64, name, amount string) (Income, error) {
	if referenceMonthID < 1 {
		return Income{}, ErrInvalidReferenceMonth
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 150 {
		return Income{}, ErrInvalidName
	}
	normalizedAmount, err := normalizeAmount(amount)
	if err != nil {
		return Income{}, err
	}
	return s.repository.CreateIncome(ctx, referenceMonthID, name, normalizedAmount)
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
