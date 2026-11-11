package referencemonth

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidName   = errors.New("reference month description must contain between 1 and 100 characters")
	ErrAlreadyExists = errors.New("reference month description already exists")
	ErrNotFound      = errors.New("reference month not found")
)

type ReferenceMonth struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Repository interface {
	List(context.Context) ([]ReferenceMonth, error)
	CreateReferenceMonth(context.Context, string) (ReferenceMonth, error)
	UpdateReferenceMonth(context.Context, int64, string) (ReferenceMonth, error)
	DeleteReferenceMonth(context.Context, int64) error
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context) ([]ReferenceMonth, error) {
	return s.repository.List(ctx)
}

func (s *Service) Create(ctx context.Context, name string) (ReferenceMonth, error) {
	name, err := validateName(name)
	if err != nil {
		return ReferenceMonth{}, err
	}
	return s.repository.CreateReferenceMonth(ctx, name)
}

func (s *Service) Update(ctx context.Context, id int64, name string) (ReferenceMonth, error) {
	if id < 1 {
		return ReferenceMonth{}, ErrNotFound
	}
	name, err := validateName(name)
	if err != nil {
		return ReferenceMonth{}, err
	}
	return s.repository.UpdateReferenceMonth(ctx, id, name)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id < 1 {
		return ErrNotFound
	}
	return s.repository.DeleteReferenceMonth(ctx, id)
}

func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return "", ErrInvalidName
	}
	return name, nil
}
