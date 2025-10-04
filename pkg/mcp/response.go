package mcp

import "encoding/json"

// MCPResponse represents a standardized MCP tool response
type MCPResponse struct {
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a single content block in an MCP response
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToMap converts the response to the map format expected by MCP
func (r *MCPResponse) ToMap() map[string]any {
	return map[string]any{
		"content": r.Content,
	}
}

// SuccessResponse creates a successful text response
func SuccessResponse(text string) map[string]any {
	return (&MCPResponse{
		Content: []ContentBlock{
			{Type: "text", Text: text},
		},
	}).ToMap()
}

// ErrorResponse creates an error text response
func ErrorResponse(message string) map[string]any {
	return (&MCPResponse{
		Content: []ContentBlock{
			{Type: "text", Text: message},
		},
	}).ToMap()
}

// UnmarshalArgs unmarshals tool arguments into a typed struct
func UnmarshalArgs[T any](args any) (T, error) {
	var result T
	argsBytes, err := json.Marshal(args)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(argsBytes, &result)
	return result, err
}
