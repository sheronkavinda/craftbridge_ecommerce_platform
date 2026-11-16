package payment

import (
	"context"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"craftbridge/backend/internal/platform/config"
)

const defaultPayHereCheckoutURL = "https://sandbox.payhere.lk/pay/checkout"

// PayHereSandboxConfig holds the gateway settings required by the PayHere sandbox adapter.
type PayHereSandboxConfig struct {
	MerchantID     string
	MerchantSecret string
	CheckoutURL    string
	ReturnURL      string
	CancelURL      string
	NotifyURL      string
}

// PayHereSandboxProvider implements PaymentProvider for the PayHere sandbox checkout flow.
type PayHereSandboxProvider struct {
	cfg PayHereSandboxConfig
}

// NewPayHereSandboxProvider constructs a PayHere sandbox adapter from platform configuration.
func NewPayHereSandboxProvider(appCfg *config.Config) *PayHereSandboxProvider {
	if appCfg == nil {
		return &PayHereSandboxProvider{}
	}
	return NewPayHereSandboxProviderFromConfig(PayHereSandboxConfig{
		MerchantID:     appCfg.PayHereMerchantID,
		MerchantSecret: appCfg.PayHereMerchantSecret,
		CheckoutURL:    appCfg.PayHereCheckoutURL,
		ReturnURL:      appCfg.PaymentReturnURL,
		CancelURL:      appCfg.PaymentCancelURL,
		NotifyURL:      appCfg.PaymentNotifyURL,
	})
}

// NewPayHereSandboxProviderFromConfig constructs a PayHere sandbox adapter from explicit PayHereSandboxConfig.
func NewPayHereSandboxProviderFromConfig(cfg PayHereSandboxConfig) *PayHereSandboxProvider {
	checkoutURL := strings.TrimSpace(cfg.CheckoutURL)
	if checkoutURL == "" {
		checkoutURL = defaultPayHereCheckoutURL
	}
	return &PayHereSandboxProvider{
		cfg: PayHereSandboxConfig{
			MerchantID:     strings.TrimSpace(cfg.MerchantID),
			MerchantSecret: strings.TrimSpace(cfg.MerchantSecret),
			CheckoutURL:    checkoutURL,
			ReturnURL:      strings.TrimSpace(cfg.ReturnURL),
			CancelURL:      strings.TrimSpace(cfg.CancelURL),
			NotifyURL:      strings.TrimSpace(cfg.NotifyURL),
		},
	}
}

// Name returns the canonical provider identifier stored in payment_attempts.
func (p *PayHereSandboxProvider) Name() string {
	return "payhere_sandbox"
}

// InitiatePayment validates gateway configuration and builds the signed PayHere checkout form fields.
// The merchant secret is used only to compute the MD5 signature and is never included in the output.
func (p *PayHereSandboxProvider) InitiatePayment(_ context.Context, req InitiateRequest) (InitiateResult, error) {
	if p == nil ||
		p.cfg.MerchantID == "" ||
		p.cfg.MerchantSecret == "" ||
		p.cfg.ReturnURL == "" ||
		p.cfg.CancelURL == "" ||
		p.cfg.NotifyURL == "" {
		return InitiateResult{}, ErrProviderNotConfigured
	}

	providerRef := strings.TrimSpace(req.AttemptID)
	if providerRef == "" {
		return InitiateResult{}, fmt.Errorf("%w: attempt id is required", ErrInvalidPaymentInput)
	}

	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "USD"
	}

	amountFormatted := fmt.Sprintf("%.2f", float64(req.AmountCents)/100.0)
	hash := ComputePayHereHash(p.cfg.MerchantID, providerRef, amountFormatted, currency, p.cfg.MerchantSecret)

	items := strings.TrimSpace(req.Items)
	if items == "" {
		if strings.TrimSpace(req.OrderNumber) != "" {
			items = "Order " + strings.TrimSpace(req.OrderNumber)
		} else {
			items = "Order " + strings.TrimSpace(req.OrderID)
		}
	}

	firstName := firstNonEmpty(req.FirstName, "CraftBridge")
	lastName := firstNonEmpty(req.LastName, "Buyer")
	email := firstNonEmpty(req.Email, "buyer@craftbridge.local")
	phone := firstNonEmpty(req.Phone, "0000000000")
	address := firstNonEmpty(req.Address, "N/A")
	city := firstNonEmpty(req.City, "Colombo")
	country := firstNonEmpty(req.Country, "Sri Lanka")

	fields := map[string]string{
		"merchant_id": p.cfg.MerchantID,
		"return_url":  p.cfg.ReturnURL,
		"cancel_url":  p.cfg.CancelURL,
		"notify_url":  p.cfg.NotifyURL,
		"order_id":    providerRef,
		"items":       items,
		"currency":    currency,
		"amount":      amountFormatted,
		"first_name":  firstName,
		"last_name":   lastName,
		"email":       email,
		"phone":       phone,
		"address":     address,
		"city":        city,
		"country":     country,
		"hash":        hash,
	}

	return InitiateResult{
		ProviderReference: providerRef,
		CheckoutURL:       p.cfg.CheckoutURL,
		Method:            "POST",
		Fields:            fields,
	}, nil
}

