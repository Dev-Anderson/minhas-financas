package referencemonth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRepository struct {
	month ReferenceMonth
	name  string
	err   error
}

func (r *fakeRepository) List(context.Context) ([]ReferenceMonth, error) {
	return []ReferenceMonth{r.month}, r.err
}

func (r *fakeRepository) CreateReferenceMonth(_ context.Context, name string) (ReferenceMonth, error) {
	r.name = name
	if r.err != nil {
		return ReferenceMonth{}, r.err
	}
	r.month.Name = name
	return r.month, nil
}

func (r *fakeRepository) UpdateReferenceMonth(_ context.Context, id int64, name string) (ReferenceMonth, error) {
	r.name = name
	if r.err != nil {
		return ReferenceMonth{}, r.err
	}
	return ReferenceMonth{ID: id, Name: name}, nil
}

func (r *fakeRepository) DeleteReferenceMonth(context.Context, int64) error {
	return r.err
}

func TestServiceTrimsDescription(t *testing.T) {
	repository := &fakeRepository{month: ReferenceMonth{ID: 1}}
	month, err := NewService(repository).Create(context.Background(), " Novembro de 2026 ")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repository.name != "Novembro de 2026" || month.Name != repository.name {
		t.Fatalf("description was not trimmed: %#v", month)
	}
}

func TestServiceRejectsInvalidDescription(t *testing.T) {
	for _, name := range []string{" ", strings.Repeat("a", 101)} {
		_, err := NewService(&fakeRepository{}).Create(context.Background(), name)
		if !errors.Is(err, ErrInvalidName) {
			t.Errorf("expected ErrInvalidName for length %d, got %v", len(name), err)
		}
	}
}
