package apicompat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(v int) *int       { return &v }
func strPtr(s string) *string { return &s }

func TestStreamingParallelToolUseNoGhostDelta(t *testing.T) {
	ccState := NewChatCompletionsToResponsesStreamState("glm-5.2")
	anthropicState := NewResponsesEventToAnthropicState()
	anthropicState.Model = "glm-5.2"

	chatChunk1 := &ChatCompletionsChunk{
		ID:    "chatcmpl-1",
		Model: "glm-5.2",
		Choices: []ChatChunkChoice{{
			Index: 0,
			Delta: ChatDelta{
				ToolCalls: []ChatToolCall{{
					Index: intPtr(0),
					ID:    "call_weather",
					Type:  "function",
					Function: ChatFunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"Tokyo"}`,
					},
				}},
			},
		}},
	}
	chatChunk2 := &ChatCompletionsChunk{
		ID:    "chatcmpl-1",
		Model: "glm-5.2",
		Choices: []ChatChunkChoice{{
			Index: 0,
			Delta: ChatDelta{
				ToolCalls: []ChatToolCall{{
					Index: intPtr(1),
					ID:    "call_time",
					Type:  "function",
					Function: ChatFunctionCall{
						Name:      "get_time",
						Arguments: `{}`,
					},
				}},
			},
		}},
	}
	chatChunk3 := &ChatCompletionsChunk{
		ID:    "chatcmpl-1",
		Model: "glm-5.2",
		Choices: []ChatChunkChoice{{
			Index:        0,
			Delta:        ChatDelta{},
			FinishReason: strPtr("tool_calls"),
		}},
	}

	var allAnthropicEvents []AnthropicStreamEvent
	for _, chunk := range []*ChatCompletionsChunk{chatChunk1, chatChunk2, chatChunk3} {
		responsesEvents := ChatCompletionsChunkToResponsesEvents(chunk, ccState)
		for _, rEvent := range responsesEvents {
			allAnthropicEvents = append(allAnthropicEvents, ResponsesEventToAnthropicEvents(&rEvent, anthropicState)...)
		}
	}
	for _, rEvent := range FinalizeChatCompletionsResponsesStream(ccState) {
		allAnthropicEvents = append(allAnthropicEvents, ResponsesEventToAnthropicEvents(&rEvent, anthropicState)...)
	}

	startedBlocks := make(map[int]string)
	for _, e := range allAnthropicEvents {
		if e.Type == "content_block_start" && e.ContentBlock != nil {
			startedBlocks[*e.Index] = e.ContentBlock.Type
		}
	}
	for _, e := range allAnthropicEvents {
		if e.Type != "content_block_delta" || e.Index == nil {
			continue
		}
		_, ok := startedBlocks[*e.Index]
		require.Truef(t, ok, "content_block_delta on index %d without content_block_start", *e.Index)
	}

	var toolUseBlocks []int
	for idx, blockType := range startedBlocks {
		if blockType == "tool_use" {
			toolUseBlocks = append(toolUseBlocks, idx)
		}
	}
	assert.Len(t, toolUseBlocks, 2)
}

func TestStreamingThreeParallelToolsAllPackedDone(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:     "response.created",
		Response: &ResponsesResponse{ID: "resp_3par", Model: "glm-5.2"},
	}, state)

	for i, name := range []string{"tool_a", "tool_b", "tool_c"} {
		ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
			Type:        "response.output_item.added",
			OutputIndex: i,
			Item:        &ResponsesOutput{Type: "function_call", CallID: "call_" + name, Name: name},
		}, state)
	}

	started := map[int]bool{0: true, 1: true, 2: true}
	for i, args := range []string{`{"a":1}`, `{"b":2}`, `{"c":3}`} {
		events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
			Type:        "response.function_call_arguments.done",
			OutputIndex: i,
			Arguments:   args,
		}, state)

		for _, e := range events {
			if e.Type == "content_block_delta" && e.Index != nil {
				idx := *e.Index
				require.Truef(t, started[idx], "ghost delta: tool %d emitted delta on index %d", i, idx)
				require.Equal(t, i, idx)
			}
			if e.Type == "content_block_stop" && e.Index != nil {
				require.Truef(t, started[*e.Index], "ghost stop: tool %d emitted stop on index %d", i, *e.Index)
			}
		}
	}
}
