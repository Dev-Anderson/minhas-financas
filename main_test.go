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

func TestBuildDashboardData(t *testing.T) {
	data := buildDashboardData([]dashboardRow{
		{CategoryName: "Moradia", Title: "Aluguel", AmountCents: 180000},
		{CategoryName: "Moradia", Title: "Condomínio", AmountCents: 45000},
		{CategoryName: "Transporte", Title: "Combustível", AmountCents: 25000},
		{Uncategorized: true, Title: "Compra sem categoria", AmountCents: 1000},
	})

	if data.IncomeCents != 0 {
		t.Errorf("entradas = %d; esperado 0", data.IncomeCents)
	}
	if data.ExpenseCents != 251000 {
		t.Errorf("saídas = %d; esperado 251000", data.ExpenseCents)
	}
	if data.BalanceCents != -251000 {
		t.Errorf("saldo = %d; esperado -251000", data.BalanceCents)
	}
	if len(data.Categories) != 2 || data.Categories[0].Name != "Moradia" || data.Categories[0].TotalCents != 225000 {
		t.Errorf("categorias inesperadas: %#v", data.Categories)
	}
	if len(data.Categories[0].Expenses) != 2 {
		t.Errorf("despesas de Moradia = %d; esperado 2", len(data.Categories[0].Expenses))
	}
	if len(data.Uncategorized) != 1 || data.Uncategorized[0].TotalCents != 1000 {
		t.Errorf("despesas sem categoria inesperadas: %#v", data.Uncategorized)
	}
}

func TestBuildDashboardDataWithoutExpenses(t *testing.T) {
	data := buildDashboardData(nil)
	if data.ExpenseCents != 0 || data.BalanceCents != 0 || len(data.Categories) != 0 {
		t.Errorf("dashboard vazio inesperado: %#v", data)
	}
}

func TestValidateManualExpense(t *testing.T) {
	date, month, amount, err := validateManualExpense(manualExpenseRequest{
		Month:  "2024-02",
		Day:    29,
		Title:  "  Consulta  ",
		Amount: "125,50",
	})
	if err != nil {
		t.Fatalf("validateManualExpense retornou erro: %v", err)
	}
	if date.Format("2006-01-02") != "2024-02-29" || month.Format("2006-01-02") != "2024-02-01" || amount != 125.5 {
		t.Errorf("despesa validada inesperada: data=%s mês=%s valor=%v", date.Format("2006-01-02"), month.Format("2006-01-02"), amount)
	}
}

func TestValidateManualExpenseRejectsInvalidInput(t *testing.T) {
	tests := []manualExpenseRequest{
		{Month: "2026-02", Day: 29, Title: "Despesa", Amount: "10,00"},
		{Month: "2026-02-01", Day: 1, Title: "Despesa", Amount: "10,00"},
		{Month: "2026-02", Day: 1, Title: "  ", Amount: "10,00"},
		{Month: "2026-02", Day: 1, Title: "Despesa", Amount: "inválido"},
	}
	for _, test := range tests {
		if _, _, _, err := validateManualExpense(test); err == nil {
			t.Errorf("validateManualExpense(%#v) deveria retornar erro", test)
		}
	}
}

func TestParseBackupRecordCategory(t *testing.T) {
	record, err := parseBackupRecord([]string{"CATEGORY", "12", "Moradia", "", "", "", "", ""})
	if err != nil {
		t.Fatalf("parseBackupRecord retornou erro: %v", err)
	}
	if record.typeName != "CATEGORY" || record.id != 12 || record.values[0] != "Moradia" {
		t.Fatalf("registro inesperado: %#v", record)
	}
}

func TestParseBackupRecordTransaction(t *testing.T) {
	row := []string{"TRANSACTION", "23", "2026-10-02", "Aluguel", "1800.00", "2026-10-01", "MANUAL", "4"}
	if _, err := parseBackupRecord(row); err != nil {
		t.Fatalf("parseBackupRecord retornou erro: %v", err)
	}
}

func TestParseBackupRecordRejectsInvalidData(t *testing.T) {
	tests := [][]string{
		{"UNKNOWN", "1", "", "", "", "", "", ""},
		{"CATEGORY", "0", "Moradia", "", "", "", "", ""},
		{"EXPENSE_CATEGORY", "1", "invalid", "Aluguel", "", "", "", ""},
		{"TRANSACTION", "1", "2026-10-32", "Aluguel", "10.00", "2026-10-01", "MANUAL", ""},
		{"TRANSACTION", "1", "2026-10-01", "Aluguel", "10.00", "2026-10-01", "MANUAL", "invalid"},
	}
	for _, row := range tests {
		if _, err := parseBackupRecord(row); err == nil {
			t.Errorf("parseBackupRecord(%v) deveria retornar erro", row)
		}
	}
}

func TestIsLocalCSVFileName(t *testing.T) {
	if !isLocalCSVFileName("backup.csv") {
		t.Fatal("nome CSV local deveria ser aceito")
	}
	for _, fileName := range []string{"", "backup.txt", "../backup.csv", "/tmp/backup.csv"} {
		if isLocalCSVFileName(fileName) {
			t.Errorf("nome %q deveria ser rejeitado", fileName)
		}
	}
}
