package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"minhas-financas/internal/category"
	"minhas-financas/internal/dashboard"
	"minhas-financas/internal/expense"
	"minhas-financas/internal/income"
	"minhas-financas/internal/referencemonth"
)

type fakeDatabase struct {
	err               error
	createdCategory   category.Category
	createCategoryErr error
}

func (db fakeDatabase) Ping(context.Context) error {
	return db.err
}

func (db fakeDatabase) CreateCategory(_ context.Context, name string) (category.Category, error) {
	if db.createCategoryErr != nil {
		return category.Category{}, db.createCategoryErr
	}
	created := db.createdCategory
	created.Name = strings.TrimSpace(name)
	return created, nil
}

func (db fakeDatabase) ListCategories(context.Context) ([]category.Category, error) {
	return []category.Category{db.createdCategory}, nil
}

func (db fakeDatabase) Summary(context.Context, int64) (dashboard.Summary, error) {
	return dashboard.Summary{
		Income: "0.00", Expenses: "25.50", Balance: "-25.50",
		Categories: []dashboard.CategoryGroup{},
	}, db.err
}

type fakeExpenseRepository struct {
	expense        expense.Expense
	expenses       []expense.Expense
	err            error
	updated        int64
	expenseID      int64
	categoryID     int64
	source         string
	updatedExpense expense.Expense
	deletedID      int64
	listedMonthID  *int64
}

func (r fakeExpenseRepository) CreateExpense(_ context.Context, referenceMonthID int64, name, amount string) (expense.Expense, error) {
	if r.err != nil {
		return expense.Expense{}, r.err
	}
	created := r.expense
	created.Name = name
	created.Valor = amount
	created.ReferenceMonthID = referenceMonthID
	created.Source = expense.SourceManual
	return created, nil
}

func (r *fakeExpenseRepository) ListExpenses(_ context.Context, referenceMonthID *int64) ([]expense.Expense, error) {
	r.listedMonthID = referenceMonthID
	if r.expenses != nil {
		return r.expenses, nil
	}
	return []expense.Expense{r.expense}, nil
}

func (r *fakeExpenseRepository) UpdateExpense(_ context.Context, expenseID, referenceMonthID int64, name, amount string) (expense.Expense, error) {
	r.expenseID = expenseID
	if r.err != nil {
		return expense.Expense{}, r.err
	}
	r.updatedExpense = expense.Expense{
		ID: expenseID, Name: name, Valor: amount, ReferenceMonthID: referenceMonthID, Source: expense.SourceManual,
	}
	return r.updatedExpense, nil
}

func (r *fakeExpenseRepository) DeleteExpense(_ context.Context, expenseID int64) error {
	r.deletedID = expenseID
	return r.err
}

func (r *fakeExpenseRepository) AssignCategory(_ context.Context, expenseID, categoryID int64, source string) (int64, error) {
	r.expenseID = expenseID
	r.categoryID = categoryID
	r.source = source
	return r.updated, r.err
}

type fakeIncomeRepository struct {
	income           income.Income
	referenceMonthID int64
	name             string
	amount           string
	err              error
}

func (r *fakeIncomeRepository) CreateIncome(_ context.Context, referenceMonthID int64, name, amount string) (income.Income, error) {
	r.referenceMonthID = referenceMonthID
	r.name = name
	r.amount = amount
	if r.err != nil {
		return income.Income{}, r.err
	}
	created := r.income
	created.ReferenceMonthID = referenceMonthID
	created.Name = name
	created.Amount = amount
	return created, nil
}

type fakeImporter struct {
	count            int
	referenceMonthID int64
	calls            int
	err              error
}

type fakeReferenceMonthManager struct {
	months []referencemonth.ReferenceMonth
	err    error
}

func (m fakeReferenceMonthManager) List(context.Context) ([]referencemonth.ReferenceMonth, error) {
	return m.months, m.err
}

func (m fakeReferenceMonthManager) Create(_ context.Context, name string) (referencemonth.ReferenceMonth, error) {
	if m.err != nil {
		return referencemonth.ReferenceMonth{}, m.err
	}
	return referencemonth.ReferenceMonth{ID: 1, Name: strings.TrimSpace(name)}, nil
}

