package category

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRepository struct {
	category   Category
	categories []Category
	name       string
	err        error
}

func (r *fakeRepository) CreateCategory(_ context.Context, name string) (Category, error) {
	r.name = name
	if r.err != nil {
		return Category{}, r.err
	}
	r.category.Name = name
	return r.category, nil
}

func (r *fakeRepository) ListCategories(context.Context) ([]Category, error) {
	return r.categories, r.err
}

func TestServiceCreatesTrimmedCategoryName(t *testing.T) {
	repository := &fakeRepository{category: Category{ID: 1}}
	created, err := NewService(repository).Create(context.Background(), "  Mercado  ")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repository.name != "Mercado" || created.Name != "Mercado" {
		t.Fatalf("expected trimmed name, got repository=%q result=%q", repository.name, created.Name)
	}
}

func TestServiceListsCategories(t *testing.T) {
	want := []Category{{ID: 1, Name: "Alimentação"}}
	got, err := NewService(&fakeRepository{categories: want}).List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("unexpected categories: %#v", got)
	}
}

func TestServiceRejectsInvalidCategoryNames(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "   "},
		{name: strings.Repeat("a", 101)},
	}

	for _, test := range tests {
		_, err := NewService(&fakeRepository{}).Create(context.Background(), test.name)
		if !errors.Is(err, ErrInvalidName) {
			t.Fatalf("expected ErrInvalidName for name length %d, got %v", len(test.name), err)
		}
	}
}

func TestServicePropagatesRepositoryErrors(t *testing.T) {
	wantErr := errors.New("database unavailable")
	repository := &fakeRepository{err: wantErr}
	_, err := NewService(repository).Create(context.Background(), "Mercado")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected repository error %v, got %v", wantErr, err)
	}
}
