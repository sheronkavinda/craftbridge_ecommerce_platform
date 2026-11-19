package payment

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"craftbridge/backend/internal/platform/config"
)

const (
	defaultPayHereCheckoutURL = "https://sandbox.payhere.lk/pay/checkout"
	defaultPayHereAPIBaseURL  = "https://sandbox.payhere.lk"
	defaultPayHereHTTPTimeout = 10 * time.Second
)

// PayHereSandboxConfig holds the gateway settings required by the PayHere sandbox adapter.
type PayHereSandboxConfig struct {
	MerchantID     string
	MerchantSecret string
	CheckoutURL    string
	ReturnURL      string
	CancelURL      string
	NotifyURL      string
	AppID          string
	AppSecret      string
	APIBaseURL     string
	HTTPClient     HTTPDoer
}

// PayHereSandboxProvider implements PaymentProvider for the PayHere sandbox checkout and retrieval flows.
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
		AppID:          appCfg.PayHereAppID,
		AppSecret:      appCfg.PayHereAppSecret,
		APIBaseURL:     appCfg.PayHereAPIBaseURL,
	})
}

// NewPayHereSandboxProviderFromConfig constructs a PayHere sandbox adapter from explicit PayHereSandboxConfig.
func NewPayHereSandboxProviderFromConfig(cfg PayHereSandboxConfig) *PayHereSandboxProvider {
	checkoutURL := strings.TrimSpace(cfg.CheckoutURL)
	if checkoutURL == "" {
		checkoutURL = defaultPayHereCheckoutURL
	}
	apiBaseURL := strings.TrimRight(strings.TrimSpace(cfg.APIBaseURL), "/")
	if apiBaseURL == "" {
		apiBaseURL = defaultPayHereAPIBaseURL
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultPayHereHTTPTimeout}
	}

	return &PayHereSandboxProvider{
		cfg: PayHereSandboxConfig{
			MerchantID:     strings.TrimSpace(cfg.MerchantID),
			MerchantSecret: strings.TrimSpace(cfg.MerchantSecret),
			CheckoutURL:    checkoutURL,
			ReturnURL:      strings.TrimSpace(cfg.ReturnURL),
			CancelURL:      strings.TrimSpace(cfg.CancelURL),
			NotifyURL:      strings.TrimSpace(cfg.NotifyURL),
			AppID:          strings.TrimSpace(cfg.AppID),
			AppSecret:      strings.TrimSpace(cfg.AppSecret),
			APIBaseURL:     apiBaseURL,
			HTTPClient:     httpClient,
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

	amountFormatted := FormatCentsDecimal(req.AmountCents)
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

// QueryStatus queries PayHere's Merchant Retrieval API for the status of a payment attempt.
// Note: Secrets and access tokens are never included in returned errors or logs.
func (p *PayHereSandboxProvider) QueryStatus(ctx context.Context, providerRef string) (ProviderStatus, error) {
	if p == nil || p.cfg.AppID == "" || p.cfg.AppSecret == "" {
		return ProviderStatus{}, ErrProviderNotConfigured
	}

	trimmedRef := strings.TrimSpace(providerRef)
	if trimmedRef == "" {
		return ProviderStatus{}, fmt.Errorf("%w: attempt reference is required", ErrInvalidPaymentInput)
	}

	client := p.cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultPayHereHTTPTimeout}
	}

	baseURL := strings.TrimRight(p.cfg.APIBaseURL, "/")
	if baseURL == "" {
		baseURL = defaultPayHereAPIBaseURL
	}

	accessToken, err := p.fetchOAuthToken(ctx, client, baseURL)
	if err != nil {
		return ProviderStatus{}, err
	}

	searchURL := fmt.Sprintf("%s/merchant/v1/payment/search?order_id=%s", baseURL, url.QueryEscape(trimmedRef))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return ProviderStatus{}, fmt.Errorf("failed to build payhere search request")
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return ProviderStatus{}, fmt.Errorf("payhere payment search request failed")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return ProviderStatus{}, fmt.Errorf("failed to read payhere search response")
	}

	if resp.StatusCode == http.StatusNotFound {
		return ProviderStatus{
			ProviderReference: trimmedRef,
			Status:            PaymentStatusPending,
		}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ProviderStatus{}, fmt.Errorf("payhere search returned HTTP %d", resp.StatusCode)
	}

	return parsePayHereSearchResponse(trimmedRef, bodyBytes)
}

