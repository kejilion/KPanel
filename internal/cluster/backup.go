package cluster

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// PruneBackupSecrets operates on already-sanitized backup states, never live files.
// Schema-v1 panel exports are generated after NewService creates all three states.
// Return an error for absent references before any live destination is changed.
func PruneBackupSecrets(files map[string][]byte) error {
	var legacy persistedState
	var v2 persistedStateV2
	var light lightPersistedState
	for name, target := range map[string]any{
		"cluster-state.json":   &legacy,
		clusterStateV2FileName: &v2,
		lightStateFileName:     &light,
	} {
		raw, ok := files[name]
		if !ok || len(raw) == 0 {
			return fmt.Errorf("cluster backup state missing: %s", name)
		}
		if err := json.Unmarshal(raw, target); err != nil {
			return fmt.Errorf("cluster backup state invalid: %s", name)
		}
	}
	allowed := map[string]bool{}
	require := func(name string) error {
		if raw, ok := files[name]; !ok || len(raw) == 0 {
			return fmt.Errorf("cluster backup credential missing: %s", name)
		}
		allowed[name] = true
		return nil
	}
	if err := require(clusterSecretsV2DirectoryName + "/" + nodeIdentityV2FileName); err != nil {
		return err
	}
	for _, record := range legacy.Hosts {
		if !validCredentialName(record.CredentialFile) {
			return errors.New("invalid legacy backup credential reference")
		}
		if err := require("cluster-secrets/" + record.CredentialFile); err != nil {
			return err
		}
	}
	for _, record := range v2.Hosts {
		if record.State != hostStateV2Active || !validCredentialNameV2(record.CredentialFile) || record.PairingCredentialFile != "" {
			return errors.New("backup must contain only finalized v2 host credentials")
		}
		if err := require(clusterSecretsV2DirectoryName + "/" + record.CredentialFile); err != nil {
			return err
		}
	}
	for _, record := range light.Hosts {
		if !validID(record.ID) || !validLightCredentialName(record.CredentialFile) {
			return errors.New("invalid light backup credential reference")
		}
		if err := require(lightSecretsDirectory + "/" + record.CredentialFile); err != nil {
			return err
		}
		// Old telemetry-only nodes intentionally lack a terminal key.
		// Preserve and validate every present key without generating replacements.
		name := lightTerminalKeysDirectory + "/" + terminalKeyName(record.ID)
		if _, ok := files[name]; ok {
			if err := require(name); err != nil {
				return err
			}
		}
	}
	for name := range files {
		root, _, _ := strings.Cut(name, "/")
		switch root {
		case "cluster-secrets", clusterSecretsV2DirectoryName, lightSecretsDirectory, lightTerminalKeysDirectory:
			if !allowed[name] {
				delete(files, name)
			}
		}
	}
	return nil
}

// ValidateBackupReferences reads referenced material only. Call on a temporary
// Service after NewService succeeds, before closing it; it starts no workers.
func (s *Service) ValidateBackupReferences() error {
	for _, record := range s.store.Hosts() {
		if _, err := s.secrets.Read(record.CredentialFile); err != nil {
			return fmt.Errorf("legacy backup credential %s: %w", record.ID, err)
		}
	}
	for _, record := range s.storeV2.Hosts() {
		if record.State != hostStateV2Active {
			return errors.New("backup contains unfinished v2 host")
		}
		credential, err := s.secretsV2.ReadCredential(record.CredentialFile)
		if err != nil {
			return fmt.Errorf("v2 backup credential %s: %w", record.ID, err)
		}
		expected, err := base64.RawURLEncoding.DecodeString(record.TargetPublicKey)
		if err != nil || !bytes.Equal(expected, credential.TargetPublic) {
			return errors.New("v2 backup credential target key mismatch")
		}
	}
	for _, record := range s.light.Hosts() {
		if _, err := s.light.ReadSecret(record); err != nil {
			return fmt.Errorf("light backup credential %s: %w", record.ID, err)
		}
		if _, err := s.light.ReadTerminalPublicKey(record); err != nil {
			return fmt.Errorf("light backup terminal key %s: %w", record.ID, err)
		}
	}
	return nil
}
