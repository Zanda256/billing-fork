package nmi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/config"
)

const (
	DefaultDirectPostURL = "https://secure.networkmerchants.com/api/transact.php"
	DefaultQueryAPIURL   = "https://secure.nmi.com/api/query.php"
)

type NMIClient struct {
	providerName  string
	config        *config.NMIProviderSettings
	SecurityKey   string
	WebhookSecret string
	DirectPostURL string
	QueryURL      string
	IsProd        bool
}

type CreateCustomerVaultData struct {
	// Prefer using PaymentToken from Collect.js. If provided, cc fields are ignored.
	PaymentToken string
	FirstName    string
	LastName     string
	Address1     string
	City         string
	State        string
	Zip          string
	Country      string
	Phone        string
	Email        string
	Company      string
	Address2     string
}

type UpdateCustomerVaultData struct {
	CustomerVaultID string
	CreateCustomerVaultData
}

type DeleteCustomerVaultData struct {
	CustomerVaultID string
}

type CardUserData struct {
	FirstName string
	LastName  string
	Address1  string
	City      string
	State     string
	Zip       string
	Country   string
}

type RecurringPaymentData struct {
	CardUserData
	PlanID          string
	CustomerVaultID string
	Email           string
	Currency        string
	PaymentToken    string
	Amount          float64
	OrderID         string
	PONumber        string
	CustomerID      string
}

type QueryFilter struct {
	StartDate   string
	EndDate     string
	Condition   string
	ActionType  string
	PageNumber  int
	ResultLimit int
}

type CreateCustomerVaultResponse struct {
	CustomerVaultID string
}

type AddSubscriptionResponse struct {
	Type           string
	SubscriptionID string
	TransactionID  string
	Authcode       string
}

type ManualRebillParams struct {
	VaultID        string
	BillingID      string
	SubscriptionID string
}

type ManualRebillResponse struct {
	Success       bool
	TransactionID string
	ErrorMessage  string
}

func resolveWebhookSecret(provider string, cfg *config.NMIProviderSettings) string {
	upperProvider := strings.ToUpper(provider)
	secrets := []string{
		strings.TrimSpace(os.Getenv(fmt.Sprintf("NMI_%s_WEBHOOK_SECRET", upperProvider))),
	}
	if strings.EqualFold(provider, "mobius") {
		secrets = append(secrets, strings.TrimSpace(os.Getenv("MOBIUS_WEBHOOK_SECRET")))
	}
	secrets = append(secrets,
		strings.TrimSpace(os.Getenv("NMI_WEBHOOK_SECRET")),
	)
	for _, candidate := range secrets {
		if candidate != "" {
			return candidate
		}
	}
	if cfg != nil {
		return strings.TrimSpace(cfg.WebhookSecret)
	}
	return ""
}

func resolveEndpoint(defaultURL string, cfgValue string, envKeys ...string) string {
	for _, key := range envKeys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	if value := strings.TrimSpace(cfgValue); value != "" {
		return value
	}
	return defaultURL
}

