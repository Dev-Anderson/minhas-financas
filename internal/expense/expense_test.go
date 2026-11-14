package expense

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRepository struct {
	expense          Expense
	expenses         []Expense
	name             string
	amount           string
	referenceMonthID int64
	updated          int64
	updatedExpense   Expense
	deletedID        int64
	expenseID        int64
	categoryID       int64
	source           string
	err              error
}

func (r *fakeRepository) CreateExpense(_ context.Context, referenceMonthID int64, name, amount string) (Expense, error) {
	r.name = name
	r.amount = amount
	r.referenceMonthID = referenceMonthID
	if r.err != nil {
		return Expense{}, r.err
	}
	r.expense.Name = name
	r.expense.Valor = amount
	r.expense.ReferenceMonthID = referenceMonthID
	return r.expense, nil
}

func (r *fakeRepository) ListExpenses(context.Context, *int64) ([]Expense, error) {
	return r.expenses, r.err
}

func (r *fakeRepository) UpdateExpense(_ context.Context, expenseID, referenceMonthID int64, name, amount string) (Expense, error) {
	r.expenseID = expenseID
	r.referenceMonthID = referenceMonthID
	r.name = name
	r.amount = amount
	if r.err != nil {
		return Expense{}, r.err
	}
	r.updatedExpense = Expense{
		ID: expenseID, ReferenceMonthID: referenceMonthID, Name: name, Valor: amount, Source: SourceManual,
	}
	return r.updatedExpense, nil
}

func (r *fakeRepository) DeleteExpense(_ context.Context, expenseID int64) error {
	r.deletedID = expenseID
	return r.err
}

func (r *fakeRepository) AssignCategory(_ context.Context, expenseID, categoryID int64, source string) (int64, error) {
	r.expenseID = expenseID
	r.categoryID = categoryID
	r.source = source
	return r.updated, r.err
}

func TestServiceCreatesExpenseWithNormalizedData(t *testing.T) {
	repository := &fakeRepository{expense: Expense{ID: 8}}
	created, err := NewService(repository).Create(context.Background(), 2, "  Mercado  ", "1.234,5")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repository.referenceMonthID != 2 || repository.name != "Mercado" || repository.amount != "1234.50" {
		t.Fatalf("unexpected repository values: month=%d name=%q amount=%q", repository.referenceMonthID, repository.name, repository.amount)
	}
	if created.Name != "Mercado" || created.Valor != "1234.50" || created.ReferenceMonthID != 2 {
		t.Fatalf("unexpected created expense: %#v", created)
	}
}

func TestServiceUpdatesExpenseWithNormalizedData(t *testing.T) {
	repository := &fakeRepository{}
	updated, err := NewService(repository).Update(context.Background(), 8, 2, "  Mercado  ", "1.234,5")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if repository.expenseID != 8 || repository.referenceMonthID != 2 || repository.name != "Mercado" || repository.amount != "1234.50" {
		t.Fatalf("unexpected repository values: %#v", repository)
	}
	if updated.ID != 8 || updated.ReferenceMonthID != 2 || updated.Name != "Mercado" || updated.Valor != "1234.50" {
		t.Fatalf("unexpected updated expense: %#v", updated)
	}
}

func TestServiceDeletesExpense(t *testing.T) {
	repository := &fakeRepository{}
	if err := NewService(repository).Delete(context.Background(), 8); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if repository.deletedID != 8 {
		t.Fatalf("expected expense ID 8 to be deleted, got %d", repository.deletedID)
	}
}

func TestServiceAssignsCategoryToMatchingExpenseGroup(t *testing.T) {
	repository := &fakeRepository{updated: 2}
	updated, err := NewService(repository).AssignCategory(context.Background(), 8, 3, SourceManual)
	if err != nil {
		t.Fatalf("AssignCategory() error = %v", err)
	}
	if updated != 2 || repository.expenseID != 8 || repository.categoryID != 3 || repository.source != SourceManual {
		t.Fatalf("unexpected category assignment: updated=%d repository=%#v", updated, repository)
	}
}

func TestServiceRejectsInvalidCategoryAssignmentIDs(t *testing.T) {
	for _, ids := range [][2]int64{{0, 1}, {1, 0}} {
		_, err := NewService(&fakeRepository{}).AssignCategory(context.Background(), ids[0], ids[1], SourceManual)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound for IDs %v, got %v", ids, err)
		}
	}
}

func TestServiceRejectsInvalidCategoryAssignmentSource(t *testing.T) {
	_, err := NewService(&fakeRepository{}).AssignCategory(context.Background(), 1, 1, "unknown")
	if !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("expected ErrInvalidSource, got %v", err)
	}
}

func TestServiceRejectsInvalidNames(t *testing.T) {
	tests := []string{"   ", strings.Repeat("a", 151)}
	for _, name := range tests {
		_, err := NewService(&fakeRepository{}).Create(context.Background(), 1, name, "10,00")
		if !errors.Is(err, ErrInvalidName) {
			t.Errorf("expected ErrInvalidName for length %d, got %v", len(name), err)
		}
	}
}

func TestServiceRejectsInvalidAmounts(t *testing.T) {
	tests := []string{"", "0", "0,00", "-1,00", "12.34", "1,234", "abc", "1.23,45"}
	for _, amount := range tests {
		_, err := NewService(&fakeRepository{}).Create(context.Background(), 1, "Mercado", amount)
		if !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("expected ErrInvalidAmount for %q, got %v", amount, err)
		}
	}
}

func TestServicePropagatesRepositoryErrors(t *testing.T) {
	wantErr := errors.New("database unavailable")
	_, err := NewService(&fakeRepository{err: wantErr}).Create(context.Background(), 1, "Mercado", "12,34")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected repository error %v, got %v", wantErr, err)
	}
}

func TestServiceRequiresReferenceMonth(t *testing.T) {
	for _, referenceMonthID := range []int64{0, -1} {
		_, err := NewService(&fakeRepository{}).Create(context.Background(), referenceMonthID, "Mercado", "12,34")
		if !errors.Is(err, ErrInvalidReferenceMonth) {
			t.Errorf("expected ErrInvalidReferenceMonth for ID %d, got %v", referenceMonthID, err)
		}
	}
}

func TestServiceRejectsInvalidUpdateData(t *testing.T) {
	tests := []struct {
		name             string
		expenseID        int64
		referenceMonthID int64
		expenseName      string
		amount           string
		wantErr          error
	}{
		{name: "invalid expense ID", expenseID: 0, referenceMonthID: 1, expenseName: "Mercado", amount: "12,34", wantErr: ErrNotFound},
		{name: "invalid month ID", expenseID: 1, referenceMonthID: 0, expenseName: "Mercado", amount: "12,34", wantErr: ErrInvalidReferenceMonth},
		{name: "invalid name", expenseID: 1, referenceMonthID: 1, expenseName: " ", amount: "12,34", wantErr: ErrInvalidName},
		{name: "invalid amount", expenseID: 1, referenceMonthID: 1, expenseName: "Mercado", amount: "-1,00", wantErr: ErrInvalidAmount},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewService(&fakeRepository{}).Update(
				context.Background(), test.expenseID, test.referenceMonthID, test.expenseName, test.amount,
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("expected error %v, got %v", test.wantErr, err)
			}
		})
	}
}

func TestServiceRejectsInvalidDeleteID(t *testing.T) {
	if err := NewService(&fakeRepository{}).Delete(context.Background(), 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
