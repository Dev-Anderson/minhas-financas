package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		fmt.Printf("Erro ao carregar arquivo .env: %v\n", err)
		return
	}

	ctx := context.Background()
	db, err := connectDB(ctx)
	if err != nil {
		fmt.Printf("Erro ao conectar no banco: %v\n", err)
		return
	}

	defer db.Close()

	if err := createTables(ctx, db); err != nil {
		fmt.Printf("Erro ao preparar tabelas: %v\n", err)
		return
	}

	reader := bufio.NewScanner(os.Stdin)
	for {
		fmt.Println("\n=== Minhas Finanças ===")
		fmt.Println("1 - Importar extrato CSV")
		fmt.Println("2 - Cadastrar categoria")
		fmt.Println("3 - Listar categorias")
		fmt.Println("4 - Excluir categoria")
		fmt.Println("5 - Vincular despesa a categoria")
		fmt.Println("6 - Listar despesas e categorias")
		fmt.Println("7 - Categorizar lançamentos")
		fmt.Println("8 - Cadastrar despesa manual")
		fmt.Println("9 - Mostrar total geral de despesas")
		fmt.Println("10 - Mostrar despesas por categoria")
		fmt.Println("0 - Sair")
		fmt.Print("> ")

		if !reader.Scan() {
			break
		}

		switch strings.ToLower(strings.TrimSpace(reader.Text())) {
		case "1", "import":
			fmt.Print("Mês e ano do extrato (AAAA-MM): ")
			if !reader.Scan() {
				return
			}
			statementMonth, err := time.Parse("2006-01", strings.TrimSpace(reader.Text()))
			if err != nil {
				fmt.Println("Período inválido. Use o formato AAAA-MM, por exemplo 2026-10.")
				continue
			}

			fmt.Print("Nome do arquivo CSV (na pasta do programa): ")
			if !reader.Scan() {
				return
			}
			csvFile := strings.TrimSpace(reader.Text())
			if csvFile == "" || filepath.Base(csvFile) != csvFile || strings.ToLower(filepath.Ext(csvFile)) != ".csv" {
				fmt.Println("Informe apenas o nome de um arquivo .csv que esteja na pasta do programa.")
				continue
			}

			importedCount, err := importTransactions(ctx, db, csvFile, statementMonth)
			if err != nil {
				fmt.Printf("Erro ao importar arquivo CSV: %v\n", err)
				continue
			}
			fmt.Printf("Importação de %s concluída: %d lançamentos carregados.\n", statementMonth.Format("01/2006"), importedCount)
		case "2", "categoria", "cadastrar categoria":
			name, ok := prompt(reader, "Nome da categoria: ")
			if !ok {
				return
			}
			if err := createCategory(ctx, db, name); err != nil {
				fmt.Printf("Erro ao cadastrar categoria: %v\n", err)
				continue
			}
			fmt.Println("Categoria cadastrada como ativa.")
		case "3", "listar categorias", "categorias":
			if err := listCategories(ctx, db); err != nil {
				fmt.Printf("Erro ao listar categorias: %v\n", err)
			}
		case "4", "excluir categoria", "remover categoria":
			if err := deleteCategory(ctx, db, reader); err != nil {
				fmt.Printf("Erro ao excluir categoria: %v\n", err)
			}
		case "5", "vincular despesa", "categoria despesa":
			if err := createExpenseCategory(ctx, db, reader); err != nil {
				fmt.Printf("Erro ao vincular despesa: %v\n", err)
			}
		case "6", "listar despesas", "vinculos":
			if err := listExpenseCategories(ctx, db); err != nil {
				fmt.Printf("Erro ao listar vínculos: %v\n", err)
			}
		case "7", "categorizar", "categorizar lançamentos":
			categorized, uncategorized, err := categorizeTransactions(ctx, db)
			if err != nil {
				fmt.Printf("Erro ao categorizar lançamentos: %v\n", err)
				continue
			}
			fmt.Printf("Categorizações aplicadas: %d. Sem categoria correspondente: %d.\n", categorized, uncategorized)
		case "8", "cadastrar despesas", "cadastrar despesa":
			if err := createManualExpense(ctx, db, reader); err != nil {
				fmt.Printf("Erro ao cadastrar despesa: %v\n", err)
			}
		case "9", "total geral", "total despesas":
			if err := reportTotalExpenses(ctx, db); err != nil {
				fmt.Printf("Erro ao calcular total de despesas: %v\n", err)
			}
		case "10", "despesas por categoria", "total por categoria":
			if err := reportExpensesByCategory(ctx, db); err != nil {
				fmt.Printf("Erro ao calcular despesas por categoria: %v\n", err)
			}
		case "0", "sair", "exit", "quit":
			fmt.Println("Até mais!")
			return
		case "":
		default:
			fmt.Println("Opção não reconhecida. Escolha um número ou nome do menu.")
		}
	}

	if err := reader.Err(); err != nil {
		fmt.Printf("Erro ao ler entrada: %v\n", err)
	}
}

