package postgres

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"minhas-financas/internal/category"
	"minhas-financas/internal/dashboard"
	"minhas-financas/internal/expense"
	"minhas-financas/internal/income"
	"minhas-financas/internal/nubank"
	"minhas-financas/internal/referencemonth"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	pool *pgxpool.Pool
}

func Connect(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}

	if err := migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate PostgreSQL schema: %w", err)
	}

	return &Store{pool: pool}, nil
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) Close() {
	s.pool.Close()
}

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	for _, migration := range []struct {
		version  int
		filename string
	}{
		{version: 1, filename: "migrations/001_initial_schema.sql"},
		{version: 2, filename: "migrations/002_nubank_imports.sql"},
		{version: 3, filename: "migrations/003_unique_category_names.sql"},
		{version: 4, filename: "migrations/004_active_reference_month_names.sql"},
		{version: 5, filename: "migrations/005_expense_categories.sql"},
		{version: 6, filename: "migrations/006_nubank_categories.sql"},
		{version: 7, filename: "migrations/007_income_reference_month.sql"},
	} {
		var applied bool
		if err := tx.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)",
			migration.version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check schema migration version %d: %w", migration.version, err)
		}

		if applied {
			continue
		}

		script, err := migrations.ReadFile(migration.filename)
		if err != nil {
			return fmt.Errorf("read schema migration %d: %w", migration.version, err)
		}
		if _, err := tx.Exec(ctx, string(script)); err != nil {
			return fmt.Errorf("apply schema migration %d: %w", migration.version, err)
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (version) VALUES ($1)",
			migration.version,
		); err != nil {
			return fmt.Errorf("record schema migration %d: %w", migration.version, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration transaction: %w", err)
	}
	return nil
}

func (s *Store) CreateCategory(ctx context.Context, name string) (category.Category, error) {
	var created category.Category
	err := s.pool.QueryRow(ctx, `
		INSERT INTO category (name)
		VALUES ($1)
		RETURNING id, name
	`, name).Scan(&created.ID, &created.Name)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return category.Category{}, category.ErrAlreadyExists
		}
		return category.Category{}, fmt.Errorf("create category: %w", err)
	}
	return created, nil
}

func (s *Store) ListCategories(ctx context.Context) ([]category.Category, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name
		FROM category
		WHERE date_delete IS NULL
		ORDER BY LOWER(name), id
	`)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	categories := make([]category.Category, 0)
	for rows.Next() {
		var item category.Category
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		categories = append(categories, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate categories: %w", err)
	}
	return categories, nil
}

func (s *Store) CreateExpense(ctx context.Context, name, amount string) (expense.Expense, error) {
	var created expense.Expense
	err := s.pool.QueryRow(ctx, `
		INSERT INTO expense (name, valor)
		VALUES ($1, $2)
		RETURNING id, name, valor::text
	`, name, amount).Scan(&created.ID, &created.Name, &created.Valor)
	if err != nil {
		return expense.Expense{}, fmt.Errorf("create expense: %w", err)
	}
	created.Source = expense.SourceManual
	return created, nil
}

func (s *Store) CreateIncome(ctx context.Context, referenceMonthID int64, name, amount string) (income.Income, error) {
	var created income.Income
	err := s.pool.QueryRow(ctx, `
		INSERT INTO entrada (id_mes_ref, name, valor)
		SELECT id, $2, $3
		FROM mes_ref
		WHERE id = $1 AND date_delete IS NULL
		RETURNING id, name, valor::text, id_mes_ref
	`, referenceMonthID, name, amount).Scan(&created.ID, &created.Name, &created.Amount, &created.ReferenceMonthID)
	if errors.Is(err, pgx.ErrNoRows) {
		return income.Income{}, income.ErrReferenceMonthNotFound
	}
	if err != nil {
		return income.Income{}, fmt.Errorf("create income: %w", err)
	}
	return created, nil
}

func (s *Store) ListExpenses(ctx context.Context) ([]expense.Expense, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, valor, id_category, source
		FROM (
			SELECT id, name, valor::text AS valor, id_category, 'manual'::text AS source
			FROM expense
			WHERE date_delete IS NULL
			UNION ALL
			SELECT id, title AS name, amount::text AS valor, id_category, 'nubank'::text AS source
			FROM nubank
		) all_expenses
		ORDER BY LOWER(name), source, id
	`)
	if err != nil {
		return nil, fmt.Errorf("list expenses: %w", err)
	}
	defer rows.Close()

	expenses := make([]expense.Expense, 0)
	for rows.Next() {
		var item expense.Expense
		var categoryID pgtype.Int8
		if err := rows.Scan(&item.ID, &item.Name, &item.Valor, &categoryID, &item.Source); err != nil {
			return nil, fmt.Errorf("scan expense: %w", err)
		}
		if categoryID.Valid {
			item.CategoryID = &categoryID.Int64
		}
		expenses = append(expenses, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expenses: %w", err)
	}
	return expenses, nil
}

