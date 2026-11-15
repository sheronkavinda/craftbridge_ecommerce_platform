package payment

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
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

// VerifyNotification is implemented in a subsequent step.
func (p *PayHereSandboxProvider) VerifyNotification(_ NotificationInput) (VerifiedEvent, error) {
	return VerifiedEvent{}, ErrNotImplemented
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