func prompt(reader *bufio.Scanner, label string) (string, bool) {
	fmt.Print(label)
	if !reader.Scan() {
		return "", false
	}
	return strings.TrimSpace(reader.Text()), true
}

func connectDB(ctx context.Context) (*pgxpool.Pool, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://user:password@localhost:5432/finances?sslmode=disable"
	}

	var lastErr error
	for i := 0; i < 10; i++ {
		db, err := pgxpool.New(ctx, dbURL)
		if err == nil {
			if err = db.Ping(ctx); err == nil {
				return db, nil
			}
			db.Close()
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}

	return nil, lastErr
}

func createTables(ctx context.Context, db *pgxpool.Pool) error {
	const schema = `
		CREATE TABLE IF NOT EXISTS categoria (
			id BIGSERIAL PRIMARY KEY,
			nome TEXT NOT NULL UNIQUE
		);

		CREATE TABLE IF NOT EXISTS categoria_despesa (
			id BIGSERIAL PRIMARY KEY,
			id_categoria BIGINT NOT NULL REFERENCES categoria(id),
			nome_despesas TEXT NOT NULL UNIQUE
		);

		CREATE TABLE IF NOT EXISTS transactions (
			id BIGSERIAL PRIMARY KEY,
			date DATE NOT NULL,
			title TEXT NOT NULL,
			amount NUMERIC(12, 2) NOT NULL,
			statement_month DATE NOT NULL,
			origin TEXT NOT NULL DEFAULT 'IMPORT',
			id_categoria BIGINT REFERENCES categoria(id)
		)`

	if _, err := db.Exec(ctx, schema); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE categoria DROP COLUMN IF EXISTS ativo`); err != nil {
		return err
	}

	if _, err := db.Exec(ctx, `ALTER TABLE transactions ADD COLUMN IF NOT EXISTS statement_month DATE`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE transactions ADD COLUMN IF NOT EXISTS id BIGSERIAL`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE transactions ADD COLUMN IF NOT EXISTS origin TEXT NOT NULL DEFAULT 'IMPORT'`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_date_title_amount_key`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS transactions_id_unique ON transactions(id)`); err != nil {
		return err
	}
	_, err := db.Exec(ctx, `ALTER TABLE transactions ADD COLUMN IF NOT EXISTS id_categoria BIGINT REFERENCES categoria(id)`)
	return err
}

type category struct {
	id   int64
	name string
}

func createCategory(ctx context.Context, db *pgxpool.Pool, name string) error {
	if name == "" {
		return fmt.Errorf("o nome da categoria não pode ficar vazio")
	}
	_, err := db.Exec(ctx, `INSERT INTO categoria (nome) VALUES ($1)`, name)
	return err
}

func getCategories(ctx context.Context, db *pgxpool.Pool) ([]category, error) {
	rows, err := db.Query(ctx, `SELECT id, nome FROM categoria ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []category
	for rows.Next() {
		var item category
		if err := rows.Scan(&item.id, &item.name); err != nil {
			return nil, err
		}
		categories = append(categories, item)
	}
	return categories, rows.Err()
}