// VerifyNotification validates a PayHere webhook notification signature, merchant ID, amount, and status code.
func (p *PayHereSandboxProvider) VerifyNotification(input NotificationInput) (VerifiedEvent, error) {
	if p == nil || p.cfg.MerchantID == "" || p.cfg.MerchantSecret == "" {
		return VerifiedEvent{}, ErrProviderNotConfigured
	}
	if len(input.Form) == 0 {
		return VerifiedEvent{}, ErrMalformedNotification
	}

	merchantID := strings.TrimSpace(input.Form["merchant_id"])
	orderID := strings.TrimSpace(input.Form["order_id"])
	paymentID := strings.TrimSpace(input.Form["payment_id"])
	payhereAmount := strings.TrimSpace(input.Form["payhere_amount"])
	payhereCurrency := strings.TrimSpace(input.Form["payhere_currency"])
	statusCode := strings.TrimSpace(input.Form["status_code"])
	md5sig := strings.TrimSpace(input.Form["md5sig"])
	statusMessage := strings.TrimSpace(input.Form["status_message"])
	method := strings.TrimSpace(input.Form["method"])

	if merchantID == "" || orderID == "" || payhereAmount == "" || payhereCurrency == "" || statusCode == "" || md5sig == "" {
		return VerifiedEvent{}, ErrMalformedNotification
	}

	if subtle.ConstantTimeCompare([]byte(merchantID), []byte(p.cfg.MerchantID)) != 1 {
		return VerifiedEvent{}, ErrInvalidSignature
	}

	expectedSig := ComputePayHereNotifySig(merchantID, orderID, payhereAmount, payhereCurrency, statusCode, p.cfg.MerchantSecret)
	providedSigUpper := strings.ToUpper(md5sig)
	if subtle.ConstantTimeCompare([]byte(providedSigUpper), []byte(expectedSig)) != 1 {
		return VerifiedEvent{}, ErrInvalidSignature
	}

	amountCents, err := ParseDecimalToCents(payhereAmount)
	if err != nil {
		return VerifiedEvent{}, fmt.Errorf("%w: invalid payhere_amount", ErrMalformedNotification)
	}

	var (
		mappedStatus PaymentStatus
		eventType    string
	)
	switch statusCode {
	case "2":
		mappedStatus = PaymentStatusSuccess
		eventType = "PAYMENT_SUCCESS"
	case "0":
		mappedStatus = PaymentStatusPending
		eventType = "PAYMENT_PENDING"
	case "-1":
		mappedStatus = PaymentStatusCancelled
		eventType = "PAYMENT_CANCELLED"
	case "-2":
		mappedStatus = PaymentStatusFailed
		eventType = "PAYMENT_FAILED"
	case "-3":
		// Chargeback: record the event without automatically altering the attempt status.
		mappedStatus = ""
		eventType = "CHARGEBACK"
	default:
		return VerifiedEvent{}, fmt.Errorf("%w: unsupported status_code", ErrMalformedNotification)
	}

	dedupeID := orderID + ":" + statusCode
	if paymentID != "" {
		dedupeID = paymentID + ":" + statusCode
	}

	// Store only non-sensitive metadata in the JSONB payload (never store md5sig or secrets).
	sanitizedPayload, _ := json.Marshal(map[string]string{
		"merchant_id":      merchantID,
		"order_id":         orderID,
		"payment_id":       paymentID,
		"payhere_amount":   payhereAmount,
		"payhere_currency": strings.ToUpper(payhereCurrency),
		"status_code":      statusCode,
		"status_message":   statusMessage,
		"method":           method,
	})

	return VerifiedEvent{
		Provider:          p.Name(),
		ProviderEventID:   dedupeID,
		AttemptID:         orderID,
		ProviderReference: paymentID,
		EventType:         eventType,
		Status:            mappedStatus,
		AmountCents:       amountCents,
		Currency:          strings.ToUpper(payhereCurrency),
		SignatureValid:    true,
		FailureReason:     statusMessage,
		RawPayload:        sanitizedPayload,
	}, nil
}

