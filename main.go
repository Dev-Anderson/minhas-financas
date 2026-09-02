package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Transaction struct {
	ID       int
	Date     time.Time
	Title    string
	Amount   float64
	Category string
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://user:password@localhost:5432/finances?sslmode=disable"
	}

	ctx := context.Background()

	// Aguarda conexão com o banco
	var db *pgxpool.Pool
	var err error
	for i := 0; i < 10; i++ {
		db, err = pgxpool.New(ctx, dbURL)
		if err == nil && db.Ping(ctx) == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		fmt.Printf("Erro ao conectar no banco: %v\n", err)
		return
	}
	defer db.Close()

	initDB(ctx, db)

	fmt.Println("=== Gerenciador de Gastos Nubank ===")
	// fmt.Println("Comandos disponíveis: /import, /classificar, /categoria")
	fmt.Println("Digite /help para listar todos os comandos e suas descrições.")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		parts := strings.Fields(input)
		if len(parts) == 0 {
			continue
		}

		switch parts[0] {
		case "/help":
			showHelp()
		case "/import":
			importCSV(ctx, db)
		case "/classificar":
			classifyTransactions(ctx, db)
		case "/categoria":
			if len(parts) < 3 {
				fmt.Println("Uso correto: /categoria \"<titulo>\" <nome_categoria>")
				fmt.Println("Exemplo: /categoria \"Mercado Lizza\" Mercado")
				continue
			}
			addCategoryMapping(ctx, db, input)
		default:
			// fmt.Println("Comando não reconhecido. Use: /import, /classificar ou /categoria")
			fmt.Println("Comando não reconhecido. Digite /help para ver os comandos disponíveis.")
		}
	}
}

func showHelp() {
	fmt.Println("\n--- Comandos Disponíveis ---")
	fmt.Println("/help")
	fmt.Println("  Exibe esta lista com todos os comandos disponíveis e suas breves descrições.")
	fmt.Println()
	fmt.Println("/import")
	fmt.Println("  Lê todos os arquivos .csv do diretório e importa os novos gastos para a tabela do banco de dados.")
	fmt.Println()
	fmt.Println("/classificar")
	fmt.Println("  Classifica os gastos pendentes conforme o histórico de regras e lista os itens que ainda não possuem classificação.")
	fmt.Println()
	fmt.Println("/categoria \"<titulo>\" <nome_categoria>")
	fmt.Println("  Cria ou atualiza uma regra no histórico. Sempre que o título do gasto contiver o texto especificado, ele será atribuído à categoria informada.")
	fmt.Println("  Exemplo: /categoria \"Mercado Lizza\" Mercado")
}

func initDB(ctx context.Context, db *pgxpool.Pool) {
	schema := `
	CREATE TABLE IF NOT EXISTS category_mappings (
		id SERIAL PRIMARY KEY,
		title_pattern VARCHAR(255) UNIQUE NOT NULL,
		category VARCHAR(100) NOT NULL
	);

	CREATE TABLE IF NOT EXISTS transactions (
		id SERIAL PRIMARY KEY,
		date DATE NOT NULL,
		title VARCHAR(255) NOT NULL,
		amount NUMERIC(10, 2) NOT NULL,
		category VARCHAR(100) DEFAULT 'Não Classificado',
		CONSTRAINT unique_transaction UNIQUE (date, title, amount)
	);`

	_, err := db.Exec(ctx, schema)
	if err != nil {
		fmt.Printf("Erro ao criar tabelas: %v\n", err)
	}
}

