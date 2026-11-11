package nubank

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestParseCSV(t *testing.T) {
	input := "\ufeffdate,title,amount\r\n" +
		"2026-01-31,Example,\"1.234,56\"\r\n" +
		"2026-02-01,Another example,\"- 12,34\"\r\n"

	transactions, err := ParseCSV([]byte(input))
	if err != nil {
		t.Fatalf("ParseCSV() error = %v", err)
	}
	if len(transactions) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(transactions))
	}
	if transactions[0].Amount != "1234.56" || transactions[0].Date.Format("2006-01-02") != "2026-01-31" {
		t.Fatalf("unexpected first transaction date or amount: %#v", transactions[0])
	}
	if transactions[1].Amount != "-12.34" || transactions[1].Date.Format("2006-01-02") != "2026-02-01" {
		t.Fatalf("unexpected second transaction date or amount: %#v", transactions[1])
	}
}

func TestParseCSVRejectsInvalidRows(t *testing.T) {
	tests := []struct {
		name string
		csv  string
	}{
		{name: "unexpected header", csv: "date,title,value\n"},
		{name: "invalid date", csv: "date,title,amount\n2026/01/01,Example,\"1,00\"\n"},
		{name: "invalid amount", csv: "date,title,amount\n2026-01-01,Example,invalid\n"},
		{name: "empty title", csv: "date,title,amount\n2026-01-01, ,\"1,00\"\n"},
		{name: "empty file", csv: "date,title,amount\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseCSV([]byte(test.csv))
			if err == nil {
				t.Fatal("expected invalid CSV error")
			}
		})
	}
}

func TestImporterReturnsValidationError(t *testing.T) {
	importer := NewImporter(fakeRepository{})
	_, err := importer.ImportCSV(t.Context(), []byte("date,title,amount\n"), 1)
	if err == nil || !strings.Contains(err.Error(), "no transactions") {
		t.Fatalf("expected no-transactions validation error, got %v", err)
	}
}

func TestImporterRequiresReferenceMonth(t *testing.T) {
	importer := NewImporter(fakeRepository{})
	_, err := importer.ImportCSV(t.Context(), []byte("date,title,amount\n"), 0)
	if !errors.Is(err, ErrInvalidReferenceMonth) {
		t.Fatalf("expected ErrInvalidReferenceMonth, got %v", err)
	}
}

func TestImporterAllowsRepeatedImports(t *testing.T) {
	repository := &recordingRepository{}
	importer := NewImporter(repository)
	csv := []byte("date,title,amount\n2026-01-02,Example,\"12,34\"\n")

	for range 2 {
		if _, err := importer.ImportCSV(t.Context(), csv, 7); err != nil {
			t.Fatalf("ImportCSV() error = %v", err)
		}
	}

	if repository.calls != 2 || repository.referenceMonthID != 7 || len(repository.transactions) != 1 {
		t.Fatalf("unexpected repeated import calls: %#v", repository)
	}
}

type fakeRepository struct{}

func (fakeRepository) Import(_ context.Context, _ int64, _ []Transaction) error {
	return nil
}

type recordingRepository struct {
	calls            int
	referenceMonthID int64
	transactions     []Transaction
}

func (r *recordingRepository) Import(_ context.Context, referenceMonthID int64, transactions []Transaction) error {
	r.calls++
	r.referenceMonthID = referenceMonthID
	r.transactions = transactions
	return nil
}