// QueryStatus is implemented in a subsequent step.
func (p *PayHereSandboxProvider) QueryStatus(_ context.Context, _ string) (ProviderStatus, error) {
	return ProviderStatus{}, ErrNotImplemented
}

// ComputePayHereHash computes the PayHere checkout signature:
// strings.ToUpper(md5hex(merchant_id + order_id + amount + currency + strings.ToUpper(md5hex(merchant_secret))))
func ComputePayHereHash(merchantID, orderID, amountFormatted, currency, merchantSecret string) string {
	hashedSecret := strings.ToUpper(md5Hex(merchantSecret))
	payload := merchantID + orderID + amountFormatted + currency + hashedSecret
	return strings.ToUpper(md5Hex(payload))
}

// ComputePayHereNotifySig computes the PayHere webhook notification signature:
// strings.ToUpper(md5hex(merchant_id + order_id + payhere_amount + payhere_currency + status_code + strings.ToUpper(md5hex(merchant_secret))))
func ComputePayHereNotifySig(merchantID, orderID, payhereAmount, payhereCurrency, statusCode, merchantSecret string) string {
	hashedSecret := strings.ToUpper(md5Hex(merchantSecret))
	payload := merchantID + orderID + payhereAmount + payhereCurrency + statusCode + hashedSecret
	return strings.ToUpper(md5Hex(payload))
}

// ParseDecimalToCents parses a positive or zero decimal amount string (with up to 2 decimal places,
// e.g. "2000.00", "0.10", "12.5", "45") into integer cents without floating-point rounding.
func ParseDecimalToCents(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("empty decimal amount")
	}

	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid decimal amount %q", raw)
	}

	wholeStr := parts[0]
	if wholeStr == "" || !isAllASCIIDigits(wholeStr) {
		return 0, fmt.Errorf("invalid whole part in decimal amount %q", raw)
	}

	wholeVal, err := strconv.ParseInt(wholeStr, 10, 64)
	if err != nil || wholeVal < 0 || wholeVal > (math.MaxInt64-99)/100 {
		return 0, fmt.Errorf("decimal amount out of range %q", raw)
	}

	var fracVal int64
	if len(parts) == 2 {
		fracStr := parts[1]
		if len(fracStr) == 0 || len(fracStr) > 2 || !isAllASCIIDigits(fracStr) {
			return 0, fmt.Errorf("invalid fractional part in decimal amount %q", raw)
		}
		if len(fracStr) == 1 {
			fracStr += "0"
		}
		fracVal, err = strconv.ParseInt(fracStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid fractional part in decimal amount %q", raw)
		}
	}

	return wholeVal*100 + fracVal, nil
}

func isAllASCIIDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

func md5Hex(input string) string {
	sum := md5.Sum([]byte(input))
	return hex.EncodeToString(sum[:])
}

func firstNonEmpty(val, fallback string) string {
	if trimmed := strings.TrimSpace(val); trimmed != "" {
		return trimmed
	}
	return fallback
}
