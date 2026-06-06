package handler

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const requestAuditCaptureLimit = service.DefaultRequestAuditBodyLimit

type auditCaptureWriter struct {
	gin.ResponseWriter
	buf   []byte
	limit int
}

type requestAuditCaptureDecision struct {
	Enabled        bool
	RetentionHours int
	ScopeUserIDs   []int64
	ScopeGroupIDs  []int64
}

func newAuditCaptureWriter(w gin.ResponseWriter) *auditCaptureWriter {
	return &auditCaptureWriter{ResponseWriter: w, limit: requestAuditCaptureLimit}
}

func (w *auditCaptureWriter) Write(data []byte) (int, error) {
	w.capture(data)
	return w.ResponseWriter.Write(data)
}

func (w *auditCaptureWriter) WriteString(s string) (int, error) {
	w.capture([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

func (w *auditCaptureWriter) capture(data []byte) {
	if w == nil || len(data) == 0 || len(w.buf) >= w.limit {
		return
	}
	remaining := w.limit - len(w.buf)
	if len(data) > remaining {
		data = data[:remaining]
	}
	w.buf = append(w.buf, data...)
}

func (w *auditCaptureWriter) Captured() []byte {
	if w == nil || len(w.buf) == 0 {
		return nil
	}
	out := make([]byte, len(w.buf))
	copy(out, w.buf)
	return out
}

func (w *auditCaptureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.Hijack()
}

func (w *auditCaptureWriter) Flush() {
	w.ResponseWriter.Flush()
}

func (w *auditCaptureWriter) CloseNotify() <-chan bool {
	return w.ResponseWriter.CloseNotify()
}

func (w *auditCaptureWriter) Pusher() http.Pusher {
	return w.ResponseWriter.Pusher()
}

func recordRequestAuditBestEffort(parent context.Context, svc *service.RequestAuditLogService, input service.RequestAuditLogCreateInput) {
	if svc == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 3*time.Second)
		defer cancel()
		if err := svc.Create(ctx, input); err != nil {
			logger.L().With(zap.String("component", "handler.request_audit")).Warn("request_audit.create_failed", zap.Error(err))
		}
	}()
}

func resolveRequestAuditCaptureDecision(ctx context.Context, settingService *service.SettingService, userID int64, groupID *int64) requestAuditCaptureDecision {
	if settingService == nil {
		return requestAuditCaptureDecision{}
	}
	settings, err := settingService.GetAllSettings(ctx)
	if err != nil || settings == nil || !settings.RequestAuditEnabled {
		return requestAuditCaptureDecision{}
	}
	decision := requestAuditCaptureDecision{
		Enabled:        service.ShouldCaptureRequestAudit(userID, groupID, settings.RequestAuditUserScope, settings.RequestAuditGroupScope),
		RetentionHours: settings.RequestAuditRetentionHours,
		ScopeUserIDs:   settings.RequestAuditUserScope,
		ScopeGroupIDs:  settings.RequestAuditGroupScope,
	}
	return decision
}

func reqModelForAudit(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get("request_audit_model"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func streamForAudit(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if v, ok := c.Get("request_audit_stream"); ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
