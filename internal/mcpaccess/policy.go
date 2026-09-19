package mcpaccess

import (
	"path"
	"slices"
	"strings"
)

// Policy is an explicit snapshot of the granted domains, never a wildcard.
// A nil policy is the original six-tool inspection grant; upgrades cannot
// silently authorize additional tools, files, or mutations.
type Policy struct {
	Domains           []string          `json:"domains"`
	Operations        []string          `json:"operations"`
	OperationVersions map[string]string `json:"operationVersions"`
	Write             bool              `json:"write,omitempty"`
	AutoApprove       bool              `json:"autoApprove,omitempty"`
	FileRoots         []string          `json:"fileRoots,omitempty"`
}

var Domains = []string{"system", "docker", "sites", "apps", "diagnostics", "files", "backups"}

func (p *Policy) Valid() bool {
	if p == nil {
		return true
	}
	if len(p.Domains) == 0 || len(p.Domains) > len(Domains) || len(p.Operations) == 0 || len(p.Operations) > 128 || len(p.FileRoots) > 16 || (p.AutoApprove && !p.Write) {
		return false
	}
	operations := map[string]bool{}
	if len(p.OperationVersions) != len(p.Operations) {
		return false
	}
	for _, name := range p.Operations {
		if len(name) == 0 || len(name) > 128 || operations[name] || !validHex(p.OperationVersions[name], 64) {
			return false
		}
		for _, c := range name {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
				return false
			}
		}
		operations[name] = true
	}
	seen := map[string]bool{}
	for _, d := range p.Domains {
		if !slices.Contains(Domains, d) || seen[d] {
			return false
		}
		seen[d] = true
	}
	if seen["files"] != (len(p.FileRoots) > 0) {
		return false
	}
	for _, root := range p.FileRoots {
		if !validFilePath(root) || path.Clean(root) != root || root == "/" {
			return false
		}
	}
	return true
}

func validFilePath(value string) bool {
	return len(value) <= 4096 && strings.HasPrefix(value, "/") && !strings.ContainsAny(value, "\x00\r\n\\")
}

func (p *Policy) Allows(domain string, write bool) bool {
	return p != nil && (!write || p.Write) && slices.Contains(p.Domains, domain)
}

func (p *Policy) AllowsOperation(name, domain string, write bool) bool {
	return p.Allows(domain, write) && slices.Contains(p.Operations, name)
}

// AllowsPath is only the delegation boundary. The Agent must additionally
// enforce canonical filesystem resolution and its protected-path policy.
func (p *Policy) AllowsPath(value string) bool {
	if p == nil || !validFilePath(value) {
		return false
	}
	clean := path.Clean(value)
	for _, root := range p.FileRoots {
		if clean == root || strings.HasPrefix(clean, root+"/") {
			return true
		}
	}
	return false
}

func clonePolicy(p *Policy) *Policy {
	if p == nil {
		return nil
	}
	copy := *p
	copy.Domains = slices.Clone(p.Domains)
	copy.Operations = slices.Clone(p.Operations)
	copy.OperationVersions = make(map[string]string, len(p.OperationVersions))
	for key, value := range p.OperationVersions {
		copy.OperationVersions[key] = value
	}
	copy.FileRoots = slices.Clone(p.FileRoots)
	return &copy
}
