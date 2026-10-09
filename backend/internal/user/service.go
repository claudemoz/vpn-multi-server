package user

import (
	"context"

	apperrors "backend/pkg/errors"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// AccountCleanup removes devices and VPN sessions that still point at a user.
type AccountCleanup interface {
	DeleteAllForUser(ctx context.Context, userID uuid.UUID) error
}

type Service struct {
	repo    *Repository
	cleanup AccountCleanup
}

func NewService(repo *Repository, cleanup AccountCleanup) *Service {
	return &Service{repo: repo, cleanup: cleanup}
}

func (s *Service) GetAll() ([]UserResponse, error) {
	users, err := s.repo.FindAll()
	if err != nil {
		return nil, err
	}
	return ToResponseList(users), nil
}

func (s *Service) GetByID(id uuid.UUID) (UserResponse, error) {
	user, err := s.repo.FindByID(id)
	if err != nil {
		return UserResponse{}, err
	}
	return ToResponse(*user), nil
}

func (s *Service) GetByEmail(email string) (*User, error) {
	return s.repo.FindByEmail(email)
}

func (s *Service) Update(id uuid.UUID, req UpdateUserRequest) (UserResponse, error) {
	user, err := s.repo.FindByID(id)
	if err != nil {
		return UserResponse{}, err
	}

	if req.FirstName != nil {
		user.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		user.LastName = *req.LastName
	}
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.Phone != nil {
		user.Phone = *req.Phone
	}

	if err := s.repo.Update(user); err != nil {
		return UserResponse{}, err
	}
	return ToResponse(*user), nil
}

func (s *Service) ChangePassword(id uuid.UUID, req ChangePasswordRequest) error {
	user, err := s.repo.FindByID(id)
	if err != nil {
		return err
	}
	if CheckPassword(user.Password, req.CurrentPassword) != nil {
		return apperrors.NewUnauthorized("mot de passe actuel incorrect")
	}
	hashed, err := hashPassword(req.Password)
	if err != nil {
		return err
	}
	user.Password = hashed
	return s.repo.Update(user)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if s.cleanup != nil {
		if err := s.cleanup.DeleteAllForUser(ctx, id); err != nil {
			return err
		}
	}
	return s.repo.Delete(id)
}

func (s *Service) DeleteWithPassword(ctx context.Context, id uuid.UUID, password string) error {
	user, err := s.repo.FindByID(id)
	if err != nil {
		return err
	}
	if CheckPassword(user.Password, password) != nil {
		return apperrors.NewUnauthorized("mot de passe incorrect")
	}
	return s.Delete(ctx, id)
}

func (s *Service) Create(req CreateUserRequest) (UserResponse, error) {
	hashed, err := hashPassword(req.Password)
	if err != nil {
		return UserResponse{}, err
	}

	user := &User{
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Email:     req.Email,
		Phone:     req.Phone,
		Password:  hashed,
	}

	if err := s.repo.Create(user); err != nil {
		return UserResponse{}, err
	}
	return ToResponse(*user), nil
}

func hashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", apperrors.NewInvalidInput("failed to hash password")
	}
	return string(hashed), nil
}

func CheckPassword(hashed, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain))
}
