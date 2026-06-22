package repository

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/requestauditlog"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type requestAuditLogRepository struct {
	client *ent.Client
}

func NewRequestAuditLogRepository(client *ent.Client) service.RequestAuditLogRepository {
	return &requestAuditLogRepository{client: client}
}

func (r *requestAuditLogRepository) Create(ctx context.Context, log *service.RequestAuditLog) error {
	if r == nil || r.client == nil || log == nil {
		return nil
	}
	builder := r.client.RequestAuditLog.Create().
		SetUserID(log.UserID).
		SetAPIKeyID(log.APIKeyID).
		SetPlatform(log.Platform).
		SetStream(log.Stream).
		SetRequestBodyTruncated(log.RequestBodyTruncated).
		SetResponseBodyTruncated(log.ResponseBodyTruncated).
		SetRequestBodyBytes(log.RequestBodyBytes).
		SetResponseBodyBytes(log.ResponseBodyBytes).
		SetIsMocked(log.IsMocked)
	if log.RequestID != nil {
		builder.SetRequestID(*log.RequestID)
	}
	if log.AccountID != nil {
		builder.SetAccountID(*log.AccountID)
	}
	if log.GroupID != nil {
		builder.SetGroupID(*log.GroupID)
	}
	if log.Endpoint != nil {
		builder.SetEndpoint(*log.Endpoint)
	}
	if log.Model != nil {
		builder.SetModel(*log.Model)
	}
	if log.StatusCode != nil {
		builder.SetStatusCode(*log.StatusCode)
	}
	if log.DurationMs != nil {
		builder.SetDurationMs(*log.DurationMs)
	}
	if log.RequestBody != nil {
		builder.SetRequestBody(*log.RequestBody)
	}
	if log.ResponseBody != nil {
		builder.SetResponseBody(*log.ResponseBody)
	}
	if log.MockRuleID != nil {
		builder.SetMockRuleID(*log.MockRuleID)
	}
	if log.ErrorMessage != nil {
		builder.SetErrorMessage(*log.ErrorMessage)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}
	log.ID = created.ID
	log.CreatedAt = created.CreatedAt
	return nil
}

func (r *requestAuditLogRepository) List(ctx context.Context, params pagination.PaginationParams, filters service.RequestAuditLogFilter) ([]service.RequestAuditLog, *pagination.PaginationResult, error) {
	if r == nil || r.client == nil {
		return []service.RequestAuditLog{}, &pagination.PaginationResult{Total: 0, Page: params.Page, PageSize: params.Limit()}, nil
	}
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 20
	}
	query := r.client.RequestAuditLog.Query()
	applyRequestAuditFilters(query, filters)

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)
	sortField := strings.TrimSpace(params.SortBy)
	sortOpt := ent.Desc(requestauditlog.FieldCreatedAt)
	if sortOrder == pagination.SortOrderAsc {
		sortOpt = ent.Asc(requestauditlog.FieldCreatedAt)
	}
	switch sortField {
	case requestauditlog.FieldDurationMs:
		if sortOrder == pagination.SortOrderAsc {
			sortOpt = ent.Asc(requestauditlog.FieldDurationMs)
		} else {
			sortOpt = ent.Desc(requestauditlog.FieldDurationMs)
		}
	case requestauditlog.FieldCreatedAt, "":
		// default above
	default:
		return nil, nil, fmt.Errorf("invalid sort_by: %s", sortField)
	}

	rows, err := query.Order(sortOpt).Offset(params.Offset()).Limit(params.Limit()).All(ctx)
	if err != nil {
		return nil, nil, err
	}
	items := make([]service.RequestAuditLog, 0, len(rows))
	for _, row := range rows {
		items = append(items, requestAuditLogToService(row))
	}
	if err := r.populateUserEmails(ctx, items); err != nil {
		return nil, nil, err
	}
	pageSize := params.Limit()
	pages := 0
	if pageSize > 0 && total > 0 {
		pages = int(math.Ceil(float64(total) / float64(pageSize)))
	}
	return items, &pagination.PaginationResult{Total: int64(total), Page: params.Page, PageSize: pageSize, Pages: pages}, nil
}

func (r *requestAuditLogRepository) GetByID(ctx context.Context, id int64) (*service.RequestAuditLog, error) {
	row, err := r.client.RequestAuditLog.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, service.ErrUsageLogNotFound
		}
		return nil, err
	}
	items := []service.RequestAuditLog{requestAuditLogToService(row)}
	if err := r.populateUserEmails(ctx, items); err != nil {
		return nil, err
	}
	return &items[0], nil
}

