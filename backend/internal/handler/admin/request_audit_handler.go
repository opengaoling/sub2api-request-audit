package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type RequestAuditHandler struct {
	service *service.RequestAuditLogService
}

func NewRequestAuditHandler(service *service.RequestAuditLogService) *RequestAuditHandler {
	return &RequestAuditHandler{service: service}
}

func (h *RequestAuditHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filters, ok := parseRequestAuditFilters(c)
	if !ok {
		return
	}
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}
	items, result, err := h.service.List(c.Request.Context(), params, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, result.Total, page, pageSize)
}

func (h *RequestAuditHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid id")
		return
	}
	item, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func parseRequestAuditFilters(c *gin.Context) (service.RequestAuditLogFilter, bool) {
	var f service.RequestAuditLogFilter
	parseID := func(name string) (*int64, bool) {
		raw := strings.TrimSpace(c.Query(name))
		if raw == "" {
			return nil, true
		}
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			response.BadRequest(c, "Invalid "+name)
			return nil, false
		}
		return &id, true
	}
	var ok bool
	if f.UserID, ok = parseID("user_id"); !ok {
		return f, false
	}
	if f.APIKeyID, ok = parseID("api_key_id"); !ok {
		return f, false
	}
	if f.AccountID, ok = parseID("account_id"); !ok {
		return f, false
	}
	if f.GroupID, ok = parseID("group_id"); !ok {
		return f, false
	}
	f.Platform = strings.TrimSpace(c.Query("platform"))
	f.Model = strings.TrimSpace(c.Query("model"))
	f.RequestID = strings.TrimSpace(c.Query("request_id"))
	f.Query = strings.TrimSpace(c.Query("q"))
	if raw := strings.TrimSpace(c.Query("is_mocked")); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			response.BadRequest(c, "Invalid is_mocked")
			return f, false
		}
		f.IsMocked = &v
	}
	userTZ := c.Query("timezone")
	if raw := strings.TrimSpace(c.Query("start_date")); raw != "" {
		t, err := timezone.ParseInUserLocation("2006-01-02", raw, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid start_date format, use YYYY-MM-DD")
			return f, false
		}
		f.StartTime = &t
	}
	if raw := strings.TrimSpace(c.Query("end_date")); raw != "" {
		t, err := timezone.ParseInUserLocation("2006-01-02", raw, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid end_date format, use YYYY-MM-DD")
			return f, false
		}
		t = t.AddDate(0, 0, 1)
		f.EndTime = &t
	}
	if raw := strings.TrimSpace(c.Query("start_time")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "Invalid start_time format, use RFC3339")
			return f, false
		}
		f.StartTime = &t
	}
	if raw := strings.TrimSpace(c.Query("end_time")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "Invalid end_time format, use RFC3339")
			return f, false
		}
		f.EndTime = &t
	}
	return f, true
}

func (h *RequestAuditHandler) Cleanup(c *gin.Context) {
	hours, err := strconv.Atoi(strings.TrimSpace(c.Query("older_than_hours")))
	if err != nil || hours <= 0 {
		response.BadRequest(c, "Invalid older_than_hours")
		return
	}
	deleted, err := h.service.Cleanup(c.Request.Context(), time.Now().Add(-time.Duration(hours)*time.Hour))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": deleted})
}
