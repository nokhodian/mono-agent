package ai

// NewBedrockClient creates a Bedrock client.
// For now it wraps the OpenAI-compatible adapter.
// Full SigV4 auth can be added later.
func NewBedrockClient(apiKey, baseURL, extraHeaders string) AIClient {
	if baseURL == "" {
		baseURL = "https://bedrock-runtime.us-east-1.amazonaws.com"
	}
	return NewOpenAIClient(apiKey, baseURL, extraHeaders)
}
