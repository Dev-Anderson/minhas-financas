package main

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"minhas-financas/internal/category"
	"minhas-financas/internal/dashboard"
	"minhas-financas/internal/expense"
	"minhas-financas/internal/income"
	"minhas-financas/internal/infrastructure/postgres"
	"minhas-financas/internal/nubank"
	"minhas-financas/internal/referencemonth"
	frontend "minhas-financas/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db, err := postgres.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	server := &http.Server{
		Addr: ":" + port,
		Handler: newHandler(
			db,
			nubank.NewImporter(db),
			category.NewService(db),
			expense.NewService(db),
			income.NewService(db),
			dashboard.NewService(db),
			referencemonth.NewService(db),
		),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("API listening on :%s", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

type databasePinger interface {
	Ping(context.Context) error
}

type csvImporter interface {
	ImportCSV(context.Context, []byte, int64) (int, error)
}

type categoryCreator interface {
	Create(context.Context, string) (category.Category, error)
	List(context.Context) ([]category.Category, error)
}

type expenseCreator interface {
	Create(context.Context, string, string) (expense.Expense, error)
	List(context.Context) ([]expense.Expense, error)
	AssignCategory(context.Context, int64, int64, string) (int64, error)
}

type incomeCreator interface {
	Create(context.Context, int64, string, string) (income.Income, error)
}

type dashboardSummaryReader interface {
	GetSummary(context.Context, int64) (dashboard.Summary, error)
}

type referenceMonthManager interface {
	List(context.Context) ([]referencemonth.ReferenceMonth, error)
	Create(context.Context, string) (referencemonth.ReferenceMonth, error)
	Update(context.Context, int64, string) (referencemonth.ReferenceMonth, error)
	Delete(context.Context, int64) error
}

type categoryRequest struct {
	Name string `json:"name"`
}

type referenceMonthRequest struct {
	Name string `json:"name"`
}

type expenseRequest struct {
	Name  string `json:"name"`
	Valor string `json:"valor"`
}

type incomeRequest struct {
	ReferenceMonthID int64  `json:"referenceMonthId"`
	Name             string `json:"name"`
	Amount           string `json:"amount"`
}

type expenseCategoryRequest struct {
	CategoryID int64  `json:"categoryId"`
	Source     string `json:"source"`
}

func newHandler(
	db databasePinger,
	importer csvImporter,
	categories categoryCreator,
	expenses expenseCreator,
	incomes incomeCreator,
	dashboardSummary dashboardSummaryReader,
	referenceMonths referenceMonthManager,
) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.GET("/", serveFrontendPage("index.html"))
	router.GET("/settings", serveFrontendPage("settings.html"))
	router.StaticFS("/assets", http.FS(frontend.Files))
	router.GET("/health", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			c.String(http.StatusServiceUnavailable, "database unavailable\n")
			return
		}
		c.String(http.StatusOK, "ok\n")
	})
	router.GET("/api/reference-months", func(c *gin.Context) {
		months, err := referenceMonths.List(c.Request.Context())
		if err != nil {
			log.Printf("list reference months: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list reference months"})
			return
		}
		c.JSON(http.StatusOK, months)
	})
	router.GET("/api/dashboard/summary", func(c *gin.Context) {
		referenceMonthID, err := strconv.ParseInt(c.Query("referenceMonthId"), 10, 64)
		if err != nil || referenceMonthID < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "a valid reference month must be selected"})
			return
		}
		summary, err := dashboardSummary.GetSummary(c.Request.Context(), referenceMonthID)
		if errors.Is(err, dashboard.ErrInvalidReferenceMonth) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, referencemonth.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			log.Printf("load dashboard summary: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load financial summary"})
			return
		}
		c.JSON(http.StatusOK, summary)
	})
	router.POST("/api/reference-months", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
		var request referenceMonthRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "request body must contain a valid reference month description"})
			return
		}

		created, err := referenceMonths.Create(c.Request.Context(), request.Name)
		if errors.Is(err, referencemonth.ErrInvalidName) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, referencemonth.ErrAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			log.Printf("create reference month: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create reference month"})
			return
		}
		c.JSON(http.StatusCreated, created)
	})
	router.PATCH("/api/reference-months/:id", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid reference month ID"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
		var request referenceMonthRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "request body must contain a valid reference month description"})
			return
		}

		updated, err := referenceMonths.Update(c.Request.Context(), id, request.Name)
		if errors.Is(err, referencemonth.ErrInvalidName) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, referencemonth.ErrAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, referencemonth.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			log.Printf("update reference month: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update reference month"})
			return
		}
		c.JSON(http.StatusOK, updated)
	})
	router.DELETE("/api/reference-months/:id", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid reference month ID"})
			return
		}
		if err := referenceMonths.Delete(c.Request.Context(), id); errors.Is(err, referencemonth.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		} else if err != nil {
			log.Printf("delete reference month: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete reference month"})
			return
		}
		c.Status(http.StatusNoContent)
	})
	router.POST("/nubank/import", func(c *gin.Context) {
		const maxUploadSize = 10 << 20
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize)

		fileHeader, err := c.FormFile("file")
		if err != nil {
			var maxBytesError *http.MaxBytesError
			if errors.As(err, &maxBytesError) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "CSV file exceeds the 10 MiB limit"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "multipart field 'file' is required"})
			return
		}
		if !strings.EqualFold(filepath.Ext(fileHeader.Filename), ".csv") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "uploaded file must have a .csv extension"})
			return
		}
		referenceMonthID, err := strconv.ParseInt(c.PostForm("referenceMonthId"), 10, 64)
		if err != nil || referenceMonthID < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "a valid reference month must be selected"})
			return
		}

		file, err := fileHeader.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "could not read uploaded CSV"})
			return
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "could not read uploaded CSV"})
			return
		}
		imported, err := importer.ImportCSV(c.Request.Context(), data, referenceMonthID)
		if errors.Is(err, nubank.ErrInvalidCSV) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, nubank.ErrInvalidReferenceMonth) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, nubank.ErrReferenceMonthNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			log.Printf("import Nubank CSV: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not import CSV"})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"imported": imported})
	})
	router.POST("/api/categories", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
		var request categoryRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "request body must contain a valid JSON category name"})
			return
		}

		created, err := categories.Create(c.Request.Context(), request.Name)
		if errors.Is(err, category.ErrInvalidName) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, category.ErrAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			log.Printf("create category: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create category"})
			return
		}
		c.JSON(http.StatusCreated, created)
	})
	router.GET("/api/categories", func(c *gin.Context) {
		items, err := categories.List(c.Request.Context())
		if err != nil {
			log.Printf("list categories: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list categories"})
			return
		}
		c.JSON(http.StatusOK, items)
	})
	router.POST("/api/expenses", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
		var request expenseRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "request body must contain a valid expense name and amount"})
			return
		}

		created, err := expenses.Create(c.Request.Context(), request.Name, request.Valor)
		if errors.Is(err, expense.ErrInvalidName) || errors.Is(err, expense.ErrInvalidAmount) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			log.Printf("create expense: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create expense"})
			return
		}
		c.JSON(http.StatusCreated, created)
	})
	router.POST("/api/incomes", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
		var request incomeRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "request body must contain a reference month, name, and amount"})
			return
		}
		created, err := incomes.Create(c.Request.Context(), request.ReferenceMonthID, request.Name, request.Amount)
		switch {
		case errors.Is(err, income.ErrInvalidReferenceMonth),
			errors.Is(err, income.ErrInvalidName),
			errors.Is(err, income.ErrInvalidAmount):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, income.ErrReferenceMonthNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case err != nil:
			log.Printf("create income: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create income"})
		default:
			c.JSON(http.StatusCreated, created)
		}
	})
	router.GET("/api/expenses", func(c *gin.Context) {
		items, err := expenses.List(c.Request.Context())
		if err != nil {
			log.Printf("list expenses: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list expenses"})
			return
		}
		c.JSON(http.StatusOK, items)
	})
	router.PATCH("/api/expenses/:id/category", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid expense ID"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
		var request expenseCategoryRequest
		if err := c.ShouldBindJSON(&request); err != nil || request.CategoryID < 1 || (request.Source != expense.SourceManual && request.Source != expense.SourceNubank) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "a valid category ID and expense source are required"})
			return
		}
		updated, err := expenses.AssignCategory(c.Request.Context(), id, request.CategoryID, request.Source)
		if errors.Is(err, expense.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, expense.ErrCategoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			log.Printf("assign category to expense: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not assign category to expense"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"updated": updated})
	})
	return router
}

func serveFrontendPage(name string) gin.HandlerFunc {
	content, err := fs.ReadFile(frontend.Files, name)
	return func(c *gin.Context) {
		if err != nil {
			log.Printf("read embedded frontend page %q: %v", name, err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(http.StatusOK, "text/html; charset=utf-8", content)
	}
}
