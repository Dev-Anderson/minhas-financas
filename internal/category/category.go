package category

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidName   = errors.New("category name must contain between 1 and 100 characters")
	ErrAlreadyExists = errors.New("category already exists")
)

type Category struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Repository interface {
	CreateCategory(context.Context, string) (Category, error)
	ListCategories(context.Context) ([]Category, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Create(ctx context.Context, name string) (Category, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return Category{}, ErrInvalidName
	}
	return s.repository.CreateCategory(ctx, name)
}

func (s *Service) List(ctx context.Context) ([]Category, error) {
	return s.repository.ListCategories(ctx)
}