func NewClient(provider string, cfg *config.NMIProviderSettings, isProd bool) (*NMIClient, error) {
	if cfg == nil {
		return nil, errors.New("nmi provider configuration is required")
	}

	webhookSecret := resolveWebhookSecret(provider, cfg)
	if webhookSecret == "" {
		log.WithField("provider", provider).Warn("NMI webhook secret not configured - webhook signature verification will be disabled")
	}

	securityKey := strings.TrimSpace(firstNonEmpty(
		os.Getenv(fmt.Sprintf("NMI_%s_SECURITY_KEY", strings.ToUpper(provider))),
		cfg.SecurityKey,
	))
	if strings.EqualFold(provider, "mobius") && securityKey == "" {
		securityKey = strings.TrimSpace(os.Getenv("MOBIUS_SECURITY_KEY"))
	}
	if securityKey == "" && cfg.SecurityKey != "" {
		securityKey = strings.TrimSpace(cfg.SecurityKey)
	}

	if isProd && securityKey == "" {
		return nil, fmt.Errorf("nmi provider '%s' security key is required in production mode", provider)
	}

	if !isProd && securityKey == "" {
		log.WithField("provider", provider).Warn("NMI security_key not configured - NMI API calls will be disabled")
	}

	directPostURL := resolveEndpoint(
		firstNonEmpty(cfg.DirectPostURL, DefaultDirectPostURL),
		cfg.DirectPostURL,
		fmt.Sprintf("NMI_%s_DIRECT_POST_URL", strings.ToUpper(provider)),
	)
	if strings.EqualFold(provider, "mobius") {
		directPostURL = resolveEndpoint(directPostURL, cfg.DirectPostURL, "MOBIUS_DIRECT_POST_URL")
	}

	queryURL := resolveEndpoint(
		firstNonEmpty(cfg.QueryURL, DefaultQueryAPIURL),
		cfg.QueryURL,
		fmt.Sprintf("NMI_%s_QUERY_URL", strings.ToUpper(provider)),
	)
	if strings.EqualFold(provider, "mobius") {
		queryURL = resolveEndpoint(queryURL, cfg.QueryURL, "MOBIUS_QUERY_URL")
	}

	return &NMIClient{
		providerName:  provider,
		config:        cfg,
		SecurityKey:   securityKey,
		WebhookSecret: webhookSecret,
		DirectPostURL: directPostURL,
		QueryURL:      queryURL,
		IsProd:        isProd,
	}, nil
}

// Config returns the NMI configuration
func (c *NMIClient) Config() *config.NMIProviderSettings {
	return c.config
}

// isConfigured returns true if the client has valid configuration
func (c *NMIClient) isConfigured() bool {
	return c.SecurityKey != ""
}

// checkConfiguration returns an error if the client is not configured properly
func (c *NMIClient) checkConfiguration() error {
	if !c.isConfigured() {
		return fmt.Errorf("nmi provider '%s' payment processing is not configured - this feature is disabled in development mode", c.providerName)
	}
	return nil
}

func (c *NMIClient) CreateCustomerVault(data CreateCustomerVaultData) (*CreateCustomerVaultResponse, error) {
	if err := c.checkConfiguration(); err != nil {
		return nil, err
	}

	values := url.Values{
		"customer_vault": {"add_customer"},
		"security_key":   {c.SecurityKey},
	}

	values.Set("payment_token", strings.TrimSpace(data.PaymentToken))

	if data.FirstName != "" {
		values.Set("first_name", data.FirstName)
	}

	if data.LastName != "" {
		values.Set("last_name", data.LastName)
	}

	if data.Address1 != "" {
		values.Set("address1", data.Address1)
	}

	if data.City != "" {
		values.Set("city", data.City)
	}

	if data.State != "" {
		values.Set("state", data.State)
	}

	if data.Zip != "" {
		values.Set("zip", data.Zip)
	}

	if data.Country != "" {
		values.Set("country", data.Country)
	}

	if data.Phone != "" {
		values.Set("phone", data.Phone)
	}

	if data.Email != "" {
		values.Set("email", data.Email)
	}

	if !c.IsProd {
		values.Set("test_mode", "enabled")
	}

	response, err := c.sendDirectRequest(values)
	if err != nil {
		return nil, err
	}

	if !strings.Contains(response, "response=1") {
		return nil, fmt.Errorf("failed to create customer vault: %s", response)
	}

	output, err := url.ParseQuery(response)
	if err != nil {
		return nil, err
	}

	if output.Get("response") != "1" {
		return nil, fmt.Errorf("failed to create customer vault: %s", output.Get("response_message"))
	}

	vaultID := output.Get("customer_vault_id")
	if vaultID == "" {
		return nil, fmt.Errorf("failed to create customer vault: %s", output.Get("response_message"))
	}

	return &CreateCustomerVaultResponse{CustomerVaultID: vaultID}, nil
}

