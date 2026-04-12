package sdk

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PageRequest holds parsed pagination parameters.
type PageRequest struct {
	// Page is the 1-based page number (default: 1).
	Page int

	// PerPage is the number of items per page (default: 25, max: 100).
	PerPage int

	// offset is the computed offset for SQL queries.
	offset int
}

// Offset returns the computed SQL offset.
func (p PageRequest) Offset() int {
	return p.offset
}

// ParsePageRequest extracts page and per_page from query parameters.
// Applies sensible defaults and caps per_page at 100.
func ParsePageRequest(c *gin.Context) PageRequest {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "25"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}

	return PageRequest{
		Page:    page,
		PerPage: perPage,
		offset:  (page - 1) * perPage,
	}
}

// PageResult holds a page of results with pagination metadata.
type PageResult[T any] struct {
	Items []T        `json:"result"`
	Meta  ResultInfo `json:"result_info"`
}

// Paginate executes a paginated GORM query and returns the results
// with pagination metadata. The query should NOT include Offset/Limit -
// this function applies them.
//
// Usage:
//
//	page := sdk.ParsePageRequest(c)
//	result, err := sdk.Paginate[Order](db.Where("tenant_id = ?", tenantID), page)
//	if err != nil {
//	    sdk.Error(c, err)
//	    return
//	}
//	sdk.OK(c, result)
func Paginate[T any](query *gorm.DB, page PageRequest) (*PageResult[T], error) {
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var items []T
	if err := query.Offset(page.Offset()).Limit(page.PerPage).Find(&items).Error; err != nil {
		return nil, err
	}

	if items == nil {
		items = []T{} // never return null in JSON
	}

	totalPages := int(total) / page.PerPage
	if int(total)%page.PerPage > 0 {
		totalPages++
	}

	return &PageResult[T]{
		Items: items,
		Meta: ResultInfo{
			Page:       page.Page,
			PerPage:    page.PerPage,
			TotalCount: total,
			TotalPages: totalPages,
		},
	}, nil
}
