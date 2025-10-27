package manifest

import (
	"encoding/json"
	"fmt"
	"os"
)

// Manifest represents the MCP manifest contract exposed to clients.
type Manifest struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	EntryPoint  string   `json:"entry_point"`
	Resources   []string `json:"resources"`
	Tools       []string `json:"tools"`
}

// Load reads a manifest from disk.
func Load(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	var mf Manifest
	if err := json.Unmarshal(raw, &mf); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}

	return mf, nil
}

// MustLoad wraps Load with panic on error.
func MustLoad(path string) Manifest {
	mf, err := Load(path)
	if err != nil {
		panic(err)
	}
	return mf
}
