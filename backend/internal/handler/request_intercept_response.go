package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func handleOpenAIChatRequestIntercept(c *gin.Context, settingService *service.SettingService, model string, stream bool, body []byte) bool {
	result, ok := evaluateRequestIntercept(c, settingService, service.RequestInterceptProtocolOpenAIChat, body)
	if !ok {
		return false
	}
	if stream {
		sendOpenAIChatInterceptStream(c, model, result.Content)
	} else {
		sendOpenAIChatInterceptResponse(c, model, result.Content)
	}
	return true
}

func handleAnthropicRequestIntercept(c *gin.Context, settingService *service.SettingService, model string, stream bool, body []byte) bool {
	result, ok := evaluateRequestIntercept(c, settingService, service.RequestInterceptProtocolAnthropic, body)
	if !ok {
		return false
	}
	if stream {
		sendAnthropicInterceptStream(c, model, result.Content)
	} else {
		sendAnthropicInterceptResponse(c, model, result.Content)
	}
	return true
}

func handleOpenAIResponsesRequestIntercept(c *gin.Context, settingService *service.SettingService, model string, stream bool, body []byte) bool {
	result, ok := evaluateRequestIntercept(c, settingService, service.RequestInterceptProtocolOpenAIResponses, body)
	if !ok {
		return false
	}
	if stream {
		sendOpenAIResponsesInterceptStream(c, model, result.Content)
	} else {
		sendOpenAIResponsesInterceptResponse(c, model, result.Content)
	}
	return true
}

func evaluateRequestIntercept(c *gin.Context, settingService *service.SettingService, protocol service.RequestInterceptProtocol, body []byte) (*service.RequestInterceptResult, bool) {
	if settingService == nil {
		return nil, false
	}
	result, err := settingService.EvaluateRequestIntercept(c.Request.Context(), protocol, body)
	if err != nil || result == nil {
		return nil, false
	}
	c.Header("X-Sub2API-Request-Intercepted", result.Reason)
	return result, true
}

func sendOpenAIChatInterceptResponse(c *gin.Context, model string, content string) {
	now := time.Now().Unix()
	c.JSON(http.StatusOK, gin.H{
		"id":      fmt.Sprintf("chatcmpl_intercept_%d", now),
		"object":  "chat.completion",
		"created": now,
		"model":   model,
		"choices": []gin.H{{
			"index": 0,
			"message": gin.H{
				"role":    "assistant",
				"content": content,
			},
			"finish_reason": "stop",
		}},
		"usage": zeroOpenAIUsage(),
	})
}

func sendOpenAIChatInterceptStream(c *gin.Context, model string, content string) {
	now := time.Now().Unix()
	id := fmt.Sprintf("chatcmpl_intercept_%d", now)
	writeSSEJSON(c, "", gin.H{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": now,
		"model":   model,
		"choices": []gin.H{{"index": 0, "delta": gin.H{"role": "assistant"}, "finish_reason": nil}},
	})
	if content != "" {
		writeSSEJSON(c, "", gin.H{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": now,
			"model":   model,
			"choices": []gin.H{{"index": 0, "delta": gin.H{"content": content}, "finish_reason": nil}},
		})
	}
	writeSSEJSON(c, "", gin.H{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": now,
		"model":   model,
		"choices": []gin.H{{"index": 0, "delta": gin.H{}, "finish_reason": "stop"}},
	})
	_, _ = c.Writer.WriteString("data: [DONE]\n\n")
	c.Writer.Flush()
}

func sendAnthropicInterceptResponse(c *gin.Context, model string, content string) {
	c.JSON(http.StatusOK, gin.H{
		"id":            fmt.Sprintf("msg_intercept_%d", time.Now().UnixNano()),
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       []gin.H{{"type": "text", "text": content}},
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage": gin.H{
			"input_tokens":                0,
			"output_tokens":               0,
			"cache_creation_input_tokens": 0,
			"cache_read_input_tokens":     0,
		},
	})
}

