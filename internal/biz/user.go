package biz

import (
	"context"
	"strings"
	"time"

	v1 "testdemo/api/todo/v1"

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

// UserRepo is a user repo interface.
type UserRepo interface {
	FindByID(context.Context, int64) (*User, error)
	ListUsers(context.Context, ...ListOption) ([]*User, error)
	CreateUser(context.Context, *User) (*User, error)
	UpdateUser(context.Context, *User) (*User, error)
	DeleteUser(context.Context, int64) error
	Login(context.Context, string, string) (*v1.LoginUserReply, error)
}

// UserUsecase is a User usecase.
type UserUsecase struct {
	repo UserRepo
}

// NewUserUsecase new a User usecase.相当于构造函数
func NewUserUsecase(repo UserRepo) *UserUsecase {
	return &UserUsecase{repo: repo}
}

func (uc *UserUsecase) Login(ctx context.Context, username string, password string) (*v1.LoginUserReply, error) {
	return uc.repo.Login(ctx, username, password)
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
	if err := validateUser(user); err != nil {
		return nil, err
	}
	return uc.repo.CreateUser(ctx, user)
}

// UpdateUser updates a user.
func (uc *UserUsecase) UpdateUser(ctx context.Context, user *User) (*User, error) {
	if user == nil || user.ID <= 0 {
		return nil, ErrUserInvalidArgument
	}
	return uc.repo.UpdateUser(ctx, user)
}

// DeleteUser deletes a user.
func (uc *UserUsecase) DeleteUser(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrUserInvalidArgument
	}
	return uc.repo.DeleteUser(ctx, id)
}

func validateUser(user *User) error {
	if user == nil {
		return ErrUserInvalidArgument
	}
	if strings.TrimSpace(user.Name) == "" {
		return ErrUserInvalidArgument
	}
	if strings.TrimSpace(user.Email) == "" {
		return ErrUserInvalidArgument
	}
	return nil
}
