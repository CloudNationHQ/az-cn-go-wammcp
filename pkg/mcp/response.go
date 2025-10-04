package mcp

import "encoding/json"

type MCPResponse struct {
	Content []ContentBlock `json:"content"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (r *MCPResponse) ToMap() map[string]any {
	return map[string]any{
		"content": r.Content,
	}
}

func SuccessResponse(text string) map[string]any {
	return (&MCPResponse{
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
	}).ToMap()
}

func ErrorResponse(message string) map[string]any {
	return (&MCPResponse{
		Content: []ContentBlock{
			{Type: "text", Text: message},
		},
	}).ToMap()
}

func UnmarshalArgs[T any](args any) (T, error) {
	var result T
	argsBytes, err := json.Marshal(args)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(argsBytes, &result)
	return result, err
}
