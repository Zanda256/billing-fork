package request

import (
	"github.com/doujins-org/doujins-billing/internal/manager/api/subscription"
	"time"
)

type IRequest interface {
	Path() any
	Query() any
	Body() any
	SetDefaults()
}

// BaseRequest provides default nil implementations for Request interface methods.
type BaseRequest struct{}

// Path returns nil by default, indicating no URI parameters.
func (b *BaseRequest) Path() any {
	return nil
}

// Query returns nil by default, indicating no query parameters.
func (b *BaseRequest) Query() any {
	return nil
}

// Body returns nil by default, indicating no body parameters.
func (b *BaseRequest) Body() any {
	return nil
}

func (b *BaseRequest) SetDefaults() {
}

// PaginationParams provides common pagination parameters that can be embedded in request structs
type PaginationParams struct {
	Page     int `form:"page" default:"1" validate:"min=1,max=1000"`
	PageSize int `form:"page_size" default:"20" validate:"min=1,max=500"`
}

// SetPaginationDefaults sets default values for pagination parameters
// This method accepts a custom default page size to allow flexibility across different endpoints
func (p *PaginationParams) SetPaginationDefaults(defaultPageSize int) {
	p.Page = 1
	p.PageSize = defaultPageSize
}

// This method makes PageSize=min(PageSize,maxPageSize) and return true is PageSize have been changed
func (p *PaginationParams) CapPageSize(maxPageSize int) bool {
	if maxPageSize < p.PageSize {
		p.PageSize = maxPageSize
		return true
	}
	return false
}

// -------------------------------- GetSubscription Request --------------------------------

type GetSubscriptionRequest struct {
	BaseRequest
}

// -------------------------------- Subscribe Request --------------------------------

type SubscribeBodyParams struct {
	subscription.SubscribeData
}

type SubscribeRequest struct {
	BaseRequest
	SubscribeBodyParams
}

func (r *SubscribeRequest) Body() any {
	return &r.SubscribeBodyParams
}

// -------------------------------- GetProducts Request --------------------------------

type GetProductsRequest struct {
	BaseRequest
}

// -------------------------------- GetSubscribePageData Request --------------------------------

type GetSubscribePageDataRequest struct {
	BaseRequest
}

// -------------------------------- CancelSubscription Request --------------------------------

type CancelSubscriptionBodyParams struct {
	Feedback string `json:"feedback" validate:"max=500"`
}

type CancelSubscriptionRequest struct {
	BaseRequest
	CancelSubscriptionBodyParams
}

func (r *CancelSubscriptionRequest) Body() any {
	return &r.CancelSubscriptionBodyParams
}

// -------------------------------- GenerateFlexFormURL Request --------------------------------

type GenerateFlexFormURLBodyParams struct {
	PriceID   string `json:"price_id" validate:"required,uuid"`
	FirstName string `json:"first_name" validate:"required,min=1,max=100"`
	LastName  string `json:"last_name" validate:"required,min=1,max=100"`
	Address1  string `json:"address1" validate:"required,min=1,max=200"`
	City      string `json:"city" validate:"required,min=1,max=100"`
	State     string `json:"state" validate:"required,min=1,max=50"`
	ZipCode   string `json:"zip_code" validate:"required,min=1,max=20"`
	Country   string `json:"country" validate:"required,min=2,max=2"` // ISO 2-letter country code
}

type GenerateFlexFormURLRequest struct {
	BaseRequest
	GenerateFlexFormURLBodyParams
}

func (r *GenerateFlexFormURLRequest) Body() any {
	return &r.GenerateFlexFormURLBodyParams
}

// -------------------------------- Solana Generate Payment Request --------------------------------

type GeneratePaymentBodyParams struct {
	PriceID    string `json:"price_id" validate:"required,uuid"`
	Token      string `json:"token" validate:"required"`       // e.g., SOL, USDC
	UserWallet string `json:"user_wallet" validate:"required"` // base58 address
}

type GeneratePaymentRequest struct {
	BaseRequest
	GeneratePaymentBodyParams
}

func (r *GeneratePaymentRequest) Body() any {
	return &r.GeneratePaymentBodyParams
}

// -------------------------------- Solana Submit Payment Request --------------------------------

type SubmitPaymentBodyParams struct {
	SignedTransaction string `json:"signed_transaction" validate:"required"`
	PriceID           string `json:"price_id" validate:"required,uuid"`
	Memo              string `json:"memo"`
	IntentID          string `json:"intent_id" validate:"required,uuid"`
}

type SubmitPaymentRequest struct {
	BaseRequest
	SubmitPaymentBodyParams
}

func (r *SubmitPaymentRequest) Body() any {
	return &r.SubmitPaymentBodyParams
}

// -------------------------------- Solana Check Payment Request --------------------------------

type CheckSolanaPaymentQueryParams struct {
	Reference string `form:"reference" validate:"required"`
	Memo      string `form:"memo"`
}

type CheckSolanaPaymentRequest struct {
	BaseRequest
	CheckSolanaPaymentQueryParams
}

func (r *CheckSolanaPaymentRequest) Query() any {
	return &r.CheckSolanaPaymentQueryParams
}

// -------------------------------- Solana Generate QR Request --------------------------------

type GenerateSolanaPayQRBodyParams struct {
	PriceID    string `json:"price_id" validate:"required,uuid"`
	Token      string `json:"token" validate:"required"`
	UserWallet string `json:"user_wallet" validate:"required"`
}

type GenerateSolanaPayQRRequest struct {
	BaseRequest
	GenerateSolanaPayQRBodyParams
}

