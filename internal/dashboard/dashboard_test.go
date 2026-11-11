package dashboard

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeRepository struct {
	summary Summary
	err     error
	monthID int64
}

func (r *fakeRepository) Summary(_ context.Context, referenceMonthID int64) (Summary, error) {
	r.monthID = referenceMonthID
	return r.summary, r.err
}

func TestServiceGetsSummaryForSelectedReferenceMonth(t *testing.T) {
	repository := &fakeRepository{summary: Summary{Income: "0.00", Expenses: "123.45", Balance: "-123.45"}}
	got, err := NewService(repository).GetSummary(context.Background(), 9)
	if err != nil {
		t.Fatalf("GetSummary() error = %v", err)
	}
	if repository.monthID != 9 || !reflect.DeepEqual(got, repository.summary) {
		t.Fatalf("unexpected summary request: month=%d summary=%#v", repository.monthID, got)
	}
}

func TestServiceRejectsInvalidReferenceMonth(t *testing.T) {
	_, err := NewService(&fakeRepository{}).GetSummary(context.Background(), 0)
	if !errors.Is(err, ErrInvalidReferenceMonth) {
		t.Fatalf("expected ErrInvalidReferenceMonth, got %v", err)
	}
}