func (s *Store) Summary(ctx context.Context, referenceMonthID int64) (dashboard.Summary, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return dashboard.Summary{}, fmt.Errorf("begin dashboard summary read: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var summary dashboard.Summary
	err = tx.QueryRow(ctx, `
		SELECT
			COALESCE((SELECT SUM(e.valor) FROM entrada e WHERE e.id_mes_ref = mr.id AND e.date_delete IS NULL), 0)::text,
			COALESCE(SUM(n.amount), 0)::text,
			(
				COALESCE((SELECT SUM(e.valor) FROM entrada e WHERE e.id_mes_ref = mr.id AND e.date_delete IS NULL), 0)
				- COALESCE(SUM(n.amount), 0)
			)::text,
			COUNT(n.id)::integer
		FROM mes_ref mr
		LEFT JOIN nubank n ON n.id_mes_ref = mr.id
		WHERE mr.id = $1 AND mr.date_delete IS NULL
		GROUP BY mr.id
	`, referenceMonthID).Scan(&summary.Income, &summary.Expenses, &summary.Balance, &summary.ExpenseCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return dashboard.Summary{}, referencemonth.ErrNotFound
	}
	if err != nil {
		return dashboard.Summary{}, fmt.Errorf("load financial summary for reference month: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT
			COALESCE(c.name, 'Sem categoria') AS category_name,
			SUM(n.amount)::text AS category_total,
			json_agg(
				json_build_object('name', n.title, 'amount', n.amount::text)
				ORDER BY n.date, n.id
			)::text AS expenses
		FROM nubank n
		LEFT JOIN category c ON c.id = n.id_category AND c.date_delete IS NULL
		WHERE n.id_mes_ref = $1
		GROUP BY c.id, c.name
		ORDER BY category_name
	`, referenceMonthID)
	if err != nil {
		return dashboard.Summary{}, fmt.Errorf("load expenses grouped by category: %w", err)
	}
	summary.Categories = make([]dashboard.CategoryGroup, 0)
	for rows.Next() {
		var group dashboard.CategoryGroup
		var details string
		if err := rows.Scan(&group.Name, &group.Total, &details); err != nil {
			rows.Close()
			return dashboard.Summary{}, fmt.Errorf("scan category expense group: %w", err)
		}
		if err := json.Unmarshal([]byte(details), &group.Expenses); err != nil {
			rows.Close()
			return dashboard.Summary{}, fmt.Errorf("decode category expense details: %w", err)
		}
		summary.Categories = append(summary.Categories, group)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return dashboard.Summary{}, fmt.Errorf("iterate category expense groups: %w", err)
	}
	rows.Close()
	summary.CategoryCount = len(summary.Categories)

	if err := tx.Commit(ctx); err != nil {
		return dashboard.Summary{}, fmt.Errorf("commit dashboard summary read: %w", err)
	}
	return summary, nil
}

func (s *Store) AssignCategory(ctx context.Context, expenseID, categoryID int64, source string) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin expense category assignment: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var expenseName string
	query := `
		SELECT name FROM expense
		WHERE id = $1 AND date_delete IS NULL
		FOR UPDATE
	`
	if source == expense.SourceNubank {
		query = "SELECT title FROM nubank WHERE id = $1"
	}
	if err := tx.QueryRow(ctx, query, expenseID).Scan(&expenseName); errors.Is(err, pgx.ErrNoRows) {
		return 0, expense.ErrNotFound
	} else if err != nil {
		return 0, fmt.Errorf("find expense for category assignment: %w", err)
	}

	var categoryExists bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM category WHERE id = $1 AND date_delete IS NULL)", categoryID).Scan(&categoryExists); err != nil {
		return 0, fmt.Errorf("validate category for expense assignment: %w", err)
	}
	if !categoryExists {
		return 0, expense.ErrCategoryNotFound
	}

	result, err := tx.Exec(ctx, `
		UPDATE expense
		SET id_category = $1, date_update = CURRENT_TIMESTAMP
		WHERE date_delete IS NULL AND LOWER(BTRIM(name)) = LOWER(BTRIM($2))
	`, categoryID, expenseName)
	if err != nil {
		return 0, fmt.Errorf("assign category to expenses with matching names: %w", err)
	}
	nubankResult, err := tx.Exec(ctx, `
		UPDATE nubank
		SET id_category = $1
		WHERE LOWER(BTRIM(title)) = LOWER(BTRIM($2))
	`, categoryID, expenseName)
	if err != nil {
		return 0, fmt.Errorf("assign category to Nubank transactions with matching titles: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE despesa_categoria dc
		SET id_category = $1, date_update = CURRENT_TIMESTAMP
		FROM expense e
		WHERE dc.id_expense = e.id
		  AND dc.date_delete IS NULL
		  AND e.date_delete IS NULL
		  AND LOWER(BTRIM(e.name)) = LOWER(BTRIM($2))
	`, categoryID, expenseName); err != nil {
		return 0, fmt.Errorf("update categories for matching Nubank expense links: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit expense category assignment: %w", err)
	}
	return result.RowsAffected() + nubankResult.RowsAffected(), nil
}

func (s *Store) List(ctx context.Context) ([]referencemonth.ReferenceMonth, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name
		FROM mes_ref
		WHERE date_delete IS NULL
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list reference months: %w", err)
	}
	defer rows.Close()

	months := make([]referencemonth.ReferenceMonth, 0)
	for rows.Next() {
		var month referencemonth.ReferenceMonth
		if err := rows.Scan(&month.ID, &month.Name); err != nil {
			return nil, fmt.Errorf("scan reference month: %w", err)
		}
		months = append(months, month)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reference months: %w", err)
	}
	return months, nil
}

func (s *Store) CreateReferenceMonth(ctx context.Context, name string) (referencemonth.ReferenceMonth, error) {
	var month referencemonth.ReferenceMonth
	err := s.pool.QueryRow(ctx, `
		INSERT INTO mes_ref (name)
		VALUES ($1)
		RETURNING id, name
	`, name).Scan(&month.ID, &month.Name)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return referencemonth.ReferenceMonth{}, referencemonth.ErrAlreadyExists
		}
		return referencemonth.ReferenceMonth{}, fmt.Errorf("create reference month: %w", err)
	}
	return month, nil
}