func (m fakeReferenceMonthManager) Update(_ context.Context, id int64, name string) (referencemonth.ReferenceMonth, error) {
	if m.err != nil {
		return referencemonth.ReferenceMonth{}, m.err
	}
	return referencemonth.ReferenceMonth{ID: id, Name: strings.TrimSpace(name)}, nil
}

func (m fakeReferenceMonthManager) Delete(context.Context, int64) error {
	return m.err
}

type fakeCategoryRepository struct{}

func (fakeCategoryRepository) CreateCategory(_ context.Context, name string) (category.Category, error) {
	return category.Category{ID: 1, Name: name}, nil
}

func (fakeCategoryRepository) ListCategories(context.Context) ([]category.Category, error) {
	return []category.Category{{ID: 1, Name: "Alimentação"}}, nil
}

type fakeExpenseServiceRepository struct{}

func (fakeExpenseServiceRepository) CreateExpense(_ context.Context, referenceMonthID int64, name, amount string) (expense.Expense, error) {
	return expense.Expense{Name: name, Valor: amount, ReferenceMonthID: referenceMonthID, Source: expense.SourceManual}, nil
}

func (fakeExpenseServiceRepository) ListExpenses(context.Context, *int64) ([]expense.Expense, error) {
	return []expense.Expense{}, nil
}

func (fakeExpenseServiceRepository) UpdateExpense(_ context.Context, expenseID, referenceMonthID int64, name, amount string) (expense.Expense, error) {
	return expense.Expense{
		ID: expenseID, ReferenceMonthID: referenceMonthID, Name: name, Valor: amount, Source: expense.SourceManual,
	}, nil
}

func (fakeExpenseServiceRepository) DeleteExpense(context.Context, int64) error {
	return nil
}

func (fakeExpenseServiceRepository) AssignCategory(context.Context, int64, int64, string) (int64, error) {
	return 1, nil
}

func testHandler(db databasePinger, importer csvImporter) *gin.Engine {
	return newHandler(
		db,
		importer,
		category.NewService(fakeCategoryRepository{}),
		expense.NewService(fakeExpenseServiceRepository{}),
		income.NewService(&fakeIncomeRepository{}),
		dashboard.NewService(fakeDatabase{}),
		fakeReferenceMonthManager{months: []referencemonth.ReferenceMonth{{ID: 1, Name: "Teste"}}},
	)
}

func setGinTestMode(t *testing.T) {
	t.Helper()
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		gin.SetMode(previousMode)
	})
}

func (i *fakeImporter) ImportCSV(_ context.Context, _ []byte, referenceMonthID int64) (int, error) {
	i.referenceMonthID = referenceMonthID
	i.calls++
	return i.count, i.err
}

func TestHealthEndpoint(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		gin.SetMode(previousMode)
	})

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	testHandler(fakeDatabase{}, &fakeImporter{}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if response.Body.String() != "ok\n" {
		t.Fatalf("expected body %q, got %q", "ok\n", response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/plain; charset=utf-8" {
		t.Fatalf("expected content type %q, got %q", "text/plain; charset=utf-8", contentType)
	}
}

func TestFrontendPagesAndStylesAreServed(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		gin.SetMode(previousMode)
	})

	router := testHandler(fakeDatabase{}, &fakeImporter{})
	tests := []struct {
		path        string
		contentType string
		contains    string
	}{
		{path: "/", contentType: "text/html", contains: "Visão geral"},
		{path: "/settings", contentType: "text/html", contains: "Meses de referência"},
		{path: "/assets/styles.css", contentType: "text/css", contains: "--green"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
			}
			if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, test.contentType) {
				t.Fatalf("expected content type containing %q, got %q", test.contentType, contentType)
			}
			if !strings.Contains(response.Body.String(), test.contains) {
				t.Fatalf("response does not contain %q", test.contains)
			}
		})
	}
}

func TestHealthEndpointWhenDatabaseIsUnavailable(t *testing.T) {
	setGinTestMode(t)

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()
	testHandler(fakeDatabase{err: errors.New("database unavailable")}, &fakeImporter{}).ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, response.Code)
	}
	if response.Body.String() != "database unavailable\n" {
		t.Fatalf("unexpected response body %q", response.Body.String())
	}
}