func (r *requestAuditLogRepository) populateUserEmails(ctx context.Context, items []service.RequestAuditLog) error {
	if len(items) == 0 {
		return nil
	}
	userIDSet := make(map[int64]struct{}, len(items))
	userIDs := make([]int64, 0, len(items))
	for _, item := range items {
		if item.UserID <= 0 {
			continue
		}
		if _, exists := userIDSet[item.UserID]; exists {
			continue
		}
		userIDSet[item.UserID] = struct{}{}
		userIDs = append(userIDs, item.UserID)
	}
	if len(userIDs) == 0 {
		return nil
	}
	users, err := r.client.User.Query().
		Where(user.IDIn(userIDs...)).
		Select(user.FieldID, user.FieldEmail).
		All(ctx)
	if err != nil {
		return err
	}
	emails := make(map[int64]string, len(users))
	for _, u := range users {
		emails[u.ID] = u.Email
	}
	for i := range items {
		items[i].UserEmail = emails[items[i].UserID]
	}
	return nil
}

func applyRequestAuditFilters(query *ent.RequestAuditLogQuery, filters service.RequestAuditLogFilter) {
	if filters.UserID != nil {
		query.Where(requestauditlog.UserIDEQ(*filters.UserID))
	}
	if filters.APIKeyID != nil {
		query.Where(requestauditlog.APIKeyIDEQ(*filters.APIKeyID))
	}
	if filters.AccountID != nil {
		query.Where(requestauditlog.AccountIDEQ(*filters.AccountID))
	}
	if filters.GroupID != nil {
		query.Where(requestauditlog.GroupIDEQ(*filters.GroupID))
	}
	if v := strings.TrimSpace(filters.Platform); v != "" {
		query.Where(requestauditlog.PlatformEQ(v))
	}
	if v := strings.TrimSpace(filters.Model); v != "" {
		query.Where(requestauditlog.ModelEQ(v))
	}
	if v := strings.TrimSpace(filters.RequestID); v != "" {
		query.Where(requestauditlog.RequestIDContains(v))
	}
	if v := strings.TrimSpace(filters.Query); v != "" {
		query.Where(requestauditlog.Or(
			requestauditlog.RequestBodyContains(v),
			requestauditlog.ResponseBodyContains(v),
			requestauditlog.RequestIDContains(v),
		))
	}
	if filters.IsMocked != nil {
		query.Where(requestauditlog.IsMockedEQ(*filters.IsMocked))
	}
	if filters.StartTime != nil {
		query.Where(requestauditlog.CreatedAtGTE(*filters.StartTime))
	}
	if filters.EndTime != nil {
		query.Where(requestauditlog.CreatedAtLT(*filters.EndTime))
	}
}

func requestAuditLogToService(row *ent.RequestAuditLog) service.RequestAuditLog {
	return service.RequestAuditLog{
		ID:                    row.ID,
		RequestID:             row.RequestID,
		UserID:                row.UserID,
		APIKeyID:              row.APIKeyID,
		AccountID:             row.AccountID,
		GroupID:               row.GroupID,
		Platform:              row.Platform,
		Endpoint:              row.Endpoint,
		Model:                 row.Model,
		Stream:                row.Stream,
		StatusCode:            row.StatusCode,
		DurationMs:            row.DurationMs,
		RequestBody:           row.RequestBody,
		ResponseBody:          row.ResponseBody,
		RequestBodyTruncated:  row.RequestBodyTruncated,
		ResponseBodyTruncated: row.ResponseBodyTruncated,
		RequestBodyBytes:      row.RequestBodyBytes,
		ResponseBodyBytes:     row.ResponseBodyBytes,
		IsMocked:              row.IsMocked,
		MockRuleID:            row.MockRuleID,
		ErrorMessage:          row.ErrorMessage,
		CreatedAt:             row.CreatedAt,
	}
}

func (r *requestAuditLogRepository) Cleanup(ctx context.Context, olderThan time.Time) (int64, error) {
	if r == nil || r.client == nil {
		return 0, nil
	}
	n, err := r.client.RequestAuditLog.Delete().Where(requestauditlog.CreatedAtLT(olderThan)).Exec(ctx)
	return int64(n), err
}