func (c *NMIClient) UpdateCustomerVault(data UpdateCustomerVaultData) error {
	if err := c.checkConfiguration(); err != nil {
		return err
	}

	if data.CustomerVaultID == "" {
		return errors.New("customer vault ID is required")
	}

	values := url.Values{
		"customer_vault":    {"update_customer"},
		"security_key":      {c.SecurityKey},
		"customer_vault_id": {data.CustomerVaultID},
	}

	if strings.TrimSpace(data.PaymentToken) != "" {
		values.Set("payment_token", strings.TrimSpace(data.PaymentToken))
	}

	if data.FirstName != "" {
		values.Set("first_name", data.FirstName)
	}
	if data.LastName != "" {
		values.Set("last_name", data.LastName)
	}
	if data.Address1 != "" {
		values.Set("address1", data.Address1)
	}
	if data.City != "" {
		values.Set("city", data.City)
	}
	if data.State != "" {
		values.Set("state", data.State)
	}
	if data.Zip != "" {
		values.Set("zip", data.Zip)
	}
	if data.Country != "" {
		values.Set("country", data.Country)
	}
	if data.Phone != "" {
		values.Set("phone", data.Phone)
	}
	if data.Email != "" {
		values.Set("email", data.Email)
	}
	if data.Company != "" {
		values.Set("company", data.Company)
	}
	if data.Address2 != "" {
		values.Set("address2", data.Address2)
	}

	if !c.IsProd {
		values.Set("test_mode", "enabled")
	}

	response, err := c.sendDirectRequest(values)
	if err != nil {
		return err
	}

	if !strings.Contains(response, "response=1") {
		return fmt.Errorf("failed to update customer vault: %s", response)
	}

	return nil
}

func (c *NMIClient) DeleteCustomerVault(data DeleteCustomerVaultData) error {
	if err := c.checkConfiguration(); err != nil {
		return err
	}

	if data.CustomerVaultID == "" {
		return errors.New("customer vault ID is required")
	}

	values := url.Values{
		"customer_vault":    {"delete_customer"},
		"security_key":      {c.SecurityKey},
		"customer_vault_id": {data.CustomerVaultID},
	}

	if !c.IsProd {
		values.Set("test_mode", "enabled")
	}

	response, err := c.sendDirectRequest(values)
	if err != nil {
		return err
	}

	if !strings.Contains(response, "response=1") {
		return fmt.Errorf("failed to delete customer vault: %s", response)
	}

	return nil
}

func (c *NMIClient) AddRecurringSubscription(data RecurringPaymentData) (*AddSubscriptionResponse, error) {
	if err := c.checkConfiguration(); err != nil {
		return nil, err
	}

	if data.PlanID == "" {
		return nil, errors.New("PlanID is required")
	}

	if data.CustomerVaultID == "" && data.PaymentToken == "" {
		return nil, errors.New("either customer vault or payment token is required")
	}

	amtStr := strconv.FormatFloat(data.Amount, 'f', 2, 64)

	values := url.Values{
		"type":              {"sale"},
		"amount":            {amtStr},
		"email":             {data.Email},
		"plan_id":           {data.PlanID},
		"billing_method":    {"recurring"},
		"security_key":      {c.SecurityKey},
		"currency":          {data.Currency},
		"recurring":         {"add_subscription"},
		"order_description": {"Doujins Subscription"},
		"first_name":        {data.FirstName},
		"last_name":         {data.LastName},
		"address1":          {data.Address1},
		"city":              {data.City},
		"state":             {data.State},
		"zip":               {data.Zip},
		"country":           {data.Country},
	}

	if trimmed := strings.TrimSpace(data.OrderID); trimmed != "" {
		values.Set("orderid", trimmed)
	}
	if trimmed := strings.TrimSpace(data.PONumber); trimmed != "" {
		values.Set("ponumber", trimmed)
	}
	if trimmed := strings.TrimSpace(data.CustomerID); trimmed != "" && strings.TrimSpace(data.CustomerVaultID) == "" {
		values.Set("customerid", trimmed)
	}

	if data.PaymentToken != "" {
		values.Set("payment_token", data.PaymentToken)
	}

	if data.CustomerVaultID != "" {
		values.Set("customer_vault_id", data.CustomerVaultID)
	}

	if !c.IsProd {
		values.Set("test_mode", "enabled")
	}

	response, err := c.sendDirectRequest(values)
	if err != nil {
		return nil, err
	}

	output, err := url.ParseQuery(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %s", response)
	}

	if output.Get("response") != "1" {
		return nil, fmt.Errorf("failed to add subscription: %s", output.Get("responsetext"))
	}

	return &AddSubscriptionResponse{
		Type:           output.Get("type"),
		Authcode:       output.Get("authcode"),
		TransactionID:  output.Get("transactionid"),
		SubscriptionID: output.Get("subscription_id"),
	}, nil
}

