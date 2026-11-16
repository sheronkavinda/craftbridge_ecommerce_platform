
package seller

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrApplicationAlreadyExists = errors.New("seller application already exists")
	ErrApplicationNotFound = errors.New("seller application not found")
	ErrApplicationNotPending = errors.New("application is not pending")
	ErrShopNotFound = errors.New("shop not found or unavailable")
)

var countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// ApplyForSeller validates and creates a seller application.
func (s *Service) ApplyForSeller(
	ctx context.Context,
	userID int64,
	req CreateSellerApplicationRequest,
) (*SellerApplication, error) {

	if userID <= 0 {
		return nil, ErrInvalidInput
	}

	req.StoreName = strings.TrimSpace(req.StoreName)
	req.CountryCode = strings.ToUpper(
		strings.TrimSpace(req.CountryCode),
	)
	req.Description = strings.TrimSpace(req.Description)

	nameLength := utf8.RuneCountInString(req.StoreName)

	if nameLength < 3 || nameLength > 120 {
		return nil, ErrInvalidInput
	}

	if !countryCodePattern.MatchString(req.CountryCode) {
		return nil, ErrInvalidInput
	}

	if utf8.RuneCountInString(req.Description) > 2000 {
		return nil, ErrInvalidInput
	}

	application, err := s.repo.CreateApplication(
		ctx,
		userID,
		req,
	)

	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrApplicationAlreadyExists
		}

		return nil, err
	}

	return application, nil
}

// GetMyApplication retrieves the logged-in user's application.
func (s *Service) GetMyApplication(
	ctx context.Context,
	userID int64,
) (*SellerApplication, error) {

	if userID <= 0 {
		return nil, ErrInvalidInput
	}

	application, err := s.repo.GetApplicationByUserID(
		ctx,
		userID,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrApplicationNotFound
	}

	return application, err
}

// GetMyShop retrieves the logged-in seller's shop.
func (s *Service) GetMyShop(
	ctx context.Context,
	userID int64,
) (*Shop, error) {

	if userID <= 0 {
		return nil, ErrInvalidInput
	}

	shop, err := s.repo.GetShopByUserID(
		ctx,
		userID,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrShopNotFound
	}

	return shop, err
}

// UpdateMyShop validates and updates shop information.
func (s *Service) UpdateMyShop(
	ctx context.Context,
	userID int64,
	req UpdateShopRequest,
) (*Shop, error) {

	if userID <= 0 {
		return nil, ErrInvalidInput
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.ContactEmail = strings.TrimSpace(req.ContactEmail)

	nameLength := utf8.RuneCountInString(req.Name)

	if nameLength < 3 || nameLength > 120 {
		return nil, ErrInvalidInput
	}

	if utf8.RuneCountInString(req.Description) > 2000 {
		return nil, ErrInvalidInput
	}

	if req.ContactEmail != "" {
		address, err := mail.ParseAddress(req.ContactEmail)

		if err != nil || address.Address != req.ContactEmail {
			return nil, ErrInvalidInput
		}

		if len(req.ContactEmail) > 255 {
			return nil, ErrInvalidInput
		}
	}

	shop, err := s.repo.UpdateShop(
		ctx,
		userID,
		req,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrShopNotFound
	}

	return shop, err
}

// ApproveSellerApplication is called by an authorized admin.
// The route must be protected by admin middleware.
func (s *Service) ApproveSellerApplication(
	ctx context.Context,
	applicationID int64,
	adminUserID int64,
) (*Shop, error) {

	if applicationID <= 0 || adminUserID <= 0 {
		return nil, ErrInvalidInput
	}

	shop, err := s.repo.ApproveApplication(
		ctx,
		applicationID,
		adminUserID,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrApplicationNotFound
	}

	return shop, err
}

// RejectSellerApplication records an admin rejection.
func (s *Service) RejectSellerApplication(
	ctx context.Context,
	applicationID int64,
	adminUserID int64,
	reason string,
) (*SellerApplication, error) {

	reason = strings.TrimSpace(reason)

	if applicationID <= 0 || adminUserID <= 0 {
		return nil, ErrInvalidInput
	}

	if utf8.RuneCountInString(reason) < 3 ||
		utf8.RuneCountInString(reason) > 1000 {
		return nil, ErrInvalidInput
	}

	application, err := s.repo.RejectApplication(
		ctx,
		applicationID,
		adminUserID,
		reason,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		_, lookupErr := s.repo.GetApplicationByID(
			ctx,
			applicationID,
		)

		if errors.Is(lookupErr, pgx.ErrNoRows) {
			return nil, ErrApplicationNotFound
		}

		if lookupErr != nil {
			return nil, lookupErr
		}

		return nil, ErrApplicationNotPending
	}

	return application, err
}
