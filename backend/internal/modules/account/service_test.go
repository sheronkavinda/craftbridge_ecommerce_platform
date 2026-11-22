package account

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

type fakeAccountRepository struct {
	profile      Profile
	addresses    map[int64]Address
	addressOwner map[int64]int64
	nextID       int64
	updatedName  string
}

func newFakeAccountRepository() *fakeAccountRepository {
	return &fakeAccountRepository{
		addresses:    make(map[int64]Address),
		addressOwner: make(map[int64]int64),
		nextID:       1,
	}
}

func (r *fakeAccountRepository) GetProfile(_ context.Context, userID int64) (Profile, error) {
	if r.profile.ID != userID {
		return Profile{}, pgx.ErrNoRows
	}
	return r.profile, nil
}

func (r *fakeAccountRepository) UpdateProfile(_ context.Context, userID int64, name string) error {
	if r.profile.ID != userID {
		return pgx.ErrNoRows
	}
	r.updatedName = name
	return nil
}

func (r *fakeAccountRepository) ListAddresses(_ context.Context, userID int64) ([]Address, error) {
	result := make([]Address, 0)
	for id, address := range r.addresses {
		if r.addressOwner[id] == userID {
			result = append(result, address)
		}
	}
	return result, nil
}

func (r *fakeAccountRepository) CreateAddress(_ context.Context, userID int64, address Address) (Address, error) {
	address.ID = r.nextID
	r.nextID++
	r.addresses[address.ID] = address
	r.addressOwner[address.ID] = userID
	return address, nil
}

func (r *fakeAccountRepository) UpdateAddress(_ context.Context, userID, addressID int64, address Address) (Address, error) {
	if r.addressOwner[addressID] != userID {
		return Address{}, pgx.ErrNoRows
	}
	address.ID = addressID
	r.addresses[addressID] = address
	return address, nil
}

func (r *fakeAccountRepository) DeleteAddress(_ context.Context, userID, addressID int64) error {
	if r.addressOwner[addressID] != userID {
		return pgx.ErrNoRows
	}
	delete(r.addresses, addressID)
	delete(r.addressOwner, addressID)
	return nil
}

func TestUpdateProfileTrimsAndValidatesName(t *testing.T) {
	repo := newFakeAccountRepository()
	repo.profile.ID = 7
	service := NewService(repo)

	if err := service.UpdateProfile(context.Background(), 7, UpdateProfileInput{Name: "  Adeesha  "}); err != nil {
		t.Fatalf("UpdateProfile returned error: %v", err)
	}
	if repo.updatedName != "Adeesha" {
		t.Fatalf("expected trimmed name, got %q", repo.updatedName)
	}
	if err := service.UpdateProfile(context.Background(), 7, UpdateProfileInput{Name: " "}); err == nil {
		t.Fatal("expected empty name to be rejected")
	}
	if err := service.UpdateProfile(context.Background(), 7, UpdateProfileInput{Name: string(make([]byte, 121))}); err == nil {
		t.Fatal("expected overlong name to be rejected")
	}
}

func TestCreateAddressNormalizesAndRejectsInvalidInput(t *testing.T) {
	repo := newFakeAccountRepository()
	service := NewService(repo)
	address, err := service.CreateAddress(context.Background(), 7, Address{
		FullName:     " Adeesha Perera ",
		AddressLine1: " 123 Main Street ",
		City:         " Austin ",
		State:        " tx ",
		PostalCode:   " 78701 ",
		CountryCode:  " us ",
	})
	if err != nil {
		t.Fatalf("CreateAddress returned error: %v", err)
	}
	if address.FullName != "Adeesha Perera" || address.AddressLine1 != "123 Main Street" ||
		address.City != "Austin" || address.State != "tx" || address.PostalCode != "78701" ||
		address.CountryCode != "US" {
		t.Fatalf("address was not normalized: %+v", address)
	}

	_, err = service.CreateAddress(context.Background(), 7, Address{CountryCode: "U"})
	if !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("expected ErrInvalidAddress, got %v", err)
	}
}

func TestAddressMutationsAreOwnerScoped(t *testing.T) {
	repo := newFakeAccountRepository()
	repo.addresses[5] = Address{ID: 5, FullName: "Owner"}
	repo.addressOwner[5] = 8
	service := NewService(repo)
	address := Address{
		FullName:     "Another User",
		AddressLine1: "2 Oak Street",
		City:         "Austin",
		State:        "TX",
		PostalCode:   "78701",
		CountryCode:  "US",
	}

	_, err := service.UpdateAddress(context.Background(), 9, 5, address)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected cross-user update to be rejected, got %v", err)
	}
	if err := service.DeleteAddress(context.Background(), 9, 5); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected cross-user delete to be rejected, got %v", err)
	}
}