func importCSV(ctx context.Context, db *pgxpool.Pool) {
	files, err := filepath.Glob("*.csv")
	if err != nil || len(files) == 0 {
		fmt.Println("Nenhum arquivo CSV encontrado no diretório.")
		return
	}

	importedCount := 0
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			continue
		}

		reader := csv.NewReader(f)
		records, err := reader.ReadAll()
		f.Close()

		if err != nil || len(records) <= 1 {
			continue
		}

		for _, row := range records[1:] {
			if len(row) < 3 {
				continue
			}

			parsedDate, err := time.Parse("2006-01-02", row[0])
			if err != nil {
				continue
			}

			amountStr := strings.ReplaceAll(row[2], ",", ".")
			amount, err := strconv.ParseFloat(amountStr, 64)
			if err != nil {
				continue
			}

			title := strings.TrimSpace(row[1])

			query := `
				INSERT INTO transactions (date, title, amount) 
				VALUES ($1, $2, $3) 
				ON CONFLICT (date, title, amount) DO NOTHING;`

			cmd, err := db.Exec(ctx, query, parsedDate, title, amount)
			if err == nil && cmd.RowsAffected() > 0 {
				importedCount++
			}
		}
	}

	fmt.Printf("Importação concluída. %d novas transações registradas.\n", importedCount)
}

func classifyTransactions(ctx context.Context, db *pgxpool.Pool) {
	queryMap := `SELECT title_pattern, category FROM category_mappings`
	rows, err := db.Query(ctx, queryMap)
	if err != nil {
		fmt.Printf("Erro ao carregar regras de categoria: %v\n", err)
		return
	}
	defer rows.Close()

	mappings := make(map[string]string)
	for rows.Next() {
		var pattern, category string
		if err := rows.Scan(&pattern, &category); err == nil {
			mappings[strings.ToLower(pattern)] = category
		}
	}

	txRows, err := db.Query(ctx, `SELECT id, title FROM transactions WHERE category = 'Não Classificado'`)
	if err != nil {
		fmt.Printf("Erro ao buscar transações: %v\n", err)
		return
	}

	type match struct {
		id       int
		category string
	}
	var updates []match
	var unclassified []string

	for txRows.Next() {
		var id int
		var title string
		if err := txRows.Scan(&id, &title); err == nil {
			matched := false
			lowerTitle := strings.ToLower(title)

			for pattern, category := range mappings {
				if strings.Contains(lowerTitle, pattern) {
					updates = append(updates, match{id: id, category: category})
					matched = true
					break
				}
			}

			if !matched {
				unclassified = append(unclassified, title)
			}
		}
	}
	txRows.Close()

	classifiedCount := 0
	for _, u := range updates {
		_, err := db.Exec(ctx, `UPDATE transactions SET category = $1 WHERE id = $2`, u.category, u.id)
		if err == nil {
			classifiedCount++
		}
	}

	fmt.Printf("Classificação finalizada. %d itens classificados.\n", classifiedCount)

	if len(unclassified) > 0 {
		fmt.Println("\nGastos ainda não classificados:")
		uniqueUnclassified := uniqueStrings(unclassified)
		for _, title := range uniqueUnclassified {
			fmt.Printf(" - %s\n", title)
		}
	}
}

func addCategoryMapping(ctx context.Context, db *pgxpool.Pool, rawInput string) {
	// Extrai texto entre aspas para o padrão e pega a última palavra como categoria
	firstQuote := strings.Index(rawInput, "\"")
	lastQuote := strings.LastIndex(rawInput, "\"")

	if firstQuote == -1 || lastQuote == -1 || firstQuote == lastQuote {
		fmt.Println("Erro no formato. Use aspas no título: /categoria \"Mercado Lizza\" Mercado")
		return
	}

	pattern := strings.TrimSpace(rawInput[firstQuote+1 : lastQuote])
	remaining := strings.TrimSpace(rawInput[lastQuote+1:])

	if pattern == "" || remaining == "" {
		fmt.Println("Erro no formato. Certifique-se de passar o título e a categoria.")
		return
	}

	category := strings.Fields(remaining)[0]

	query := `
		INSERT INTO category_mappings (title_pattern, category) 
		VALUES ($1, $2)
		ON CONFLICT (title_pattern) DO UPDATE SET category = EXCLUDED.category;`

	_, err := db.Exec(ctx, query, pattern, category)
	if err != nil {
		fmt.Printf("Erro ao salvar regra: %v\n", err)
		return
	}

	fmt.Printf("Regra salva com sucesso: Qualquer gasto contendo \"%s\" será classificado como \"%s\".\n", pattern, category)
}

func uniqueStrings(input []string) []string {
	keys := make(map[string]bool)
	var list []string
	for _, entry := range input {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}
