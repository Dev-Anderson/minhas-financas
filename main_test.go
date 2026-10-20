package main

import (
	"testing"
	"time"
)

func TestFindExpenseCategoryPrefersMostSpecificRule(t *testing.T) {
	rules := []expenseRule{
		{name: "Mercado", categoryID: 1},
		{name: "Mercado Lizza", categoryID: 2},
	}

	categoryID, ok := findExpenseCategory("Compra no MERCADO LIZZA", rules)
	if !ok {
		t.Fatal("esperava encontrar uma categoria")
	}
	if categoryID != 2 {
		t.Fatalf("categoria = %d; esperado 2", categoryID)
	}
}

func TestFindExpenseCategoryReturnsNoMatch(t *testing.T) {
	categoryID, ok := findExpenseCategory("Farmácia", []expenseRule{{name: "Mercado", categoryID: 1}})
	if ok {
		t.Fatalf("não esperava correspondência; categoria retornada: %d", categoryID)
	}
}

func TestDaysInMonth(t *testing.T) {
	february := time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC)
	if days := daysInMonth(february); days != 29 {
		t.Fatalf("dias em fevereiro de 2024 = %d; esperado 29", days)
	}
}

func TestParseAmountBrazilianFormat(t *testing.T) {
	amount, err := parseAmount("- 6.845,42")
	if err != nil {
		t.Fatalf("parseAmount retornou erro: %v", err)
	}
	if amount != -6845.42 {
		t.Fatalf("valor = %v; esperado -6845.42", amount)
	}
}

func TestIsValidExpenseDay(t *testing.T) {
	february := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		day   int
		valid bool
	}{
		{day: 1, valid: true},
		{day: 28, valid: true},
		{day: 0, valid: false},
		{day: 29, valid: false},
	}
	for _, test := range tests {
		if got := isValidExpenseDay(february, test.day); got != test.valid {
			t.Errorf("isValidExpenseDay(fevereiro, %d) = %t; esperado %t", test.day, got, test.valid)
		}
	}
}

func TestFormatBRL(t *testing.T) {
	tests := []struct {
		amount float64
		want   string
	}{
		{amount: 1234.5, want: "R$ 1.234,50"},
		{amount: 0, want: "R$ 0,00"},
		{amount: -25.75, want: "R$ -25,75"},
	}
	for _, test := range tests {
		if got := formatBRL(test.amount); got != test.want {
			t.Errorf("formatBRL(%v) = %q; esperado %q", test.amount, got, test.want)
		}
	}
}