func TestNubankImportEndpoint(t *testing.T) {
	setGinTestMode(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("referenceMonthId", "1"); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("file", "transactions.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(file, "date,title,amount\n2026-01-02,Example,\"12,34\"\n"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/nubank/import", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()

	importer := &fakeImporter{count: 1}
	testHandler(fakeDatabase{}, importer).ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"imported":1`) {
		t.Fatalf("unexpected response body %q", response.Body.String())
	}
	if importer.referenceMonthID != 1 {
		t.Fatalf("expected selected reference month ID 1, got %d", importer.referenceMonthID)
	}
}

func TestNubankImportEndpointRejectsNonCSVFile(t *testing.T) {
	setGinTestMode(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("referenceMonthId", "1"); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("file", "document.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(file, "not a CSV file"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/nubank/import", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	testHandler(fakeDatabase{}, &fakeImporter{}).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
}

func TestNubankImportEndpointAllowsRepeatedImports(t *testing.T) {
	setGinTestMode(t)

	importer := &fakeImporter{count: 1}
	for call := 1; call <= 2; call++ {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if err := writer.WriteField("referenceMonthId", "1"); err != nil {
			t.Fatal(err)
		}
		file, err := writer.CreateFormFile("file", "transactions.csv")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(file, "date,title,amount\n2026-01-02,Example,\"12,34\"\n"); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}

		request := httptest.NewRequest(http.MethodPost, "/nubank/import", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		response := httptest.NewRecorder()
		testHandler(fakeDatabase{}, importer).ServeHTTP(response, request)

		if response.Code != http.StatusCreated {
			t.Fatalf("call %d: expected status %d, got %d: %s", call, http.StatusCreated, response.Code, response.Body.String())
		}
	}
	if importer.calls != 2 {
		t.Fatalf("expected both imports to reach the importer, got %d calls", importer.calls)
	}
}

func TestCreateCategoryEndpoint(t *testing.T) {
	setGinTestMode(t)

	request := httptest.NewRequest(http.MethodPost, "/api/categories", strings.NewReader(`{"name":"  Alimentação  "}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	db := fakeDatabase{createdCategory: category.Category{ID: 12}}
	newHandler(db, &fakeImporter{}, category.NewService(db), expense.NewService(fakeExpenseServiceRepository{}), income.NewService(&fakeIncomeRepository{}), dashboard.NewService(db), fakeReferenceMonthManager{}).ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, response.Code, response.Body.String())
	}
	if response.Body.String() != "{\"id\":12,\"name\":\"Alimentação\"}" {
		t.Fatalf("unexpected response body %q", response.Body.String())
	}
}

func TestCreateCategoryEndpointRejectsInvalidNames(t *testing.T) {
	setGinTestMode(t)

	tests := []struct {
		name string
		body string
	}{
		{name: "empty", body: `{"name":"   "}`},
		{name: "too long", body: `{"name":"` + strings.Repeat("a", 101) + `"}`},
		{name: "invalid JSON", body: `{"name":`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/categories", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			service := category.NewService(fakeCategoryRepository{})
			newHandler(fakeDatabase{}, &fakeImporter{}, service, expense.NewService(fakeExpenseServiceRepository{}), income.NewService(&fakeIncomeRepository{}), dashboard.NewService(fakeDatabase{}), fakeReferenceMonthManager{}).ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
			}
		})
	}
}

