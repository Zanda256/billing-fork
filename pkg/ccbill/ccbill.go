package ccbill

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"github.com/doujins-org/doujins-billing/config"
)

// GenerateFlexFormURLParams contains parameters for generating CCBill FlexForm URLs for subscription payments
type GenerateFlexFormURLParams struct {
	Username      string `json:"username"`
	Email         string `json:"email"`
	Password      string `json:"password"`
	CustomerFName string `json:"customer_fname"`
	CustomerLName string `json:"customer_lname"`
	Address1      string `json:"address1"`
	City          string `json:"city"`
	State         string `json:"state"`
	ZipCode       string `json:"zipcode"`
	Country       string `json:"country"`

	FlexID  string `json:"flex_id"`
	UserID  string `json:"user_id"`
	PriceID string `json:"price_id"`
}

// FlexFormResponse contains the data needed to embed a CCBill FlexForm in an iFrame
type FlexFormResponse struct {
	IFrameURL  string `json:"iframe_url"`
	Width      string `json:"width"`
	Height     string `json:"height"`
	SuccessURL string `json:"success_url"`
	DeclineURL string `json:"decline_url"`
}

type CCBillClient struct {
	config *config.CCBillConfig
	ApiURL string
}

const (
	CCBillDevAPIURL  = "https://sandbox-api.ccbill.com/wap-frontflex/flexforms/75383d6a-41d4-4bd0-ac12-6c8c37fde5e5"
	CCBillProdAPIURL = "https://api.ccbill.com/wap-frontflex/flexforms/75383d6a-41d4-4bd0-ac12-6c8c37fde5e5"
)

func NewClient(cfg *config.CCBillConfig, isProd bool) *CCBillClient {
	apiURL := CCBillDevAPIURL
	if isProd {
		apiURL = CCBillProdAPIURL
	}

	return &CCBillClient{
		config: cfg,
		ApiURL: apiURL,
	}
}

// GenerateFlexFormURL creates a CCBill FlexForm URL with subscription parameters for iFrame embedding
func (c *CCBillClient) GenerateFlexFormURL(params *GenerateFlexFormURLParams) (*FlexFormResponse, error) {
	if params.Username == "" || params.Email == "" {
		return nil, fmt.Errorf("username and email are required")
	}

	// Build FlexForm URL parameters
	q := url.Values{
		"clientAccnum": {c.config.ClientAccNum},
		"clientSubacc": {c.config.ClientSubAcc},
		"formName":     {c.config.FormName},
		"language":     {c.config.Language},
		"currencyCode": {c.config.CurrencyCode},

		// Customer information
		"email":          {params.Email},
		"username":       {params.Username},
		"password":       {params.Password},
		"customer_fname": {params.CustomerFName},
		"customer_lname": {params.CustomerLName},
		"address1":       {params.Address1},
		"city":           {params.City},
		"state":          {params.State},
		"zipcode":        {params.ZipCode},
		"country":        {params.Country},
	}

	// Generate signature if salt is configured
	if c.config.Salt != "" {
		sigInput := url.Values{"username": {params.Username}}
		q.Set("signature", c.generateCCBillSignature(sigInput))
	}

	baseURL := c.config.BaseFlexFormURL
	flexFormURL := fmt.Sprintf("%s/%s?%s", strings.TrimRight(baseURL, "/"), params.FlexID, q.Encode())

	return &FlexFormResponse{
		IFrameURL:  flexFormURL,
		Width:      c.getConfigOrDefault(c.config.IFrameWidth, "100%"),
		Height:     c.getConfigOrDefault(c.config.IFrameHeight, "600px"),
		SuccessURL: c.config.SuccessURL,
		DeclineURL: c.config.DeclineURL,
	}, nil
}

func (c *CCBillClient) generateCCBillSignature(query url.Values) string {
	hash := sha256.Sum256([]byte(c.createSignatureInput(query)))
	return hex.EncodeToString(hash[:])
}

func (c *CCBillClient) VerifyCallbackSignature(params url.Values) bool {
	signature := params.Get("X-signature")

	hash := sha256.Sum256([]byte(c.createSignatureInput(params)))
	return hex.EncodeToString(hash[:]) == strings.ToLower(signature)
}

func (c *CCBillClient) createSignatureInput(params url.Values) string {
	return params.Get("username") + c.config.Salt
}

func (c *CCBillClient) Config() *config.CCBillConfig {
	return c.config
}

func (c *CCBillClient) getConfigOrDefault(configValue, defaultValue string) string {
	if configValue == "" {
		return defaultValue
	}

	return configValue
}