func (c *NMIClient) UpdateRecurringSubscription(subscriptionID, planAmount string, planPayments int) (string, error) {
	if err := c.checkConfiguration(); err != nil {
		return "", err
	}

	if subscriptionID == "" || planAmount == "" {
		return "", errors.New("missing required fields: subscriptionID, planAmount")
	}

	values := url.Values{
		"recurring":       {"update_subscription"},
		"security_key":    {c.SecurityKey},
		"subscription_id": {subscriptionID},
		"plan_amount":     {planAmount},
		"plan_payments":   {fmt.Sprintf("%d", planPayments)},
	}

	if !c.IsProd {
		values.Set("test_mode", "enabled")
	}

	response, err := c.sendDirectRequest(values)
	if err != nil {
		return "", err
	}

	if !strings.Contains(response, "response=1") {
		return "", fmt.Errorf("failed to update subscription: %s", response)
	}

	return response, nil
}

func (c *NMIClient) DeleteRecurringSubscription(subscriptionID string) error {
	if err := c.checkConfiguration(); err != nil {
		return err
	}

	if subscriptionID == "" {
		return errors.New("subscriptionID is required")
	}

	values := url.Values{
		"recurring":       {"delete_subscription"},
		"security_key":    {c.SecurityKey},
		"subscription_id": {subscriptionID},
	}

	if !c.IsProd {
		values.Set("test_mode", "enabled")
	}

	response, err := c.sendDirectRequest(values)
	if err != nil {
		return err
	}

	if !strings.Contains(response, "response=1") {
		return fmt.Errorf("failed to delete subscription: %s", response)
	}

	return nil
}

func (c *NMIClient) AttemptManualRebill(params ManualRebillParams) (*ManualRebillResponse, error) {
	if err := c.checkConfiguration(); err != nil {
		return &ManualRebillResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, err
	}

	if params.VaultID == "" || params.BillingID == "" || params.SubscriptionID == "" {
		err := errors.New("vault ID, billing ID, and subscription ID are required")
		return &ManualRebillResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, err
	}

	values := url.Values{
		"type":              {"sale"},
		"security_key":      {c.SecurityKey},
		"customer_vault_id": {params.VaultID},
		"billing_id":        {params.BillingID},
		"subscription_id":   {params.SubscriptionID},
		"recurring":         {"rebill_subscription"},
		"order_description": {"Manual Rebill - Doujins Subscription"},
	}

	if !c.IsProd {
		values.Set("test_mode", "enabled")
	}

	response, err := c.sendDirectRequest(values)
	if err != nil {
		return &ManualRebillResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("request failed: %s", err.Error()),
		}, err
	}

	output, err := url.ParseQuery(response)
	if err != nil {
		return &ManualRebillResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to parse response: %s", response),
		}, err
	}

	if output.Get("response") == "1" {
		return &ManualRebillResponse{
			Success:       true,
			TransactionID: output.Get("transactionid"),
		}, nil
	}

	// Rebill failed
	errorMessage := output.Get("responsetext")
	if errorMessage == "" {
		errorMessage = "Unknown error"
	}

	return &ManualRebillResponse{
		Success:      false,
		ErrorMessage: errorMessage,
	}, nil
}

func (c *NMIClient) GetTransactionDetails(transactionID string) (string, error) {
	if err := c.checkConfiguration(); err != nil {
		return "", err
	}

	if transactionID == "" {
		return "", errors.New("transactionID is required")
	}

	values := url.Values{
		"Servicert_type": {"transaction"},
		"security_key":   {c.SecurityKey},
		"transaction_id": {transactionID},
	}

	if !c.IsProd {
		values.Set("test_mode", "enabled")
	}

	response, err := c.sendQueryRequest(values)
	if err != nil {
		return "", err
	}

	return response, nil
}