func TestCreateCategoryEndpointReportsDuplicate(t *testing.T) {
	setGinTestMode(t)

	request := httptest.NewRequest(http.MethodPost, "/api/categories", strings.NewReader(`{"name":"Alimentação"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	db := fakeDatabase{createCategoryErr: category.ErrAlreadyExists}
	newHandler(db, &fakeImporter{}, category.NewService(db), expense.NewService(fakeExpenseServiceRepository{}), income.NewService(&fakeIncomeRepository{}), dashboard.NewService(db), fakeReferenceMonthManager{}).ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, response.Code)
	}
}

func TestCreateExpenseEndpoint(t *testing.T) {
	setGinTestMode(t)

	request := httptest.NewRequest(http.MethodPost, "/api/expenses", strings.NewReader(`{"referenceMonthId":7,"name":"  Mercado  ","valor":"1.234,56"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	repository := &fakeExpenseRepository{expense: expense.Expense{ID: 8}}
	newHandler(fakeDatabase{}, &fakeImporter{}, category.NewService(fakeCategoryRepository{}), expense.NewService(repository), income.NewService(&fakeIncomeRepository{}), dashboard.NewService(fakeDatabase{}), fakeReferenceMonthManager{}).ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, response.Code, response.Body.String())
	}
	if response.Body.String() != "{\"id\":8,\"name\":\"Mercado\",\"valor\":\"1234.56\",\"referenceMonthId\":7,\"categoryId\":null,\"source\":\"manual\"}" {
		t.Fatalf("unexpected response body %q", response.Body.String())
	}
}

func TestUpdateExpenseEndpoint(t *testing.T) {
	setGinTestMode(t)
	repository := &fakeExpenseRepository{}
	router := newHandler(
		fakeDatabase{},
		&fakeImporter{},
		category.NewService(fakeCategoryRepository{}),
		expense.NewService(repository),
		income.NewService(&fakeIncomeRepository{}),
		dashboard.NewService(fakeDatabase{}),
		fakeReferenceMonthManager{},
	)
	request := httptest.NewRequest(http.MethodPatch, "/api/expenses/8", strings.NewReader(
		`{"referenceMonthId":7,"name":" Mercado ","valor":"1.234,5"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	if response.Body.String() != "{\"id\":8,\"name\":\"Mercado\",\"valor\":\"1234.50\",\"referenceMonthId\":7,\"categoryId\":null,\"source\":\"manual\"}" {
		t.Fatalf("unexpected response body %q", response.Body.String())
	}
	if repository.updatedExpense.ID != 8 || repository.updatedExpense.ReferenceMonthID != 7 {
		t.Fatalf("unexpected updated expense: %#v", repository.updatedExpense)
	}
}

func TestUpdateExpenseEndpointRejectsInvalidAndMissingExpenses(t *testing.T) {
	setGinTestMode(t)
	tests := []struct {
		name       string
		path       string
		body       string
		repository *fakeExpenseRepository
		wantStatus int
	}{
		{name: "invalid ID", path: "/api/expenses/invalid", wantStatus: http.StatusBadRequest},
		{name: "invalid data", path: "/api/expenses/8", body: `{"referenceMonthId":7,"name":" ","valor":"10,00"}`, wantStatus: http.StatusBadRequest},
		{name: "missing expense", path: "/api/expenses/8", body: `{"referenceMonthId":7,"name":"Mercado","valor":"10,00"}`, repository: &fakeExpenseRepository{err: expense.ErrNotFound}, wantStatus: http.StatusNotFound},
		{name: "missing month", path: "/api/expenses/8", body: `{"referenceMonthId":7,"name":"Mercado","valor":"10,00"}`, repository: &fakeExpenseRepository{err: expense.ErrReferenceMonthNotFound}, wantStatus: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := test.repository
			if repository == nil {
				repository = &fakeExpenseRepository{}
			}
			router := newHandler(
				fakeDatabase{},
				&fakeImporter{},
				category.NewService(fakeCategoryRepository{}),
				expense.NewService(repository),
				income.NewService(&fakeIncomeRepository{}),
				dashboard.NewService(fakeDatabase{}),
				fakeReferenceMonthManager{},
			)
			request := httptest.NewRequest(http.MethodPatch, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", test.wantStatus, response.Code, response.Body.String())
			}
		})
	}
}

func TestDeleteExpenseEndpoint(t *testing.T) {
	setGinTestMode(t)
	repository := &fakeExpenseRepository{}
	router := newHandler(
		fakeDatabase{},
		&fakeImporter{},
		category.NewService(fakeCategoryRepository{}),
		expense.NewService(repository),
		income.NewService(&fakeIncomeRepository{}),
		dashboard.NewService(fakeDatabase{}),
		fakeReferenceMonthManager{},
	)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/expenses/8", nil))

	if response.Code != http.StatusNoContent || repository.deletedID != 8 {
		t.Fatalf("unexpected delete response: status=%d body=%s deletedID=%d", response.Code, response.Body.String(), repository.deletedID)
	}

	repository = &fakeExpenseRepository{err: expense.ErrNotFound}
	router = newHandler(
		fakeDatabase{},
		&fakeImporter{},
		category.NewService(fakeCategoryRepository{}),
		expense.NewService(repository),
		income.NewService(&fakeIncomeRepository{}),
		dashboard.NewService(fakeDatabase{}),
		fakeReferenceMonthManager{},
	)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/expenses/8", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected missing expense status %d, got %d: %s", http.StatusNotFound, response.Code, response.Body.String())
	}
}

func TestExpenseCategoryEndpoints(t *testing.T) {
	setGinTestMode(t)
	repository := &fakeExpenseRepository{
		expense: expense.Expense{ID: 8, Name: "Desp A", Valor: "20.00"},
		expenses: []expense.Expense{
			{ID: 8, Name: "Desp A", Valor: "20.00", Source: expense.SourceManual},
			{ID: 8, Name: "Desp A", Valor: "30.00", Source: expense.SourceNubank},
		},
		updated: 2,
	}
	router := newHandler(
		fakeDatabase{createdCategory: category.Category{ID: 3, Name: "cat01"}},
		&fakeImporter{},
		category.NewService(fakeCategoryRepository{}),
		expense.NewService(repository),
		income.NewService(&fakeIncomeRepository{}),
		dashboard.NewService(fakeDatabase{}),
		fakeReferenceMonthManager{},
	)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/categories", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"Alimentação"`) {
		t.Fatalf("unexpected category list response: status=%d body=%s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/expenses?referenceMonthId=7", nil))
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"source":"manual"`) ||
		!strings.Contains(response.Body.String(), `"source":"nubank"`) {
		t.Fatalf("unexpected expense list response: status=%d body=%s", response.Code, response.Body.String())
	}
	if repository.listedMonthID == nil || *repository.listedMonthID != 7 {
		t.Fatalf("expected expense list to use reference month 7, got %v", repository.listedMonthID)
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/expenses?referenceMonthId=invalid", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid month status %d, got %d", http.StatusBadRequest, response.Code)
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/expenses", nil))
	if response.Code != http.StatusOK || repository.listedMonthID != nil {
		t.Fatalf("unexpected unfiltered expense list response: status=%d month=%v", response.Code, repository.listedMonthID)
	}

	request := httptest.NewRequest(http.MethodPatch, "/api/expenses/8/category", strings.NewReader(`{"categoryId":3,"source":"nubank"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "{\"updated\":2}" {
		t.Fatalf("unexpected expense category response: status=%d body=%s", response.Code, response.Body.String())
	}
	if repository.expenseID != 8 || repository.categoryID != 3 || repository.source != expense.SourceNubank {
		t.Fatalf("unexpected assignment: expense=%d category=%d source=%s", repository.expenseID, repository.categoryID, repository.source)
	}
}

func TestDashboardSummaryEndpoint(t *testing.T) {
	setGinTestMode(t)
	router := testHandler(fakeDatabase{}, &fakeImporter{})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/dashboard/summary?referenceMonthId=7", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, response.Code, response.Body.String())
	}
	if response.Body.String() != "{\"income\":\"0.00\",\"expenses\":\"25.50\",\"balance\":\"-25.50\",\"expenseCount\":0,\"categoryCount\":0,\"categories\":[]}" {
		t.Fatalf("unexpected summary response %q", response.Body.String())
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/dashboard/summary", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d for missing month, got %d", http.StatusBadRequest, response.Code)
	}
}

func TestCreateIncomeEndpoint(t *testing.T) {
	setGinTestMode(t)
	repository := &fakeIncomeRepository{income: income.Income{ID: 9}}
	router := newHandler(
		fakeDatabase{},
		&fakeImporter{},
		category.NewService(fakeCategoryRepository{}),
		expense.NewService(fakeExpenseServiceRepository{}),
		income.NewService(repository),
		dashboard.NewService(fakeDatabase{}),
		fakeReferenceMonthManager{},
	)
	request := httptest.NewRequest(http.MethodPost, "/api/incomes", strings.NewReader(
		`{"referenceMonthId":7,"name":" Salário ","amount":"1.234,5"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, response.Code, response.Body.String())
	}
	if response.Body.String() != "{\"id\":9,\"name\":\"Salário\",\"amount\":\"1234.50\",\"referenceMonthId\":7}" {
		t.Fatalf("unexpected income response %q", response.Body.String())
	}
	if repository.referenceMonthID != 7 || repository.amount != "1234.50" {
		t.Fatalf("unexpected income repository input: %#v", repository)
	}
}

func TestCreateIncomeEndpointRejectsInvalidRequest(t *testing.T) {
	setGinTestMode(t)
	tests := []struct {
		name string
		body string
	}{
		{name: "missing month", body: `{"name":"Salário","amount":"100,00"}`},
		{name: "invalid amount", body: `{"referenceMonthId":1,"name":"Salário","amount":"-100,00"}`},
		{name: "empty name", body: `{"referenceMonthId":1,"name":" ","amount":"100,00"}`},
		{name: "invalid JSON", body: `{"referenceMonthId":`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := testHandler(fakeDatabase{}, &fakeImporter{})
			request := httptest.NewRequest(http.MethodPost, "/api/incomes", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, response.Code, response.Body.String())
			}
		})
	}
}

func TestCreateExpenseEndpointRejectsInvalidRequests(t *testing.T) {
	setGinTestMode(t)

	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: `{"name":`},
		{name: "missing month", body: `{"name":"Mercado","valor":"10,00"}`},
		{name: "empty name", body: `{"referenceMonthId":1,"name":"  ","valor":"10,00"}`},
		{name: "invalid amount", body: `{"referenceMonthId":1,"name":"Mercado","valor":"-10,00"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/expenses", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			newHandler(fakeDatabase{}, &fakeImporter{}, category.NewService(fakeCategoryRepository{}), expense.NewService(&fakeExpenseRepository{}), income.NewService(&fakeIncomeRepository{}), dashboard.NewService(fakeDatabase{}), fakeReferenceMonthManager{}).ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, response.Code, response.Body.String())
			}
		})
	}
}

func TestCreateExpenseEndpointReturnsNotFoundForUnknownReferenceMonth(t *testing.T) {
	setGinTestMode(t)
	request := httptest.NewRequest(http.MethodPost, "/api/expenses", strings.NewReader(
		`{"referenceMonthId":7,"name":"Mercado","valor":"10,00"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router := newHandler(
		fakeDatabase{},
		&fakeImporter{},
		category.NewService(fakeCategoryRepository{}),
		expense.NewService(&fakeExpenseRepository{err: expense.ErrReferenceMonthNotFound}),
		income.NewService(&fakeIncomeRepository{}),
		dashboard.NewService(fakeDatabase{}),
		fakeReferenceMonthManager{},
	)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d: %s", http.StatusNotFound, response.Code, response.Body.String())
	}
}

func TestReferenceMonthEndpoints(t *testing.T) {
	setGinTestMode(t)
	manager := fakeReferenceMonthManager{
		months: []referencemonth.ReferenceMonth{
			{ID: 2, Name: "Novembro de 2026"},
			{ID: 1, Name: "Outubro de 2026"},
		},
	}
	router := newHandler(
		fakeDatabase{},
		&fakeImporter{},
		category.NewService(fakeCategoryRepository{}),
		expense.NewService(fakeExpenseServiceRepository{}),
		income.NewService(&fakeIncomeRepository{}),
		dashboard.NewService(fakeDatabase{}),
		manager,
	)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/reference-months", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Novembro de 2026") {
		t.Fatalf("unexpected list response: status=%d body=%s", response.Code, response.Body.String())
	}

	request := httptest.NewRequest(http.MethodPost, "/api/reference-months", strings.NewReader(`{"name":" Janeiro de 2027 "}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), "Janeiro de 2027") {
		t.Fatalf("unexpected create response: status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPatch, "/api/reference-months/2", strings.NewReader(`{"name":"Fim de ano"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Fim de ano") {
		t.Fatalf("unexpected update response: status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/reference-months/2", nil)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected delete status %d, got %d", http.StatusNoContent, response.Code)
	}
}

func TestNubankImportRequiresReferenceMonth(t *testing.T) {
	setGinTestMode(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "transactions.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(file, "date,title,amount\n2026-01-02,Example,\"12,34\"\n"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/nubank/import", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	testHandler(fakeDatabase{}, &fakeImporter{}).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
}
