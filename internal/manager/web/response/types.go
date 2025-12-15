package response

type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	TotalItems int64       `json:"total_items"`
	Page       int         `json:"page,omitempty"`
	PageSize   int         `json:"page_size,omitempty"`
	TotalPages int         `json:"total_pages,omitempty"`
}