func (c *NMIClient) GetCustomerVaultData(customerVaultID string) (string, error) {
	if err := c.checkConfiguration(); err != nil {
		return "", err
	}

	values := url.Values{
		"Servicert_type": {"customer_vault"},
		"security_key":   {c.SecurityKey},
	}

	if customerVaultID != "" {
		values.Set("customer_vault_id", customerVaultID)
	}

	if !c.IsProd {
		values.Set("test_mode", "enabled")
	}

	response, err := c.sendQueryRequest(values)
	if err != nil {
		return "", err
	}

	return response, nil
}

func (c *NMIClient) GetSubscriptionData(subscriptionID string) (string, error) {
	if err := c.checkConfiguration(); err != nil {
		return "", err
	}

	values := url.Values{
		"Servicert_type": {"recurring"},
		"security_key":   {c.SecurityKey},
	}

	if subscriptionID != "" {
		values.Set("subscription_id", subscriptionID)
	}

	if !c.IsProd {
		values.Set("test_mode", "enabled")
	}

	response, err := c.sendQueryRequest(values)
	if err != nil {
		return "", err
	}

	return response, nil
}

func (c *NMIClient) GetRecurringPlanData() (string, error) {
	if err := c.checkConfiguration(); err != nil {
		return "", err
	}

	values := url.Values{
		"Servicert_type": {"recurring_plans"},
		"security_key":   {c.SecurityKey},
	}

	if !c.IsProd {
		values.Set("test_mode", "enabled")
	}

	response, err := c.sendQueryRequest(values)
	if err != nil {
		return "", err
	}

	return response, nil
}

func (c *NMIClient) SearchTransactions(filter QueryFilter) (string, error) {
	if err := c.checkConfiguration(); err != nil {
		return "", err
	}

	values := url.Values{
		"Servicert_type": {"transaction"},
		"security_key":   {c.SecurityKey},
	}

	if !c.IsProd {
		values.Set("test_mode", "enabled")
	}

	if filter.StartDate != "" {
		values.Set("start_date", filter.StartDate)
	}
	if filter.EndDate != "" {
		values.Set("end_date", filter.EndDate)
	}
	if filter.Condition != "" {
		values.Set("condition", filter.Condition)
	}
	if filter.ActionType != "" {
		values.Set("action_type", filter.ActionType)
	}
	if filter.PageNumber > 0 {
		values.Set("page_number", fmt.Sprintf("%d", filter.PageNumber))
	}
	if filter.ResultLimit > 0 {
		values.Set("result_limit", fmt.Sprintf("%d", filter.ResultLimit))
	}

	response, err := c.sendQueryRequest(values)
	if err != nil {
		return "", err
	}

	return response, nil
}
func (c *NMIClient) sendDirectRequest(data url.Values) (_ string, err error) {
	fmt.Println("data", data)
	fmt.Println("DirectPostURL", c.DirectPostURL)

	resp, err := http.PostForm(c.DirectPostURL, data)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		cerr := resp.Body.Close()
		if err == nil {
			err = cerr
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return string(body), nil
}

func (c *NMIClient) sendQueryRequest(data url.Values) (_ string, err error) {
	resp, err := http.PostForm(c.QueryURL, data)
	if err != nil {
		return "", fmt.Errorf("failed to send query request: %w", err)
	}
	defer func() {
		cerr := resp.Body.Close()
		if err == nil {
			err = cerr
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read query response: %w", err)
	}

	return string(body), nil
}

func (c *NMIClient) VerifyWebhookSignature(body []byte, signature string) error {
	if c.WebhookSecret == "" {
		return errors.New("webhook secret not configured")
	}

	signature = strings.TrimPrefix(signature, "sha256=")

	mac := hmac.New(sha256.New, []byte(c.WebhookSecret))
	mac.Write(body)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return errors.New("webhook signature verification failed")
	}

	return nil
}

func (c *NMIClient) GetWebhookSecret() string {
	return c.WebhookSecret
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
