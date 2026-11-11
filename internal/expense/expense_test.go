package expense

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRepository struct {
	expense    Expense
	expenses   []Expense
	name       string
	amount     string
	updated    int64
	expenseID  int64
	categoryID int64
	source     string
	err        error
}

func (r *fakeRepository) CreateExpense(_ context.Context, name, amount string) (Expense, error) {
	r.name = name
	r.amount = amount
	if r.err != nil {
		return Expense{}, r.err
	}
	r.expense.Name = name
	r.expense.Valor = amount
	return r.expense, nil
}

func (r *fakeRepository) ListExpenses(context.Context) ([]Expense, error) {
	return r.expenses, r.err
}

func (r *fakeRepository) AssignCategory(_ context.Context, expenseID, categoryID int64, source string) (int64, error) {
	r.expenseID = expenseID
	r.categoryID = categoryID
	r.source = source
	return r.updated, r.err
}

func TestServiceCreatesExpenseWithNormalizedData(t *testing.T) {
	repository := &fakeRepository{expense: Expense{ID: 8}}
	created, err := NewService(repository).Create(context.Background(), "  Mercado  ", "1.234,5")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repository.name != "Mercado" || repository.amount != "1234.50" {
		t.Fatalf("unexpected repository values: name=%q amount=%q", repository.name, repository.amount)
	}
	if created.Name != "Mercado" || created.Valor != "1234.50" {
		t.Fatalf("unexpected created expense: %#v", created)
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
		_, err := NewService(&fakeRepository{}).Create(context.Background(), name, "10,00")
		if !errors.Is(err, ErrInvalidName) {
			t.Errorf("expected ErrInvalidName for length %d, got %v", len(name), err)
		}
	}
}

func TestServiceRejectsInvalidAmounts(t *testing.T) {
	tests := []string{"", "0", "0,00", "-1,00", "12.34", "1,234", "abc", "1.23,45"}
	for _, amount := range tests {
		_, err := NewService(&fakeRepository{}).Create(context.Background(), "Mercado", amount)
		if !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("expected ErrInvalidAmount for %q, got %v", amount, err)
		}
	}
}

func TestServicePropagatesRepositoryErrors(t *testing.T) {
	wantErr := errors.New("database unavailable")
	_, err := NewService(&fakeRepository{err: wantErr}).Create(context.Background(), "Mercado", "12,34")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected repository error %v, got %v", wantErr, err)
	}
}
