package nubank

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalidCSV = errors.New("invalid Nubank CSV")
var ErrReferenceMonthNotFound = errors.New("reference month not found")
var ErrInvalidReferenceMonth = errors.New("a valid reference month must be selected")

var brazilianAmountPattern = regexp.MustCompile(`^-?(?:[0-9]+|[0-9]{1,3}(?:\.[0-9]{3})+),[0-9]{2}$`)

type Transaction struct {
	Date   time.Time
	Title  string
	Amount string
}

type Repository interface {
	Import(context.Context, int64, []Transaction) error
}

type Importer struct {
	repository Repository
}

func NewImporter(repository Repository) *Importer {
	return &Importer{repository: repository}
}

func (i *Importer) ImportCSV(ctx context.Context, data []byte, referenceMonthID int64) (int, error) {
	if referenceMonthID < 1 {
		return 0, ErrInvalidReferenceMonth
	}

	transactions, err := ParseCSV(data)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidCSV, err)
	}

	if err := i.repository.Import(ctx, referenceMonthID, transactions); err != nil {
		return 0, err
	}
	return len(transactions), nil
}

func ParseCSV(data []byte) ([]Transaction, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("CSV must be encoded as UTF-8")
	}

	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\ufeff")
	}
	if len(header) != 3 || header[0] != "date" || header[1] != "title" || header[2] != "amount" {
		return nil, errors.New("CSV header must be exactly: date,title,amount")
	}

	var transactions []Transaction
	for row := 2; ; row++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read CSV row %d: %w", row, err)
		}
		if len(record) != 3 {
			return nil, fmt.Errorf("CSV row %d must contain exactly 3 columns", row)
		}

		date, err := time.Parse("2006-01-02", strings.TrimSpace(record[0]))
		if err != nil {
			return nil, fmt.Errorf("CSV row %d has an invalid date; expected YYYY-MM-DD", row)
		}
		title := strings.TrimSpace(record[1])
		if title == "" || strings.IndexByte(title, 0) >= 0 {
			return nil, fmt.Errorf("CSV row %d has an empty or invalid title", row)
		}
		amount, err := parseBrazilianAmount(record[2])
		if err != nil {
			return nil, fmt.Errorf("CSV row %d has an invalid amount; expected Brazilian currency format such as 123,45", row)
		}

		transactions = append(transactions, Transaction{
			Date:   date,
			Title:  title,
			Amount: amount,
		})
	}

	if len(transactions) == 0 {
		return nil, errors.New("CSV contains no transactions")
	}
	return transactions, nil
}

func parseBrazilianAmount(value string) (string, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), " ", "")
	if !brazilianAmountPattern.MatchString(value) {
		return "", errors.New("invalid Brazilian currency amount")
	}
	normalized := strings.ReplaceAll(strings.ReplaceAll(value, ".", ""), ",", ".")
	if _, err := strconv.ParseFloat(normalized, 64); err != nil {
		return "", err
	}
	return normalized, nil
}