func (s *Store) UpdateReferenceMonth(ctx context.Context, id int64, name string) (referencemonth.ReferenceMonth, error) {
	var month referencemonth.ReferenceMonth
	err := s.pool.QueryRow(ctx, `
		UPDATE mes_ref
		SET name = $1, date_update = CURRENT_TIMESTAMP
		WHERE id = $2 AND date_delete IS NULL
		RETURNING id, name
	`, name, id).Scan(&month.ID, &month.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return referencemonth.ReferenceMonth{}, referencemonth.ErrNotFound
	}
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return referencemonth.ReferenceMonth{}, referencemonth.ErrAlreadyExists
		}
		return referencemonth.ReferenceMonth{}, fmt.Errorf("update reference month: %w", err)
	}
	return month, nil
}

func (s *Store) DeleteReferenceMonth(ctx context.Context, id int64) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE mes_ref
		SET date_delete = CURRENT_TIMESTAMP, date_update = CURRENT_TIMESTAMP
		WHERE id = $1 AND date_delete IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("delete reference month: %w", err)
	}
	if result.RowsAffected() == 0 {
		return referencemonth.ErrNotFound
	}
	return nil
}

type transactionKey struct {
	date   string
	title  string
	amount string
}

type importedTransaction struct {
	id         int64
	key        transactionKey
	categoryID pgtype.Int8
	linkIDs    []int64
}

func makeTransactionKey(date time.Time, title, amount string) (transactionKey, error) {
	value, ok := new(big.Rat).SetString(amount)
	if !ok {
		return transactionKey{}, fmt.Errorf("normalize transaction amount %q", amount)
	}
	return transactionKey{
		date:   date.Format("2006-01-02 15:04:05.999999999"),
		title:  title,
		amount: value.RatString(),
	}, nil
}