func listCategories(ctx context.Context, db *pgxpool.Pool) error {
	categories, err := getCategories(ctx, db)
	if err != nil {
		return err
	}
	if len(categories) == 0 {
		fmt.Println("Nenhuma categoria cadastrada.")
		return nil
	}
	for _, item := range categories {
		fmt.Printf("%d - %s\n", item.id, item.name)
	}
	return nil
}

func deleteCategory(ctx context.Context, db *pgxpool.Pool, reader *bufio.Scanner) error {
	if err := listCategories(ctx, db); err != nil {
		return err
	}
	value, ok := prompt(reader, "ID da categoria para excluir: ")
	if !ok {
		return nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fmt.Errorf("ID inválido")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var categoryName string
	if err := tx.QueryRow(ctx, `SELECT nome FROM categoria WHERE id = $1 FOR UPDATE`, id).Scan(&categoryName); err != nil {
		return fmt.Errorf("categoria %d não encontrada: %w", id, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM categoria_despesa WHERE id_categoria = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE transactions SET id_categoria = NULL WHERE id_categoria = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM categoria WHERE id = $1`, id); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Printf("Categoria %q excluída. Os lançamentos foram preservados sem categoria.\n", categoryName)
	return nil
}

func createExpenseCategory(ctx context.Context, db *pgxpool.Pool, reader *bufio.Scanner) error {
	categories, err := getCategories(ctx, db)
	if err != nil {
		return err
	}
	if len(categories) == 0 {
		fmt.Println("Cadastre uma categoria antes de vincular despesas.")
		return nil
	}
	fmt.Println("Categorias disponíveis:")
	for _, item := range categories {
		fmt.Printf("%d - %s\n", item.id, item.name)
	}
	value, ok := prompt(reader, "ID da categoria: ")
	if !ok {
		return nil
	}
	categoryID, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fmt.Errorf("ID inválido")
	}
	categoryExists := false
	for _, item := range categories {
		if item.id == categoryID {
			categoryExists = true
			break
		}
	}
	if !categoryExists {
		return fmt.Errorf("selecione o ID de uma categoria da lista")
	}

	expenseName, ok := prompt(reader, "Nome da despesa como aparece no extrato: ")
	if !ok {
		return nil
	}
	if expenseName == "" {
		return fmt.Errorf("o nome da despesa não pode ficar vazio")
	}
	_, err = db.Exec(ctx,
		`INSERT INTO categoria_despesa (id_categoria, nome_despesas) VALUES ($1, $2)
		 ON CONFLICT (nome_despesas) DO UPDATE SET id_categoria = EXCLUDED.id_categoria`,
		categoryID, expenseName,
	)
	return err
}

func listExpenseCategories(ctx context.Context, db *pgxpool.Pool) error {
	rows, err := db.Query(ctx, `
		SELECT cd.id, cd.nome_despesas, c.nome
		FROM categoria_despesa cd
		JOIN categoria c ON c.id = cd.id_categoria
		ORDER BY c.nome, cd.nome_despesas`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id int64
		var expenseName, categoryName string
		if err := rows.Scan(&id, &expenseName, &categoryName); err != nil {
			return err
		}
		fmt.Printf("%d - %s => %s\n", id, expenseName, categoryName)
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count == 0 {
		fmt.Println("Nenhum vínculo de despesa e categoria cadastrado.")
	}
	return nil
}

func categorizeTransactions(ctx context.Context, db *pgxpool.Pool) (int, int, error) {
	rows, err := db.Query(ctx, `
		SELECT cd.nome_despesas, cd.id_categoria
		FROM categoria_despesa cd
		JOIN categoria c ON c.id = cd.id_categoria
		ORDER BY length(cd.nome_despesas) DESC, cd.id`)
	if err != nil {
		return 0, 0, err
	}
	var rules []expenseRule
	for rows.Next() {
		var rule expenseRule
		if err := rows.Scan(&rule.name, &rule.categoryID); err != nil {
			rows.Close()
			return 0, 0, err
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, 0, err
	}
	rows.Close()

	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	transactions, err := tx.Query(ctx, `SELECT id, title FROM transactions WHERE id_categoria IS NULL`)
	if err != nil {
		return 0, 0, err
	}
	type pendingTransaction struct {
		id    int64
		title string
	}
	var pending []pendingTransaction
	for transactions.Next() {
		var transaction pendingTransaction
		if err := transactions.Scan(&transaction.id, &transaction.title); err != nil {
			transactions.Close()
			return 0, 0, err
		}
		pending = append(pending, transaction)
	}
	if err := transactions.Err(); err != nil {
		transactions.Close()
		return 0, 0, err
	}
	transactions.Close()

	categorized, uncategorized := 0, 0
	for _, transaction := range pending {
		categoryID, matched := findExpenseCategory(transaction.title, rules)
		if matched {
			if _, err := tx.Exec(ctx, `UPDATE transactions SET id_categoria = $1 WHERE id = $2`, categoryID, transaction.id); err != nil {
				return 0, 0, err
			}
			categorized++
		} else {
			uncategorized++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return categorized, uncategorized, nil
}

type expenseRule struct {
	name       string
	categoryID int64
}

func findExpenseCategory(title string, rules []expenseRule) (int64, bool) {
	lowerTitle := strings.ToLower(title)
	var categoryID int64
	longestMatch := 0
	for _, rule := range rules {
		lowerRule := strings.ToLower(rule.name)
		if len(lowerRule) > longestMatch && strings.Contains(lowerTitle, lowerRule) {
			categoryID = rule.categoryID
			longestMatch = len(lowerRule)
		}
	}
	return categoryID, longestMatch > 0
}

func importTransactions(ctx context.Context, db *pgxpool.Pool, csvFile string, statementMonth time.Time) (int, error) {
	file, err := os.Open(csvFile)
	if err != nil {
		return 0, fmt.Errorf("abrir %q: %w", csvFile, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		return 0, fmt.Errorf("ler cabeçalho: %w", err)
	}
	if len(header) != 3 || strings.TrimSpace(header[0]) != "date" || strings.TrimSpace(header[1]) != "title" || strings.TrimSpace(header[2]) != "amount" {
		return 0, fmt.Errorf("cabeçalho inválido; esperado: date,title,amount")
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("iniciar transação: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM transactions WHERE origin = 'IMPORT'`); err != nil {
		return 0, fmt.Errorf("limpar lançamentos importados anteriores: %w", err)
	}

	importedCount := 0
	for rowNumber := 2; ; rowNumber++ {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("ler linha %d: %w", rowNumber, err)
		}
		if len(row) != 3 {
			return 0, fmt.Errorf("linha %d deve conter exatamente 3 colunas", rowNumber)
		}

		date, err := time.Parse("2006-01-02", strings.TrimSpace(row[0]))
		if err != nil {
			return 0, fmt.Errorf("data inválida na linha %d: %w", rowNumber, err)
		}
		title := strings.TrimSpace(row[1])
		amountText := strings.TrimSpace(row[2])
		amountText = strings.ReplaceAll(amountText, " ", "")
		amountText = strings.ReplaceAll(amountText, ".", "")
		amountText = strings.ReplaceAll(amountText, ",", ".")
		amount, err := strconv.ParseFloat(amountText, 64)
		if err != nil {
			return 0, fmt.Errorf("valor inválido na linha %d: %w", rowNumber, err)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO transactions (date, title, amount, statement_month, origin) VALUES ($1, $2, $3, $4, 'IMPORT')`,
			date, title, amount, statementMonth,
		); err != nil {
			return 0, fmt.Errorf("inserir linha %d: %w", rowNumber, err)
		}
		importedCount++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("confirmar importação: %w", err)
	}
	return importedCount, nil
}

func createManualExpense(ctx context.Context, db *pgxpool.Pool, reader *bufio.Scanner) error {
	monthText, ok := prompt(reader, "Mês de referência (AAAA-MM): ")
	if !ok {
		return nil
	}
	referenceMonth, err := time.Parse("2006-01", monthText)
	if err != nil {
		return fmt.Errorf("mês inválido; use AAAA-MM, por exemplo 2026-10")
	}

	dayText, ok := prompt(reader, "Dia da despesa (1-31): ")
	if !ok {
		return nil
	}
	day, err := strconv.Atoi(dayText)
	if err != nil || !isValidExpenseDay(referenceMonth, day) {
		return fmt.Errorf("dia inválido para o mês de referência")
	}
	expenseDate := time.Date(referenceMonth.Year(), referenceMonth.Month(), day, 0, 0, 0, 0, time.UTC)

	title, ok := prompt(reader, "Nome da despesa: ")
	if !ok {
		return nil
	}
	if title == "" {
		return fmt.Errorf("o nome da despesa não pode ficar vazio")
	}

	amountText, ok := prompt(reader, "Valor da despesa: ")
	if !ok {
		return nil
	}
	amount, err := parseAmount(amountText)
	if err != nil {
		return fmt.Errorf("valor inválido: %w", err)
	}

	_, err = db.Exec(ctx,
		`INSERT INTO transactions (date, title, amount, statement_month, origin) VALUES ($1, $2, $3, $4, 'MANUAL')`,
		expenseDate, title, amount, referenceMonth,
	)
	if err != nil {
		return err
	}
	fmt.Printf("Despesa cadastrada e preservada nas próximas importações (%s).\n", referenceMonth.Format("01/2006"))
	return nil
}

func daysInMonth(month time.Time) int {
	return time.Date(month.Year(), month.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func isValidExpenseDay(month time.Time, day int) bool {
	return day >= 1 && day <= daysInMonth(month)
}

func parseAmount(value string) (float64, error) {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, ".", "")
	value = strings.ReplaceAll(value, ",", ".")
	return strconv.ParseFloat(value, 64)
}

func reportTotalExpenses(ctx context.Context, db *pgxpool.Pool) error {
	var total float64
	err := db.QueryRow(ctx, `SELECT COALESCE(SUM(amount), 0)::float8 FROM transactions WHERE amount > 0`).Scan(&total)
	if err != nil {
		return err
	}
	fmt.Printf("Total geral de despesas: %s\n", formatBRL(total))
	return nil
}

func reportExpensesByCategory(ctx context.Context, db *pgxpool.Pool) error {
	rows, err := db.Query(ctx, `
		SELECT COALESCE(c.nome, 'Não categorizado'), SUM(t.amount)::float8
		FROM transactions t
		LEFT JOIN categoria c ON c.id = t.id_categoria
		WHERE t.amount > 0
		GROUP BY c.id, c.nome
		ORDER BY c.nome NULLS LAST`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var total float64
	count := 0
	for rows.Next() {
		var categoryName string
		var amount float64
		if err := rows.Scan(&categoryName, &amount); err != nil {
			return err
		}
		fmt.Printf("%-30s %s\n", categoryName, formatBRL(amount))
		total += amount
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count == 0 {
		fmt.Println("Nenhuma despesa positiva cadastrada.")
	}
	fmt.Printf("%-30s %s\n", "TOTAL", formatBRL(total))
	return nil
}

func formatBRL(amount float64) string {
	formatted := strconv.FormatFloat(amount, 'f', 2, 64)
	parts := strings.SplitN(formatted, ".", 2)
	integer := parts[0]
	sign := ""
	if strings.HasPrefix(integer, "-") {
		sign = "-"
		integer = strings.TrimPrefix(integer, "-")
	}
	for index := len(integer) - 3; index > 0; index -= 3 {
		integer = integer[:index] + "." + integer[index:]
	}
	return "R$ " + sign + integer + "," + parts[1]
}
