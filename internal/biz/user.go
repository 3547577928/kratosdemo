package biz

import (
	"context"
	"strings"
	"time"

	v1 "testdemo/api/todo/v1"
	"testdemo/pkg/auth"

	"github.com/go-kratos/kratos/v3/errors"
)

var (
	// ErrUserNotFound is returned when a user does not exist.
	ErrUserNotFound = errors.NotFound(v1.ErrorReason_USER_NOT_FOUND.String(), "user not found")
	// ErrUserInvalidArgument is returned when a user request is invalid.
	ErrUserInvalidArgument = errors.BadRequest(v1.ErrorReason_USER_INVALID_ARGUMENT.String(), "invalid user argument")
)

// User is a User domain model (DO).
type User struct {
	ID         int64
	Name       string
	Email      string
	Password   string
	CreateTime time.Time
	UpdateTime time.Time
}

const (
	defaultLoginSecret = "testdemo-jwt-secret-change-me-in-production"
	defaultLoginRole   = "user"
	defaultLoginExpire = 24
)

type LoginResult struct {
	Token string
}

// UserRepo is a user repo interface.
type UserRepo interface {
	FindByID(context.Context, int64) (*User, error)
	FindByAccount(context.Context, string) (*User, error)
	ListUsers(context.Context, ...ListOption) ([]*User, error)
	CreateUser(context.Context, *User) (*User, error)
	UpdateUser(context.Context, *User) (*User, error)
	DeleteUser(context.Context, int64) error
}

// UserUsecase is a User usecase.
type UserUsecase struct {
	repo UserRepo
}

// NewUserUsecase new a User usecase.相当于构造函数
func NewUserUsecase(repo UserRepo) *UserUsecase {
	return &UserUsecase{repo: repo}
}

func (uc *UserUsecase) Login(ctx context.Context, username string, password string) (*LoginResult, error) {
	username = strings.TrimSpace(username)
	if username == "" || strings.TrimSpace(password) == "" {
		return nil, ErrUserInvalidArgument
	}
	user, err := uc.repo.FindByAccount(ctx, username)
	if err != nil {
		return nil, err
	}
	if user.Password != password {
		return nil, ErrUserInvalidArgument
	}
	token, err := auth.GenerateToken(defaultLoginSecret, user.ID, user.Name, defaultLoginRole, defaultLoginExpire)
	if err != nil {
		return nil, err
	}
	return &LoginResult{Token: token}, nil
}

// GetUser returns a user by ID.
func (uc *UserUsecase) GetUser(ctx context.Context, id int64) (*User, error) {
	return uc.repo.FindByID(ctx, id)
}

// ListUsers lists users.
func (uc *UserUsecase) ListUsers(ctx context.Context, opts ...ListOption) ([]*User, error) {
	return uc.repo.ListUsers(ctx, opts...)
}

// CreateUser creates a user.
func (uc *UserUsecase) CreateUser(ctx context.Context, user *User) (*User, error) {
	normalized, err := normalizeNewUser(user)
	if err != nil {
		return nil, err
	}
	return uc.repo.CreateUser(ctx, normalized)
}

// UpdateUser updates a user.
func (uc *UserUsecase) UpdateUser(ctx context.Context, user *User) (*User, error) {
	normalized, err := normalizeUpdateUser(user)
	if err != nil {
		return nil, err
	}
	if !hasUserChanges(normalized) {
		return uc.repo.FindByID(ctx, normalized.ID)
	}
	return uc.repo.UpdateUser(ctx, normalized)
}

// DeleteUser deletes a user.
func (uc *UserUsecase) DeleteUser(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrUserInvalidArgument
	}
	return uc.repo.DeleteUser(ctx, id)
}

func normalizeNewUser(user *User) (*User, error) {
	if user == nil {
		return nil, ErrUserInvalidArgument
	}
	clone := *user
	clone.Name = strings.TrimSpace(clone.Name)
	clone.Email = strings.TrimSpace(clone.Email)
	if clone.Name == "" || clone.Email == "" || strings.TrimSpace(clone.Password) == "" {
		return nil, ErrUserInvalidArgument
	}
	return &clone, nil
}

func normalizeUpdateUser(user *User) (*User, error) {
	if user == nil || user.ID <= 0 {
		return nil, ErrUserInvalidArgument
	}
	clone := *user
	clone.Name = strings.TrimSpace(clone.Name)
	clone.Email = strings.TrimSpace(clone.Email)
	return &clone, nil
}

func hasUserChanges(user *User) bool {
	return user != nil && (user.Name != "" || user.Email != "" || user.Password != "")
}
