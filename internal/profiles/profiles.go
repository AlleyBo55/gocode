package profiles

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Profile represents a provider launch profile.
type Profile struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	Goal     string `json:"goal,omitempty"`
}

// DefaultProfilePath returns the default profile file path.
func DefaultProfilePath() string {
	return filepath.Join(".gocode", "profile.json")
}

// LoadProfile reads a profile from disk.
func LoadProfile(path string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// SaveProfile writes a profile to disk.
func SaveProfile(path string, p Profile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// AutoDetect scans environment variables and returns the best profile.
func AutoDetect(goal string) Profile {
	p := Profile{Goal: goal}
	switch {
	case envSet("ANTHROPIC_API_KEY"):
		p.Provider = "anthropic"
		p.Model = recommendForGoal(goal, "sonnet", "haiku", "gpt5")
	case envSet("OPENAI_API_KEY"):
		p.Provider = "openai"
		p.Model = recommendForGoal(goal, "gpt5", "gpt4o", "gpt5")
	case envSet("GEMINI_API_KEY"), envSet("GOOGLE_API_KEY"):
		p.Provider = "gemini"
		p.Model = recommendForGoal(goal, "gemini-pro", "gemini-flash", "gemini-pro")
	case envSet("XAI_API_KEY"):
		p.Provider = "xai"
		p.Model = recommendForGoal(goal, "grok", "grok-mini", "grok")
	case envSet("OPENROUTER_API_KEY"):
		p.Provider = "openrouter"
		p.Model = recommendForGoal(goal, "anthropic/claude-sonnet-4-6", "meta-llama/llama-3.3-70b", "openai/gpt-5.4")
	case envSet("TOGETHER_API_KEY"):
		p.Provider = "together"
		p.Model = "meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo"
	case envSet("GROQ_API_KEY"):
		p.Provider = "groq"
		p.Model = "llama-3.3-70b-versatile"
	case envSet("DEEPSEEK_API_KEY"):
		p.Provider = "deepseek"
		p.Model = "deepseek-chat"
	case envSet("MISTRAL_API_KEY"):
		p.Provider = "mistral"
		p.Model = "mistral-large-latest"
	case envSet("OPENAI_BASE_URL"):
		p.Provider = "local"
		p.Model = "llama3.3:70b"
		p.BaseURL = os.Getenv("OPENAI_BASE_URL")
	default:
		p.Provider = "anthropic"
		p.Model = "sonnet"
	}
	return p
}

// Recommend returns a recommended profile for a goal without saving.
func Recommend(goal string) Profile {
	return AutoDetect(goal)
}

func recommendForGoal(goal, coding, latency, balanced string) string {
	switch goal {
	case "coding":
		return coding
	case "latency":
		return latency
	case "balanced":
		return balanced
	default:
		return coding
	}
}

func envSet(key string) bool {
	v := os.Getenv(key)
	return v != ""
}
