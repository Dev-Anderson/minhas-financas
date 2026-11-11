package income

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRepository struct {
	income           Income
	referenceMonthID int64
	name             string
	amount           string
	err              error
}

func (r *fakeRepository) CreateIncome(_ context.Context, referenceMonthID int64, name, amount string) (Income, error) {
	r.referenceMonthID = referenceMonthID
	r.name = name
	r.amount = amount
	if r.err != nil {
		return Income{}, r.err
	}
	r.income.ReferenceMonthID = referenceMonthID
	r.income.Name = name
	r.income.Amount = amount
	return r.income, nil
}

func TestServiceCreatesIncomeWithNormalizedData(t *testing.T) {
	repository := &fakeRepository{income: Income{ID: 1}}
	created, err := NewService(repository).Create(context.Background(), 7, " Salário ", "1.234,5")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repository.referenceMonthID != 7 || repository.name != "Salário" || repository.amount != "1234.50" {
		t.Fatalf("unexpected repository input: %#v", repository)
	}
	if created.ReferenceMonthID != 7 || created.Amount != "1234.50" {
		t.Fatalf("unexpected created income: %#v", created)
	}
}

func TestServiceRejectsInvalidIncomeFields(t *testing.T) {
	tests := []struct {
		name          string
		monthID       int64
		incomeName    string
		amount        string
		expectedError error
	}{
		{name: "missing month", incomeName: "Salary", amount: "10,00", expectedError: ErrInvalidReferenceMonth},
		{name: "empty name", monthID: 1, amount: "10,00", expectedError: ErrInvalidName},
		{name: "long name", monthID: 1, incomeName: strings.Repeat("a", 151), amount: "10,00", expectedError: ErrInvalidName},
		{name: "invalid amount", monthID: 1, incomeName: "Salary", amount: "-10,00", expectedError: ErrInvalidAmount},
		{name: "zero amount", monthID: 1, incomeName: "Salary", amount: "0,00", expectedError: ErrInvalidAmount},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewService(&fakeRepository{}).Create(context.Background(), test.monthID, test.incomeName, test.amount)
			if !errors.Is(err, test.expectedError) {
				t.Fatalf("expected %v, got %v", test.expectedError, err)
			}
		})
	}
}