func sendAnthropicInterceptStream(c *gin.Context, model string, content string) {
	id := fmt.Sprintf("msg_intercept_%d", time.Now().UnixNano())
	writeSSEJSON(c, "message_start", gin.H{
		"type": "message_start",
		"message": gin.H{
			"id": id, "type": "message", "role": "assistant", "model": model,
			"content": []gin.H{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": gin.H{"input_tokens": 0, "output_tokens": 0},
		},
	})
	writeSSEJSON(c, "content_block_start", gin.H{"type": "content_block_start", "index": 0, "content_block": gin.H{"type": "text", "text": ""}})
	if content != "" {
		writeSSEJSON(c, "content_block_delta", gin.H{"type": "content_block_delta", "index": 0, "delta": gin.H{"type": "text_delta", "text": content}})
	}
	writeSSEJSON(c, "content_block_stop", gin.H{"type": "content_block_stop", "index": 0})
	writeSSEJSON(c, "message_delta", gin.H{"type": "message_delta", "delta": gin.H{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": gin.H{"output_tokens": 0}})
	writeSSEJSON(c, "message_stop", gin.H{"type": "message_stop"})
}

func sendOpenAIResponsesInterceptResponse(c *gin.Context, model string, content string) {
	c.JSON(http.StatusOK, openAIResponsesInterceptPayload(model, content))
}

func sendOpenAIResponsesInterceptStream(c *gin.Context, model string, content string) {
	payload := openAIResponsesInterceptPayload(model, content)
	responseID, _ := payload["id"].(string)
	itemID := "msg_intercept"
	writeSSEJSON(c, "response.created", gin.H{"type": "response.created", "response": openAIResponsesInterceptPayloadWithStatus(model, content, responseID, "in_progress")})
	writeSSEJSON(c, "response.output_item.added", gin.H{"type": "response.output_item.added", "response_id": responseID, "output_index": 0, "item": gin.H{"id": itemID, "type": "message", "status": "in_progress", "role": "assistant", "content": []gin.H{}}})
	writeSSEJSON(c, "response.content_part.added", gin.H{"type": "response.content_part.added", "response_id": responseID, "item_id": itemID, "output_index": 0, "content_index": 0, "part": gin.H{"type": "output_text", "text": "", "annotations": []any{}}})
	if content != "" {
		writeSSEJSON(c, "response.output_text.delta", gin.H{"type": "response.output_text.delta", "response_id": responseID, "item_id": itemID, "output_index": 0, "content_index": 0, "delta": content})
	}
	writeSSEJSON(c, "response.output_text.done", gin.H{"type": "response.output_text.done", "response_id": responseID, "item_id": itemID, "output_index": 0, "content_index": 0, "text": content})
	writeSSEJSON(c, "response.content_part.done", gin.H{"type": "response.content_part.done", "response_id": responseID, "item_id": itemID, "output_index": 0, "content_index": 0, "part": gin.H{"type": "output_text", "text": content, "annotations": []any{}}})
	writeSSEJSON(c, "response.output_item.done", gin.H{"type": "response.output_item.done", "response_id": responseID, "output_index": 0, "item": gin.H{"id": itemID, "type": "message", "status": "completed", "role": "assistant", "content": []gin.H{{"type": "output_text", "text": content, "annotations": []any{}}}}})
	writeSSEJSON(c, "response.completed", gin.H{"type": "response.completed", "response": payload})
}

func openAIResponsesInterceptPayload(model string, content string) gin.H {
	now := time.Now().Unix()
	return gin.H{
		"id":         fmt.Sprintf("resp_intercept_%d", now),
		"object":     "response",
		"created_at": now,
		"status":     "completed",
		"model":      model,
		"output": []gin.H{{
			"id":      "msg_intercept",
			"type":    "message",
			"status":  "completed",
			"role":    "assistant",
			"content": []gin.H{{"type": "output_text", "text": content, "annotations": []any{}}},
		}},
		"output_text": content,
		"usage":       zeroOpenAIUsage(),
	}
}

func openAIResponsesInterceptPayloadWithStatus(model string, content string, id string, status string) gin.H {
	payload := openAIResponsesInterceptPayload(model, content)
	payload["id"] = id
	payload["status"] = status
	return payload
}

func zeroOpenAIUsage() gin.H {
	return gin.H{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}
}

func writeSSEJSON(c *gin.Context, event string, payload any) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	if event != "" {
		_, _ = c.Writer.WriteString("event: " + event + "\n")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte("{}")
	}
	_, _ = c.Writer.WriteString("data: " + string(data) + "\n\n")
	c.Writer.Flush()
}
