package account

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidAddress = errors.New("invalid address")
	ErrInvalidName    = errors.New("name is required and must be 120 characters or fewer")
)

type Service struct {
	repo accountRepository
}

type accountRepository interface {
	GetProfile(context.Context, int64) (Profile, error)
	UpdateProfile(context.Context, int64, string) error
	ListAddresses(context.Context, int64) ([]Address, error)
	CreateAddress(context.Context, int64, Address) (Address, error)
	UpdateAddress(context.Context, int64, int64, Address) (Address, error)
	DeleteAddress(context.Context, int64, int64) error
}

func NewService(repo accountRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetProfile(ctx context.Context, userID int64) (Profile, error) {
	return s.repo.GetProfile(ctx, userID)
}

func (s *Service) UpdateProfile(ctx context.Context, userID int64, input UpdateProfileInput) error {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return ErrInvalidName
	}
	if utf8.RuneCountInString(input.Name) > 120 {
		return ErrInvalidName
	}
	return s.repo.UpdateProfile(ctx, userID, input.Name)
}

func (s *Service) ListAddresses(ctx context.Context, userID int64) ([]Address, error) {
	return s.repo.ListAddresses(ctx, userID)
}

func (s *Service) CreateAddress(ctx context.Context, userID int64, address Address) (Address, error) {
	address, err := validateAddress(address)
	if err != nil {
		return Address{}, err
	}
	return s.repo.CreateAddress(ctx, userID, address)
}

func (s *Service) UpdateAddress(ctx context.Context, userID, addressID int64, address Address) (Address, error) {
	address, err := validateAddress(address)
	if err != nil {
		return Address{}, err
	}
	return s.repo.UpdateAddress(ctx, userID, addressID, address)
}

func (s *Service) DeleteAddress(ctx context.Context, userID, addressID int64) error {
	return s.repo.DeleteAddress(ctx, userID, addressID)
}

func validateAddress(a Address) (Address, error) {
	a.Label = strings.TrimSpace(a.Label)
	a.FullName = strings.TrimSpace(a.FullName)
	a.Phone = strings.TrimSpace(a.Phone)
	a.AddressLine1 = strings.TrimSpace(a.AddressLine1)
	a.AddressLine2 = strings.TrimSpace(a.AddressLine2)
	a.City = strings.TrimSpace(a.City)
	a.State = strings.TrimSpace(a.State)
	a.PostalCode = strings.TrimSpace(a.PostalCode)
	a.CountryCode = strings.ToUpper(strings.TrimSpace(a.CountryCode))
	if a.FullName == "" || a.AddressLine1 == "" || a.City == "" ||
		a.State == "" || a.PostalCode == "" || len(a.CountryCode) != 2 ||
		!validCountryCode(a.CountryCode) ||
		utf8.RuneCountInString(a.FullName) > 120 || utf8.RuneCountInString(a.AddressLine1) > 180 ||
		utf8.RuneCountInString(a.AddressLine2) > 180 || utf8.RuneCountInString(a.City) > 100 ||
		utf8.RuneCountInString(a.State) > 100 || utf8.RuneCountInString(a.PostalCode) > 30 ||
		utf8.RuneCountInString(a.Phone) > 30 || utf8.RuneCountInString(a.Label) > 60 {
		return Address{}, ErrInvalidAddress
	}
	return a, nil
}

func validCountryCode(code string) bool {
	if len(code) != 2 {
		return false
	}
	for _, char := range code {
		if char < 'A' || char > 'Z' {
			return false
		}
	}
	return true
}
