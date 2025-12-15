package request

import (
	"errors"
	"github.com/doujins-org/doujins-billing/internal/manager/api/user"
	"github.com/doujins-org/doujins-billing/internal/manager/web/middleware"
	"mime/multipart"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/app"
	"github.com/doujins-org/doujins-billing/pkg/message"
)

type Request struct {
	State   *app.Runtime
	GinCtx  *gin.Context
	Request *http.Request
}

func NewRequest(ctx *gin.Context, runtime *app.Runtime) *Request {
	return &Request{
		GinCtx:  ctx,
		State:   runtime,
		Request: ctx.Request,
	}
}

func (r *Request) AbortJSON(code int, message string) {
	logrus.Error(message)
	r.GinCtx.AbortWithStatusJSON(code, message)
}

func (r *Request) ErrorJSON(code int, msg string) {
	logrus.Error(msg)
	r.GinCtx.JSON(code, message.Message(msg))
}

func (r *Request) SuccessJSON(data any) {
	r.GinCtx.JSON(http.StatusOK, data)
}

func (r *Request) SuccessJSONMessage(msg string) {
	r.GinCtx.JSON(http.StatusOK, message.Json{
		"message": msg,
	})
}

func (r *Request) Bind(data any) error {
	return r.GinCtx.Bind(data)
}

func (r *Request) BindJSON(data any) bool {
	if err := r.GinCtx.ShouldBindJSON(data); err != nil {
		r.ErrorJSON(http.StatusBadRequest, normaliseBindError(err))
		return false
	}
	return true
}

func (r *Request) BindQuery(data any) bool {
	if err := r.GinCtx.ShouldBindQuery(data); err != nil {
		r.ErrorJSON(http.StatusBadRequest, normaliseBindError(err))
		return false
	}
	return true
}

func (r *Request) BindURI(data any) bool {
	if err := r.GinCtx.ShouldBindUri(data); err != nil {
		r.ErrorJSON(http.StatusBadRequest, normaliseBindError(err))
		return false
	}
	return true
}

func (r *Request) Param(key string) string {
	return r.GinCtx.Param(key)
}

func (r *Request) Query(key string) string {
	return r.GinCtx.Query(key)
}

func (r *Request) Inner() *gin.Context {
	return r.GinCtx
}

func (r *Request) Next() {
	r.GinCtx.Next()
}

func (r *Request) Get(key string) (any, bool) {
	return r.GinCtx.Get(key)
}

func (r *Request) MustGet(key string) any {
	return r.GinCtx.MustGet(key)
}

func (r *Request) Set(key string, value any) {
	r.GinCtx.Set(key, value)
}

func (r *Request) GetUser() *user.UserIdentity {
	if userCtx := middleware.GetUserContext(r.GinCtx); userCtx != nil && userCtx.User != nil {
		return userCtx.User
	}

	userObj, ok := r.Get("user")
	if !ok {
		return nil
	}

	if ui, ok := userObj.(*user.UserIdentity); ok {
		return ui
	}

	if ctx, ok := userObj.(*middleware.UserContext); ok {
		return ctx.User
	}

	return nil
}

func (r *Request) GetClientIP() string {
	if forwarded := r.GinCtx.GetHeader("X-Forwarded-For"); forwarded != "" {
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	if realIP := r.GinCtx.GetHeader("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}

	ip, _, err := net.SplitHostPort(r.GinCtx.Request.RemoteAddr)
	if err != nil {
		return r.GinCtx.Request.RemoteAddr
	}

	return ip
}

func (r *Request) Redirect(code int, location string) {
	r.GinCtx.Redirect(code, location)
}

func (r *Request) FormValue(key string) string {
	return r.GinCtx.PostForm(key)
}

func (r *Request) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	return r.GinCtx.Request.FormFile(key)
}

func (r *Request) GetState() *app.Runtime {
	return r.State
}

func normaliseBindError(err error) string {
	var verr validator.ValidationErrors
	if errors.As(err, &verr) {
		// return first field error message for brevity
		if len(verr) > 0 {
			e := verr[0]
			return strings.ToLower(e.Field()) + " is invalid"
		}
	}
	if strings.Contains(err.Error(), "EOF") {
		return "empty_request_body"
	}
	return "invalid_request"
}
