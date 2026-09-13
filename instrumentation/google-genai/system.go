package googlegenai

import "google.golang.org/genai"

// Backend-aware gen_ai.system values for Google GenAI.
const (
	systemGeminiAPI = "gcp.gemini"
	systemVertexAI  = "gcp.vertex_ai"
	systemFallback  = "gcp.gen_ai"
)

// ResolveSystem maps a genai.Backend to the backend-aware
// gen_ai.system value. It is total for every genai.Backend value,
// including zero/unknown values.
func ResolveSystem(backend genai.Backend) string {
	switch backend {
	case genai.BackendGeminiAPI:
		return systemGeminiAPI
	case genai.BackendVertexAI:
		return systemVertexAI
	default:
		return systemFallback
	}
}
