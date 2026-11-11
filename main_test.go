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

func TestParseReferenceMonth(t *testing.T) {
	month, err := parseReferenceMonth("2024-02")
	if err != nil {
		t.Fatalf("parseReferenceMonth retornou erro: %v", err)
	}
	if month.Format("2006-01-02") != "2024-02-01" {
		t.Fatalf("mês = %s; esperado 2024-02-01", month.Format("2006-01-02"))
	}

	for _, invalidMonth := range []string{"", "2024-2", "2024-02-01", "24-02", "2024-13"} {
		if _, err := parseReferenceMonth(invalidMonth); err == nil {
			t.Errorf("parseReferenceMonth(%q) deveria retornar erro", invalidMonth)
		}
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

func TestTakePreservedPaymentStatus(t *testing.T) {
	key := importedTransactionKey{
		date:           "2026-10-02",
		title:          "Aluguel",
		amount:         "1800.00",
		statementMonth: "2026-10-01",
	}
	statuses := map[importedTransactionKey][]bool{key: {true, false}}

	if !takePreservedPaymentStatus(statuses, key) {
		t.Fatal("o status pago do primeiro lançamento deveria ser preservado")
	}
	if takePreservedPaymentStatus(statuses, key) {
		t.Fatal("o status não pago do segundo lançamento deveria ser preservado")
	}
	if takePreservedPaymentStatus(statuses, key) {
		t.Fatal("lançamento novo deveria iniciar como não pago")
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
		{ID: 1, CategoryName: "Moradia", Title: "Aluguel", AmountCents: 180000, Paid: true, Manual: true, StatementMonth: "2026-11", Day: 5},
		{ID: 2, CategoryName: "Moradia", Title: "Condomínio", AmountCents: 45000},
		{ID: 3, CategoryName: "Transporte", Title: "Combustível", AmountCents: 25000, Paid: true},
		{ID: 4, Uncategorized: true, Title: "Compra sem categoria", AmountCents: 1000},
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
	if data.PaidCents != 205000 || data.UnpaidCents != 46000 || data.RemainingCents != 46000 {
		t.Errorf("resumo de pagamento inesperado: pago=%d não pago=%d restante=%d", data.PaidCents, data.UnpaidCents, data.RemainingCents)
	}
	if len(data.Categories) != 2 || data.Categories[0].Name != "Moradia" || data.Categories[0].TotalCents != 225000 {
		t.Errorf("categorias inesperadas: %#v", data.Categories)
	}
	if len(data.Categories[0].Expenses) != 2 {
		t.Errorf("despesas de Moradia = %d; esperado 2", len(data.Categories[0].Expenses))
	}
	if data.Categories[0].Expenses[0].ID != 1 || !data.Categories[0].Expenses[0].Paid {
		t.Errorf("status individual do lançamento não foi preservado: %#v", data.Categories[0].Expenses[0])
	}
	if !data.Categories[0].Expenses[0].Manual || data.Categories[0].Expenses[0].StatementMonth != "2026-11" || data.Categories[0].Expenses[0].Day != 5 {
		t.Errorf("dados de edição do lançamento manual não foram preservados: %#v", data.Categories[0].Expenses[0])
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
		{Month: "2026-02", Day: 1, Title: "Despesa", Amount: "0"},
		{Month: "2026-02", Day: 1, Title: "Despesa", Amount: "-10,00"},
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

func TestParseBackupRecordPayment(t *testing.T) {
	row := []string{"PAYMENT", "23", "true", "", "", "", "", ""}
	record, err := parseBackupRecord(row)
	if err != nil {
		t.Fatalf("parseBackupRecord retornou erro: %v", err)
	}
	if record.typeName != "PAYMENT" || record.id != 23 {
		t.Fatalf("registro inesperado: %#v", record)
	}
}

func TestParseBackupRecordRejectsInvalidData(t *testing.T) {
	tests := [][]string{
		{"UNKNOWN", "1", "", "", "", "", "", ""},
		{"CATEGORY", "0", "Moradia", "", "", "", "", ""},
		{"EXPENSE_CATEGORY", "1", "invalid", "Aluguel", "", "", "", ""},
		{"TRANSACTION", "1", "2026-10-32", "Aluguel", "10.00", "2026-10-01", "MANUAL", ""},
		{"TRANSACTION", "1", "2026-10-01", "Aluguel", "10.00", "2026-10-01", "MANUAL", "invalid"},
		{"PAYMENT", "1", "false", "", "", "", "", ""},
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
