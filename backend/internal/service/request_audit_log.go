package service

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const DefaultRequestAuditBodyLimit = 64 * 1024

type RequestAuditLog struct {
	ID                    int64     `json:"id"`
	RequestID             *string   `json:"request_id,omitempty"`
	UserID                int64     `json:"user_id"`
	APIKeyID              int64     `json:"api_key_id"`
	AccountID             *int64    `json:"account_id,omitempty"`
	GroupID               *int64    `json:"group_id,omitempty"`
	Platform              string    `json:"platform"`
	Endpoint              *string   `json:"endpoint,omitempty"`
	Model                 *string   `json:"model,omitempty"`
	Stream                bool      `json:"stream"`
	StatusCode            *int      `json:"status_code,omitempty"`
	DurationMs            *int      `json:"duration_ms,omitempty"`
	RequestBody           *string   `json:"request_body,omitempty"`
	ResponseBody          *string   `json:"response_body,omitempty"`
	RequestBodyTruncated  bool      `json:"request_body_truncated"`
	ResponseBodyTruncated bool      `json:"response_body_truncated"`
	RequestBodyBytes      int       `json:"request_body_bytes"`
	ResponseBodyBytes     int       `json:"response_body_bytes"`
	IsMocked              bool      `json:"is_mocked"`
	MockRuleID            *int64    `json:"mock_rule_id,omitempty"`
	ErrorMessage          *string   `json:"error_message,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
}

type RequestAuditLogCreateInput struct {
	RequestID      string
	UserID         int64
	APIKeyID       int64
	AccountID      *int64
	GroupID        *int64
	RetentionHours int
	ScopeUserIDs   []int64
	ScopeGroupIDs  []int64
	Platform       string
	Endpoint       string
	Model          string
	Stream         bool
	StatusCode     *int
	DurationMs     *int
	RequestBody    []byte
	ResponseBody   []byte
	IsMocked       bool
	MockRuleID     *int64
	ErrorMessage   string
}

type RequestAuditLogFilter struct {
	UserID    *int64
	APIKeyID  *int64
	AccountID *int64
	GroupID   *int64
	Platform  string
	Model     string
	RequestID string
	Query     string
	IsMocked  *bool
	StartTime *time.Time
	EndTime   *time.Time
}

type RequestAuditLogRepository interface {
	Create(ctx context.Context, log *RequestAuditLog) error
	List(ctx context.Context, params pagination.PaginationParams, filters RequestAuditLogFilter) ([]RequestAuditLog, *pagination.PaginationResult, error)
	GetByID(ctx context.Context, id int64) (*RequestAuditLog, error)
	Cleanup(ctx context.Context, olderThan time.Time) (int64, error)
}

type RequestAuditLogService struct {
	repo     RequestAuditLogRepository
	maxBytes int
}

func NewRequestAuditLogService(repo RequestAuditLogRepository) *RequestAuditLogService {
	return &RequestAuditLogService{repo: repo, maxBytes: DefaultRequestAuditBodyLimit}
}

func (s *RequestAuditLogService) Create(ctx context.Context, input RequestAuditLogCreateInput) error {
	if s == nil || s.repo == nil {
		return nil
	}
	if !ShouldCaptureRequestAudit(input.UserID, input.GroupID, input.ScopeUserIDs, input.ScopeGroupIDs) {
		return nil
	}
	if input.RetentionHours > 0 {
		_, _ = s.repo.Cleanup(ctx, time.Now().Add(-time.Duration(input.RetentionHours)*time.Hour))
	}
	maxBytes := s.maxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultRequestAuditBodyLimit
	}
	reqBody, reqTruncated, reqBytes := truncateAuditBody(input.RequestBody, maxBytes)
	respBody, respTruncated, respBytes := truncateAuditBody(input.ResponseBody, maxBytes)

	log := &RequestAuditLog{
		UserID:                input.UserID,
		APIKeyID:              input.APIKeyID,
		AccountID:             input.AccountID,
		GroupID:               input.GroupID,
		Platform:              strings.TrimSpace(input.Platform),
		Stream:                input.Stream,
		StatusCode:            input.StatusCode,
		DurationMs:            input.DurationMs,
		RequestBody:           reqBody,
		ResponseBody:          respBody,
		RequestBodyTruncated:  reqTruncated,
		ResponseBodyTruncated: respTruncated,
		RequestBodyBytes:      reqBytes,
		ResponseBodyBytes:     respBytes,
		IsMocked:              input.IsMocked,
		MockRuleID:            input.MockRuleID,
	}
	if v := strings.TrimSpace(input.RequestID); v != "" {
		log.RequestID = &v
	}
	if v := strings.TrimSpace(input.Endpoint); v != "" {
		log.Endpoint = &v
	}
	if v := strings.TrimSpace(input.Model); v != "" {
		log.Model = &v
	}
	if v := strings.TrimSpace(input.ErrorMessage); v != "" {
		if len(v) > 1024 {
			v = v[:1024]
		}
		log.ErrorMessage = &v
	}
	return s.repo.Create(ctx, log)
}

func (s *RequestAuditLogService) List(ctx context.Context, params pagination.PaginationParams, filters RequestAuditLogFilter) ([]RequestAuditLog, *pagination.PaginationResult, error) {
	if s == nil || s.repo == nil {
		return []RequestAuditLog{}, &pagination.PaginationResult{Total: 0, Page: 1, PageSize: params.Limit()}, nil
	}
	return s.repo.List(ctx, params, filters)
}

func (s *RequestAuditLogService) GetByID(ctx context.Context, id int64) (*RequestAuditLog, error) {
	if s == nil || s.repo == nil {
		return nil, ErrUsageLogNotFound
	}
	return s.repo.GetByID(ctx, id)
}

func truncateAuditBody(body []byte, maxBytes int) (*string, bool, int) {
	bodyBytes := len(body)
	if bodyBytes == 0 {
		return nil, false, 0
	}
	truncated := false
	if len(body) > maxBytes {
		body = body[:maxBytes]
		truncated = true
	}
	bodyString := string(body)
	return &bodyString, truncated, bodyBytes
}

func ShouldCaptureRequestAudit(userID int64, groupID *int64, scopeUserIDs []int64, scopeGroupIDs []int64) bool {
	hasUsers := len(scopeUserIDs) > 0
	hasGroups := len(scopeGroupIDs) > 0
	if !hasUsers && !hasGroups {
		return true
	}
	userMatch := !hasUsers || requestAuditContainsInt64(scopeUserIDs, userID)
	groupVal := int64(0)
	if groupID != nil {
		groupVal = *groupID
	}
	groupMatch := !hasGroups || requestAuditContainsInt64(scopeGroupIDs, groupVal)
	return userMatch && groupMatch
}

func requestAuditContainsInt64(vals []int64, target int64) bool {
	for _, v := range vals {
		if v == target {
			return true
		}
	}
	return false
}

func (s *RequestAuditLogService) Cleanup(ctx context.Context, olderThan time.Time) (int64, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	return s.repo.Cleanup(ctx, olderThan)
}