func (r *GenerateSolanaPayQRRequest) Body() any {
	return &r.GenerateSolanaPayQRBodyParams
}

// -------------------------------- Payment Methods Requests --------------------------------

type ListPaymentMethodsQueryParams struct {
	PaginationParams
	IncludeInactive bool `form:"include_inactive"`
}

type ListPaymentMethodsRequest struct {
	BaseRequest
	ListPaymentMethodsQueryParams
}

func (r *ListPaymentMethodsRequest) Query() any {
	return &r.ListPaymentMethodsQueryParams
}

func (r *ListPaymentMethodsRequest) SetDefaults() {
	r.SetPaginationDefaults(20)
}

type DeletePaymentMethodPathParams struct {
	ID string `uri:"id" binding:"required,uuid"`
}

type DeletePaymentMethodRequest struct {
	BaseRequest
	DeletePaymentMethodPathParams
}

func (r *DeletePaymentMethodRequest) Path() any {
	return &r.DeletePaymentMethodPathParams
}

type ActivatePaymentMethodPathParams struct {
	ID string `uri:"id" binding:"required,uuid"`
}

type ActivatePaymentMethodRequest struct {
	BaseRequest
	ActivatePaymentMethodPathParams
}

func (r *ActivatePaymentMethodRequest) Path() any {
	return &r.ActivatePaymentMethodPathParams
}

type CreatePaymentMethodRequest struct {
	PaymentToken string `json:"payment_token" binding:"required"`
	FirstName    string `json:"first_name" binding:"required"`
	LastName     string `json:"last_name" binding:"required"`
	Address1     string `json:"address1" binding:"required"`
	City         string `json:"city" binding:"required"`
	State        string `json:"state" binding:"omitempty"`
	Zip          string `json:"zip" binding:"required"`
	Country      string `json:"country" binding:"required"`
	Phone        string `json:"phone" binding:"omitempty"`
	Email        string `json:"email" binding:"omitempty,email"`
	Company      string `json:"company" binding:"omitempty"`
	Address2     string `json:"address2" binding:"omitempty"`
	Provider     string `json:"provider" binding:"omitempty"`
}

type UpdatePaymentMethodPathParams struct {
	ID string `uri:"id" binding:"required,uuid"`
}

type UpdatePaymentMethodBodyParams struct {
	PaymentToken string  `json:"payment_token" binding:"required"`
	FirstName    *string `json:"first_name"`
	LastName     *string `json:"last_name"`
	Address1     *string `json:"address1"`
	City         *string `json:"city"`
	State        *string `json:"state"`
	Zip          *string `json:"zip"`
	Country      *string `json:"country"`
	Phone        *string `json:"phone"`
	Email        *string `json:"email" binding:"omitempty,email"`
	Company      *string `json:"company"`
	Address2     *string `json:"address2"`
	Provider     *string `json:"provider"`
}

type UpdatePaymentMethodRequest struct {
	BaseRequest
	UpdatePaymentMethodPathParams
	UpdatePaymentMethodBodyParams
}

func (r *UpdatePaymentMethodRequest) Path() any {
	return &r.UpdatePaymentMethodPathParams
}

func (r *UpdatePaymentMethodRequest) Body() any {
	return &r.UpdatePaymentMethodBodyParams
}

// -------------------------------- GetBillingHistory Request --------------------------------

type GetBillingHistoryQueryParams struct {
	PaginationParams
	StartDate     *time.Time `form:"start_date" time_format:"2006-01-02"`
	EndDate       *time.Time `form:"end_date" time_format:"2006-01-02"`
	Processor     *string    `form:"processor" validate:"omitempty,oneof=ccbill nmi system"`
	MinAmount     *float64   `form:"min_amount" validate:"omitempty,min=0"`
	MaxAmount     *float64   `form:"max_amount" validate:"omitempty,min=0"`
	IncludeStats  bool       `form:"include_stats" default:"false"`
	IncludeEvents bool       `form:"include_events" default:"true"`
}

type GetBillingHistoryRequest struct {
	BaseRequest
	GetBillingHistoryQueryParams
}

func (r *GetBillingHistoryRequest) Query() any {
	return &r.GetBillingHistoryQueryParams
}

func (r *GetBillingHistoryRequest) SetDefaults() {
	r.SetPaginationDefaults(20)
}

// -------------------------------- GetUserBillingHistory Request (Admin) --------------------------------

type GetUserBillingHistoryPathParams struct {
	UserID string `uri:"user_id" binding:"required"`
}

type GetUserBillingHistoryQueryParams struct {
	PaginationParams
	StartDate     *time.Time `form:"start_date" time_format:"2006-01-02"`
	EndDate       *time.Time `form:"end_date" time_format:"2006-01-02"`
	Processor     *string    `form:"processor" validate:"omitempty,oneof=ccbill nmi system"`
	MinAmount     *float64   `form:"min_amount" validate:"omitempty,min=0"`
	MaxAmount     *float64   `form:"max_amount" validate:"omitempty,min=0"`
	IncludeStats  bool       `form:"include_stats" default:"false"`
	IncludeEvents bool       `form:"include_events" default:"true"`
}

type GetUserBillingHistoryRequest struct {
	BaseRequest
	GetUserBillingHistoryPathParams
	GetUserBillingHistoryQueryParams
}

func (r *GetUserBillingHistoryRequest) Path() any {
	return &r.GetUserBillingHistoryPathParams
}

func (r *GetUserBillingHistoryRequest) Query() any {
	return &r.GetUserBillingHistoryQueryParams
}

func (r *GetUserBillingHistoryRequest) SetDefaults() {
	r.SetPaginationDefaults(20)
}