// RefundPayment issues a refund against PayHere's Merchant API (POST {base}/merchant/v1/payment/refund).
// Secrets and access tokens are never included in returned errors, logs, or the sanitized Raw string.
func (p *PayHereSandboxProvider) RefundPayment(ctx context.Context, input RefundRequestInput) (RefundResult, error) {
	if p == nil || p.cfg.AppID == "" || p.cfg.AppSecret == "" {
		return RefundResult{}, ErrProviderNotConfigured
	}

	paymentID := strings.TrimSpace(input.ProviderReference)
	if paymentID == "" {
		return RefundResult{}, fmt.Errorf("%w: provider payment reference is required", ErrInvalidPaymentInput)
	}

	client := p.cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultPayHereHTTPTimeout}
	}

	baseURL := strings.TrimRight(p.cfg.APIBaseURL, "/")
	if baseURL == "" {
		baseURL = defaultPayHereAPIBaseURL
	}

	accessToken, err := p.fetchOAuthToken(ctx, client, baseURL)
	if err != nil {
		return RefundResult{}, err
	}

	description := firstNonEmpty(input.Reason, "CraftBridge order refund")
	reqBody, _ := json.Marshal(map[string]string{
		"payment_id":  paymentID,
		"description": description,
	})

	refundURL := baseURL + "/merchant/v1/payment/refund"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refundURL, bytes.NewReader(reqBody))
	if err != nil {
		return RefundResult{}, fmt.Errorf("failed to build payhere refund request")
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if trimmedKey := strings.TrimSpace(input.IdempotencyKey); trimmedKey != "" {
		req.Header.Set("Idempotency-Key", trimmedKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return RefundResult{}, fmt.Errorf("payhere refund request failed")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return RefundResult{}, fmt.Errorf("failed to read payhere refund response")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return RefundResult{}, fmt.Errorf("payhere refund returned HTTP %d", resp.StatusCode)
	}

	return parsePayHereRefundResponse(paymentID, strings.TrimSpace(input.IdempotencyKey), bodyBytes)
}

func parsePayHereRefundResponse(paymentID, idempotencyKey string, bodyBytes []byte) (RefundResult, error) {
	var rawMap map[string]any
	if err := json.Unmarshal(bodyBytes, &rawMap); err != nil {
		return RefundResult{}, fmt.Errorf("invalid payhere refund response JSON")
	}

	var dataMap map[string]any
	if d, ok := rawMap["data"].(map[string]any); ok {
		dataMap = d
	}

	refundRef := extractAnyString(dataMap, "refund_id", "payment_id", "id")
	if refundRef == "" {
		refundRef = extractAnyString(rawMap, "refund_id", "payment_id", "id")
	}
	if refundRef == "" {
		if idempotencyKey != "" {
			refundRef = "ph-refund-" + idempotencyKey
		} else {
			refundRef = "ph-refund-" + paymentID
		}
	}

	outcome := RefundOutcomeCompleted
	statusStr := strings.ToUpper(extractAnyString(dataMap, "status"))
	if statusStr == "" {
		statusStr = strings.ToUpper(extractAnyString(rawMap, "status"))
	}

	switch statusStr {
	case "PENDING", "PROCESSING", "0":
		outcome = RefundOutcomePending
	case "FAILED", "ERROR", "-1", "-2":
		outcome = RefundOutcomeFailed
	case "1", "COMPLETED", "SUCCESS", "REFUNDED", "":
		outcome = RefundOutcomeCompleted
	}

	msg := extractAnyString(rawMap, "msg", "message")
	sanitizedRaw, _ := json.Marshal(map[string]string{
		"status":           string(outcome),
		"refund_reference": refundRef,
		"message":          msg,
	})

	return RefundResult{
		ProviderRefundReference: refundRef,
		Status:                  outcome,
		Raw:                     string(sanitizedRaw),
	}, nil
}

func (p *PayHereSandboxProvider) fetchOAuthToken(ctx context.Context, client HTTPDoer, baseURL string) (string, error) {
	tokenURL := baseURL + "/merchant/v1/oauth/token"
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		tokenURL,
		strings.NewReader("grant_type=client_credentials"),
	)
	if err != nil {
		return "", fmt.Errorf("failed to build payhere oauth request")
	}

	basicAuth := base64.StdEncoding.EncodeToString([]byte(p.cfg.AppID + ":" + p.cfg.AppSecret))
	req.Header.Set("Authorization", "Basic "+basicAuth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("payhere oauth request failed")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	if err != nil {
		return "", fmt.Errorf("failed to read payhere oauth response")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("payhere oauth returned HTTP %d", resp.StatusCode)
	}

	var tokenPayload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(bodyBytes, &tokenPayload); err != nil {
		return "", fmt.Errorf("invalid payhere oauth response JSON")
	}

	token := strings.TrimSpace(tokenPayload.AccessToken)
	if token == "" {
		return "", fmt.Errorf("payhere oauth response missing access_token")
	}
	return token, nil
}

func parsePayHereSearchResponse(fallbackRef string, bodyBytes []byte) (ProviderStatus, error) {
	var envelope struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &envelope); err != nil {
		return ProviderStatus{}, fmt.Errorf("invalid payhere search response JSON")
	}

	if len(envelope.Data) == 0 {
		return ProviderStatus{
			ProviderReference: fallbackRef,
			Status:            PaymentStatusPending,
		}, nil
	}

	// Select the first RECEIVED record if present, otherwise the first record in data.
	selected := envelope.Data[0]
	for _, item := range envelope.Data {
		if strings.EqualFold(extractAnyString(item, "status"), "RECEIVED") {
			selected = item
			break
		}
	}

	rawStatus := strings.ToUpper(extractAnyString(selected, "status"))
	var mappedStatus PaymentStatus
	var failureReason string

	switch rawStatus {
	case "RECEIVED", "SUCCESS":
		mappedStatus = PaymentStatusSuccess
	case "CANCELED", "CANCELLED":
		mappedStatus = PaymentStatusCancelled
		failureReason = "cancelled"
	case "FAILED":
		mappedStatus = PaymentStatusFailed
		failureReason = "payment_failed"
	case "PENDING", "":
		mappedStatus = PaymentStatusPending
	default:
		mappedStatus = PaymentStatusPending
	}

	paymentID := extractAnyString(selected, "payment_id", "id")
	if paymentID == "" {
		paymentID = fallbackRef
	}

	currency := strings.ToUpper(extractAnyString(selected, "currency", "payhere_currency"))
	amountCents, _ := extractAnyAmountCents(selected, "amount", "payhere_amount")

	return ProviderStatus{
		ProviderReference: paymentID,
		Status:            mappedStatus,
		AmountCents:       amountCents,
		Currency:          currency,
		FailureReason:     failureReason,
	}, nil
}

func extractAnyString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if raw, ok := m[k]; ok && raw != nil {
			switch v := raw.(type) {
			case string:
				if s := strings.TrimSpace(v); s != "" {
					return s
				}
			case float64:
				return strconv.FormatInt(int64(v), 10)
			case int64:
				return strconv.FormatInt(v, 10)
			}
		}
	}
	return ""
}

func extractAnyAmountCents(m map[string]any, keys ...string) (int64, error) {
	for _, k := range keys {
		if raw, ok := m[k]; ok && raw != nil {
			switch v := raw.(type) {
			case string:
				return ParseDecimalToCents(v)
			case float64:
				return ParseDecimalToCents(fmt.Sprintf("%.2f", v))
			case int64:
				return v * 100, nil
			}
		}
	}
	return 0, nil
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

// FormatCentsDecimal formats integer cents as a 2-decimal string (e.g. 200000 -> "2000.00") without floating-point math.
func FormatCentsDecimal(cents int64) string {
	if cents < 0 {
		abs := -cents
		return fmt.Sprintf("-%d.%02d", abs/100, abs%100)
	}
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
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