func (s *Store) Import(ctx context.Context, referenceMonthID int64, transactions []nubank.Transaction) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Nubank CSV import: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var lockedMonthID int64
	if err := tx.QueryRow(ctx, `
		SELECT id FROM mes_ref
		WHERE id = $1 AND date_delete IS NULL
		FOR UPDATE
	`, referenceMonthID).Scan(&lockedMonthID); errors.Is(err, pgx.ErrNoRows) {
		return nubank.ErrReferenceMonthNotFound
	} else if err != nil {
		return fmt.Errorf("validate reference month for import: %w", err)
	}

	oldRows, err := tx.Query(ctx, `
		SELECT n.id, n.date, n.title, n.amount::text, n.id_category, dc.id, dc.id_category
		FROM nubank n
		LEFT JOIN despesa_categoria dc ON dc.id_nubank = n.id AND dc.date_delete IS NULL
		WHERE n.id_mes_ref = $1
		ORDER BY n.id, dc.id DESC
	`, referenceMonthID)
	if err != nil {
		return fmt.Errorf("read existing Nubank transactions: %w", err)
	}

	oldTransactions := make([]importedTransaction, 0)
	for oldRows.Next() {
		var id int64
		var date time.Time
		var title, amount string
		var categoryID, linkID, linkCategoryID pgtype.Int8
		if err := oldRows.Scan(&id, &date, &title, &amount, &categoryID, &linkID, &linkCategoryID); err != nil {
			oldRows.Close()
			return fmt.Errorf("scan existing Nubank transaction: %w", err)
		}
		key, err := makeTransactionKey(date, title, amount)
		if err != nil {
			oldRows.Close()
			return err
		}
		if len(oldTransactions) == 0 || oldTransactions[len(oldTransactions)-1].id != id {
			if !categoryID.Valid && linkCategoryID.Valid {
				categoryID = linkCategoryID
			}
			oldTransactions = append(oldTransactions, importedTransaction{id: id, key: key, categoryID: categoryID})
		} else if !oldTransactions[len(oldTransactions)-1].categoryID.Valid && linkCategoryID.Valid {
			oldTransactions[len(oldTransactions)-1].categoryID = linkCategoryID
		}
		if linkID.Valid {
			index := len(oldTransactions) - 1
			oldTransactions[index].linkIDs = append(oldTransactions[index].linkIDs, linkID.Int64)
		}
	}
	if err := oldRows.Err(); err != nil {
		oldRows.Close()
		return fmt.Errorf("iterate existing Nubank transactions: %w", err)
	}
	oldRows.Close()

	newIDsByKey := make(map[transactionKey][]int64, len(transactions))
	for _, transaction := range transactions {
		key, err := makeTransactionKey(transaction.Date, transaction.Title, transaction.Amount)
		if err != nil {
			return err
		}
		var id int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO nubank (id_mes_ref, date, title, parcela, amount)
			VALUES ($1, $2, $3, NULL, $4)
			RETURNING id
		`, referenceMonthID, transaction.Date, transaction.Title, transaction.Amount).Scan(&id); err != nil {
			return fmt.Errorf("insert Nubank transaction: %w", err)
		}
		newIDsByKey[key] = append(newIDsByKey[key], id)
	}

	oldOccurrences := make(map[transactionKey]int)
	for _, old := range oldTransactions {
		occurrence := oldOccurrences[old.key]
		oldOccurrences[old.key]++
		newIDs := newIDsByKey[old.key]
		if occurrence < len(newIDs) {
			if old.categoryID.Valid {
				if _, err := tx.Exec(ctx, "UPDATE nubank SET id_category = $1 WHERE id = $2", old.categoryID.Int64, newIDs[occurrence]); err != nil {
					return fmt.Errorf("preserve Nubank transaction category: %w", err)
				}
			}
			for _, linkID := range old.linkIDs {
				if _, err := tx.Exec(ctx,
					"UPDATE despesa_categoria SET id_nubank = $1 WHERE id = $2",
					newIDs[occurrence], linkID,
				); err != nil {
					return fmt.Errorf("reassociate category link: %w", err)
				}
			}
			continue
		}
		for _, linkID := range old.linkIDs {
			if _, err := tx.Exec(ctx, "DELETE FROM despesa_categoria WHERE id = $1", linkID); err != nil {
				return fmt.Errorf("delete unmatched category link: %w", err)
			}
		}
	}

	for _, old := range oldTransactions {
		if _, err := tx.Exec(ctx, "DELETE FROM nubank WHERE id = $1 AND id_mes_ref = $2", old.id, referenceMonthID); err != nil {
			return fmt.Errorf("delete replaced Nubank transaction %d: %w", old.id, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Nubank CSV import: %w", err)
	}
	return nil
}
