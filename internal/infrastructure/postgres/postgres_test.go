package postgres

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"minhas-financas/internal/dashboard"
	"minhas-financas/internal/nubank"
)

func TestMakeTransactionKeyNormalizesAmounts(t *testing.T) {
	date := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)

	first, err := makeTransactionKey(date, "Example", "12.34")
	if err != nil {
		t.Fatalf("makeTransactionKey() error = %v", err)
	}
	second, err := makeTransactionKey(date, "Example", "12.3400")
	if err != nil {
		t.Fatalf("makeTransactionKey() error = %v", err)
	}
	if first != second {
		t.Fatalf("expected numerically equal amounts to produce equal keys: %#v != %#v", first, second)
	}
}

func TestImportReplacesOnlyPreviousTransactions(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	store, err := Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer store.Close()

	var monthID int64
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	monthName := "Import integration " + suffix
	if err := store.pool.QueryRow(ctx, "INSERT INTO mes_ref (name) VALUES ($1) RETURNING id", monthName).Scan(&monthID); err != nil {
		t.Fatalf("create test reference month: %v", err)
	}
	var categoryID int64
	categoryName := "Integration category " + suffix
	if err := store.pool.QueryRow(ctx, "INSERT INTO category (name) VALUES ($1) RETURNING id", categoryName).Scan(&categoryID); err != nil {
		t.Fatalf("create test category: %v", err)
	}
	defer func() {
		_, _ = store.pool.Exec(ctx, "DELETE FROM entrada WHERE id_mes_ref = $1", monthID)
		_, _ = store.pool.Exec(ctx, "DELETE FROM expense WHERE id_mes_ref = $1", monthID)
		_, _ = store.pool.Exec(ctx, "DELETE FROM nubank WHERE id_mes_ref = $1", monthID)
		_, _ = store.pool.Exec(ctx, "DELETE FROM mes_ref WHERE id = $1", monthID)
		_, _ = store.pool.Exec(ctx, "DELETE FROM category WHERE id = $1", categoryID)
	}()

	transactionDate := time.Date(2026, time.October, 12, 0, 0, 0, 0, time.UTC)
	var oldTransactionID int64
	if err := store.pool.QueryRow(ctx, `
		INSERT INTO nubank (id_mes_ref, date, title, amount, id_category)
		VALUES ($1, $2, 'Categorized integration transaction', 42.75, $3)
		RETURNING id
	`, monthID, transactionDate, categoryID).Scan(&oldTransactionID); err != nil {
		t.Fatalf("create existing categorized transaction: %v", err)
	}
	transactions := []nubank.Transaction{
		{Date: transactionDate, Title: "Categorized integration transaction", Amount: "42.75"},
		{Date: transactionDate, Title: "Uncategorized integration transaction", Amount: "7.25"},
	}
	for range 2 {
		if err := store.Import(ctx, monthID, transactions); err != nil {
			t.Fatalf("Import() error = %v", err)
		}
	}
	createdIncome, err := store.CreateIncome(ctx, monthID, "Integration salary", "100.00")
	if err != nil {
		t.Fatalf("CreateIncome() error = %v", err)
	}
	if createdIncome.ReferenceMonthID != monthID || createdIncome.Amount != "100.00" {
		t.Fatalf("unexpected created income: %#v", createdIncome)
	}
	createdExpense, err := store.CreateExpense(ctx, monthID, "Integration manual expense", "20.00")
	if err != nil {
		t.Fatalf("CreateExpense() error = %v", err)
	}
	if createdExpense.ReferenceMonthID != monthID || createdExpense.Valor != "20.00" {
		t.Fatalf("unexpected created manual expense: %#v", createdExpense)
	}
	updatedExpense, err := store.UpdateExpense(ctx, createdExpense.ID, monthID, "Updated manual expense", "22.50")
	if err != nil {
		t.Fatalf("UpdateExpense() error = %v", err)
	}
	if updatedExpense.Name != "Updated manual expense" || updatedExpense.Valor != "22.50" {
		t.Fatalf("unexpected updated manual expense: %#v", updatedExpense)
	}
	filteredExpenses, err := store.ListExpenses(ctx, &monthID)
	if err != nil {
		t.Fatalf("ListExpenses() error = %v", err)
	}
	if len(filteredExpenses) != 3 {
		t.Fatalf("expected three expenses for reference month %d, got %#v", monthID, filteredExpenses)
	}
	foundManualExpense := false
	for _, item := range filteredExpenses {
		if item.ID == createdExpense.ID && item.Source == "manual" {
			foundManualExpense = true
		}
	}
	if !foundManualExpense {
		t.Fatalf("filtered expense list is missing the manual expense: %#v", filteredExpenses)
	}

	var count int
	if err := store.pool.QueryRow(ctx, "SELECT COUNT(*) FROM nubank WHERE id_mes_ref = $1", monthID).Scan(&count); err != nil {
		t.Fatalf("count imported transactions: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected two transactions after repeated imports, got %d", count)
	}

	summary, err := store.Summary(ctx, monthID)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.Income != "100.00" || summary.Expenses != "72.50" || summary.Balance != "27.50" || summary.ExpenseCount != 3 || summary.CategoryCount != 2 {
		t.Fatalf("unexpected summary after import: %#v", summary)
	}
	if len(summary.Categories) != 2 {
		t.Fatalf("expected categorized and uncategorized groups, got %#v", summary.Categories)
	}
	groups := make(map[string]dashboard.CategoryGroup, len(summary.Categories))
	for _, group := range summary.Categories {
		groups[group.Name] = group
	}
	categorized := groups[categoryName]
	if categorized.Total != "42.75" || len(categorized.Expenses) != 1 || categorized.Expenses[0].Name != transactions[0].Title {
		t.Fatalf("unexpected categorized group: %#v", categorized)
	}
	uncategorized := groups["Sem categoria"]
	if uncategorized.Total != "29.75" || len(uncategorized.Expenses) != 2 {
		t.Fatalf("unexpected uncategorized group: %#v", uncategorized)
	}
	foundSummaryManualExpense := false
	for _, line := range uncategorized.Expenses {
		if line.ID == createdExpense.ID && line.Source == "manual" && line.Name == "Updated manual expense" {
			foundSummaryManualExpense = true
		}
	}
	if !foundSummaryManualExpense {
		t.Fatalf("manual expense action data missing from summary: %#v", uncategorized.Expenses)
	}

	if err := store.DeleteExpense(ctx, createdExpense.ID); err != nil {
		t.Fatalf("DeleteExpense() error = %v", err)
	}
	summary, err = store.Summary(ctx, monthID)
	if err != nil {
		t.Fatalf("Summary() after deleting manual expense error = %v", err)
	}
	if summary.Expenses != "50.00" || summary.ExpenseCount != 2 {
		t.Fatalf("unexpected summary after deleting manual expense: %#v", summary)
	}

	var preservedCategoryID int64
	if err := store.pool.QueryRow(ctx, "SELECT id_category FROM nubank WHERE id_mes_ref = $1 AND title = $2", monthID, transactions[0].Title).Scan(&preservedCategoryID); err != nil {
		t.Fatalf("read preserved category: %v", err)
	}
	if preservedCategoryID != categoryID {
		t.Fatalf("expected category %d to be preserved after replacement, got %d", categoryID, preservedCategoryID)
	}
}
