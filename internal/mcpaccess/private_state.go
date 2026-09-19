package mcpaccess

import (
	"encoding/json"
	"errors"

	"github.com/kejilion/kejilion-panel/internal/ai"
	"github.com/kejilion/kejilion-panel/internal/backup"
)

// Reuse the existing authenticated-encryption primitive with a separate MCP
// key. Operation arguments can contain a certificate, file text or a backup
// password; they must not be written as plaintext audit/history records.
func writePrivateJSON(path string, value any) error {
	plain, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(plain) > 16<<20 {
		return errors.New("mcp_private_state_limit")
	}
	box, err := ai.OpenSecretBox(path+".key", false)
	if err != nil {
		return err
	}
	ciphertext, err := box.Seal("kpanel-mcp-private-state-v1", string(plain))
	if err != nil {
		return err
	}
	return backup.WriteJSON(path, struct {
		Version    int    `json:"version"`
		Ciphertext []byte `json:"ciphertext"`
	}{1, ciphertext})
}

func readPrivateJSON(path string) ([]byte, error) {
	data, err := backup.ReadFile(path, 24<<20)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Version    int    `json:"version"`
		Ciphertext []byte `json:"ciphertext"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Version != 1 || len(envelope.Ciphertext) == 0 {
		return nil, errors.New("invalid_mcp_private_state")
	}
	box, err := ai.OpenSecretBox(path+".key", true)
	if err != nil {
		return nil, err
	}
	plain, err := box.Open("kpanel-mcp-private-state-v1", envelope.Ciphertext)
	return []byte(plain), err
}
