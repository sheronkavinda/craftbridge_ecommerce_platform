package account

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"craftbridge/backend/internal/platform/config"
	platformhttp "craftbridge/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func TestAccountRoutesUseAuthenticatedUserForAddressOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newFakeAccountRepository()
	repo.profile = Profile{ID: 9, Name: "Signed-in user", Email: "user@example.test"}
	repo.addresses[5] = Address{
		ID:           5,
		FullName:     "Another user",
		AddressLine1: "2 Oak Street",
		City:         "Austin",
		State:        "TX",
		PostalCode:   "78701",
		CountryCode:  "US",
	}
	repo.addressOwner[5] = 8
	handler := NewHandler(NewService(repo))
	secret := "member01-account-test-secret"
	router := platformhttp.NewRouter(&config.Config{
		AppEnv:    "test",
		JWTSecret: secret,
	}, nil, func(v1 *gin.RouterGroup) {
		RegisterRoutes(v1, handler)
	})

	body := `{"full_name":"Attacker","address_line1":"3 Maple Road","city":"Austin","state":"TX","postal_code":"78702","country_code":"US"}`
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/me/addresses/5", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accountTestJWT(secret, 9))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected another user's address to appear absent, got %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodDelete, "/api/v1/me/addresses/5", nil)
	request.Header.Set("Authorization", "Bearer "+accountTestJWT(secret, 9))
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected another user's address to appear absent, got %d", response.Code)
	}
}

func accountTestJWT(secret string, userID int64) string {
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"sub":  strconv.FormatInt(userID, 10),
		"name": "Signed-in user",
		"role": "buyer",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(claims)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
