package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed web/*.html
var dashboardFiles embed.FS

type dashboardData struct {
	IncomeCents   int64               `json:"incomeCents"`
	ExpenseCents  int64               `json:"expenseCents"`
	BalanceCents  int64               `json:"balanceCents"`
	Categories    []dashboardCategory `json:"categories"`
	Uncategorized []dashboardExpense  `json:"uncategorized"`
}

type dashboardCategory struct {
	Name       string             `json:"name"`
	TotalCents int64              `json:"totalCents"`
	Expenses   []dashboardExpense `json:"expenses"`
}

type dashboardExpense struct {
	Name       string `json:"name"`
	TotalCents int64  `json:"totalCents"`
}

type dashboardRow struct {
	CategoryName  string
	Uncategorized bool
	Title         string
	AmountCents   int64
}

type settingsCategory struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type expenseCategoryRule struct {
	ID           int64  `json:"id"`
	ExpenseName  string `json:"expenseName"`
	CategoryID   int64  `json:"categoryId"`
	CategoryName string `json:"categoryName"`
}

type manualExpenseRequest struct {
	Month  string `json:"month"`
	Day    int    `json:"day"`
	Title  string `json:"title"`
	Amount string `json:"amount"`
}

func serveDashboard(db *pgxpool.Pool) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFileFS(w, r, dashboardFiles, "web/index.html")
	})
	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFileFS(w, r, dashboardFiles, "web/settings.html")
	})
	mux.HandleFunc("GET /api/dashboard", func(w http.ResponseWriter, r *http.Request) {
		data, err := loadDashboardData(r.Context(), db)
		if err != nil {
			http.Error(w, "Não foi possível carregar os dados do dashboard.", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(data); err != nil {
			fmt.Printf("Erro ao enviar dados do dashboard: %v\n", err)
		}
	})
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		categories, err := getSettingsCategories(r.Context(), db)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "Não foi possível carregar as categorias.")
			return
		}
		rules, err := getExpenseCategoryRules(r.Context(), db)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "Não foi possível carregar os vínculos.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"categories": categories, "expenseCategories": rules})
	})
	mux.HandleFunc("POST /api/categories", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Name string `json:"name"`
		}
		if !decodeJSONBody(w, r, &request) {
			return
		}
		name := strings.TrimSpace(request.Name)
		if name == "" {
			writeAPIError(w, http.StatusBadRequest, "O nome da categoria não pode ficar vazio.")
			return
		}
		if err := createCategory(r.Context(), db, name); err != nil {
			writeAPIError(w, http.StatusConflict, "Não foi possível cadastrar a categoria. Verifique se o nome já existe.")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"message": "Categoria cadastrada."})
	})
	mux.HandleFunc("DELETE /api/categories/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			writeAPIError(w, http.StatusBadRequest, "ID de categoria inválido.")
			return
		}
		if err := removeCategory(r.Context(), db, id); err != nil {
			writeAPIError(w, http.StatusNotFound, "Categoria não encontrada ou não foi possível excluí-la.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/expense-categories", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			CategoryID  int64  `json:"categoryId"`
			ExpenseName string `json:"expenseName"`
		}
		if !decodeJSONBody(w, r, &request) {
			return
		}
		request.ExpenseName = strings.TrimSpace(request.ExpenseName)
		if request.CategoryID < 1 || request.ExpenseName == "" {
			writeAPIError(w, http.StatusBadRequest, "Selecione uma categoria e informe o nome da despesa.")
			return
		}
		_, err := db.Exec(r.Context(),
			`INSERT INTO categoria_despesa (id_categoria, nome_despesas) VALUES ($1, $2)
			 ON CONFLICT (nome_despesas) DO UPDATE SET id_categoria = EXCLUDED.id_categoria`,
			request.CategoryID, request.ExpenseName,
		)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "Não foi possível vincular a despesa. Confira a categoria selecionada.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "Vínculo salvo."})
	})
	mux.HandleFunc("POST /api/transactions/categorize", func(w http.ResponseWriter, r *http.Request) {
		categorized, uncategorized, err := categorizeTransactions(r.Context(), db)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "Não foi possível categorizar os lançamentos.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{
			"categorized":   categorized,
			"uncategorized": uncategorized,
		})
	})
	mux.HandleFunc("POST /api/expenses", func(w http.ResponseWriter, r *http.Request) {
		var request manualExpenseRequest
		if !decodeJSONBody(w, r, &request) {
			return
		}
		expenseDate, referenceMonth, amount, err := validateManualExpense(request)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		if _, err := db.Exec(r.Context(),
			`INSERT INTO transactions (date, title, amount, statement_month, origin) VALUES ($1, $2, $3, $4, 'MANUAL')`,
			expenseDate, strings.TrimSpace(request.Title), amount, referenceMonth,
		); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "Não foi possível cadastrar a despesa manual.")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"message": "Despesa manual cadastrada."})
	})

	server := &http.Server{
		Addr:              "127.0.0.1:8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Printf("Dashboard disponível em http://%s\n", server.Addr)
	return server.ListenAndServe()
}

func getSettingsCategories(ctx context.Context, db *pgxpool.Pool) ([]settingsCategory, error) {
	items, err := getCategories(ctx, db)
	if err != nil {
		return nil, err
	}
	categories := make([]settingsCategory, 0, len(items))
	for _, item := range items {
		categories = append(categories, settingsCategory{ID: item.id, Name: item.name})
	}
	return categories, nil
}

func getExpenseCategoryRules(ctx context.Context, db *pgxpool.Pool) ([]expenseCategoryRule, error) {
	rows, err := db.Query(ctx, `
		SELECT cd.id, cd.nome_despesas, cd.id_categoria, c.nome
		FROM categoria_despesa cd
		JOIN categoria c ON c.id = cd.id_categoria
		ORDER BY c.nome, cd.nome_despesas`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]expenseCategoryRule, 0)
	for rows.Next() {
		var rule expenseCategoryRule
		if err := rows.Scan(&rule.ID, &rule.ExpenseName, &rule.CategoryID, &rule.CategoryName); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func removeCategory(ctx context.Context, db *pgxpool.Pool, id int64) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var categoryName string
	if err := tx.QueryRow(ctx, `SELECT nome FROM categoria WHERE id = $1 FOR UPDATE`, id).Scan(&categoryName); err != nil {
		return err
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
	return tx.Commit(ctx)
}

func validateManualExpense(request manualExpenseRequest) (time.Time, time.Time, float64, error) {
	monthText := strings.TrimSpace(request.Month)
	referenceMonth, err := time.Parse("2006-01", monthText)
	if err != nil || referenceMonth.Format("2006-01") != monthText {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("mês inválido; use AAAA-MM")
	}
	if !isValidExpenseDay(referenceMonth, request.Day) {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("dia inválido para o mês de referência")
	}
	title := strings.TrimSpace(request.Title)
	if title == "" {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("o nome da despesa não pode ficar vazio")
	}
	amount, err := parseAmount(request.Amount)
	if err != nil {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("valor inválido: %w", err)
	}
	date := time.Date(referenceMonth.Year(), referenceMonth.Month(), request.Day, 0, 0, 0, 0, time.UTC)
	return date, referenceMonth, amount, nil
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, destination any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeAPIError(w, http.StatusUnsupportedMediaType, "O conteúdo deve ser enviado em JSON.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Requisição JSON inválida.")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		fmt.Printf("Erro ao enviar resposta JSON: %v\n", err)
	}
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func loadDashboardData(ctx context.Context, db *pgxpool.Pool) (dashboardData, error) {
	rows, err := db.Query(ctx, `
		SELECT
			COALESCE(c.nome, ''),
			c.id IS NULL,
			COALESCE(NULLIF(BTRIM(t.title), ''), 'Despesa sem descrição'),
			SUM(ROUND(t.amount * 100))::BIGINT
		FROM transactions t
		LEFT JOIN categoria c ON c.id = t.id_categoria
		WHERE t.amount > 0
		GROUP BY c.id, c.nome, COALESCE(NULLIF(BTRIM(t.title), ''), 'Despesa sem descrição')
		ORDER BY c.id IS NULL, c.nome, SUM(ROUND(t.amount * 100)) DESC,
			COALESCE(NULLIF(BTRIM(t.title), ''), 'Despesa sem descrição')`)
	if err != nil {
		return dashboardData{}, err
	}
	defer rows.Close()

	var dashboardRows []dashboardRow
	for rows.Next() {
		var row dashboardRow
		if err := rows.Scan(&row.CategoryName, &row.Uncategorized, &row.Title, &row.AmountCents); err != nil {
			return dashboardData{}, err
		}
		dashboardRows = append(dashboardRows, row)
	}
	if err := rows.Err(); err != nil {
		return dashboardData{}, err
	}
	return buildDashboardData(dashboardRows), nil
}

func buildDashboardData(rows []dashboardRow) dashboardData {
	data := dashboardData{
		Categories:    make([]dashboardCategory, 0),
		Uncategorized: make([]dashboardExpense, 0),
	}
	currentCategoryIndex := -1

	for _, row := range rows {
		data.ExpenseCents += row.AmountCents
		expense := dashboardExpense{Name: row.Title, TotalCents: row.AmountCents}
		if row.Uncategorized {
			data.Uncategorized = append(data.Uncategorized, expense)
			continue
		}

		if currentCategoryIndex < 0 || data.Categories[currentCategoryIndex].Name != row.CategoryName {
			data.Categories = append(data.Categories, dashboardCategory{
				Name:     row.CategoryName,
				Expenses: make([]dashboardExpense, 0),
			})
			currentCategoryIndex = len(data.Categories) - 1
		}
		data.Categories[currentCategoryIndex].Expenses = append(data.Categories[currentCategoryIndex].Expenses, expense)
		data.Categories[currentCategoryIndex].TotalCents += row.AmountCents
	}

	data.BalanceCents = data.IncomeCents - data.ExpenseCents
	return data
}
