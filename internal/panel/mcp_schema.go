package panel

import (
	"slices"
)

// Keep action vocabularies in the permission signature. Extending an underlying
// Panel union must not silently extend an already-issued MCP credential.
func freezeManagedSchema(name string, schema map[string]any) {
	properties := schema["properties"].(map[string]any)
	enum := func(field string, values ...string) {
		property, ok := properties[field].(map[string]any)
		if !ok {
			return
		}
		property["enum"] = values
	}
	switch name {
	case "host_file_list":
		properties["offset"] = map[string]any{"type": "integer", "minimum": 0, "maximum": 100000, "description": "Agent directory cursor. Pass nextOffset from the previous response."}
		properties["limit"].(map[string]any)["maximum"] = 50
	case "host_file_action":
		enum("action", "mkdir", "copy", "move", "rename", "chmod", "compress", "extract", "trash")
	case "host_file_trash_action":
		enum("action", "trash_restore", "trash_delete")
	case "host_system_details":
		sections := make([]string, 0, len(managedSystemSections))
		for section := range managedSystemSections {
			sections = append(sections, section)
		}
		slices.Sort(sections)
		enum("section", sections...)
	case "host_system_resource_action":
		enum("action", "hosts-add", "hosts-delete", "cron-add", "cron-update", "cron-delete", "network-interface-state", "firewall-open-port", "firewall-close-port", "firewall-allow-ip", "firewall-block-ip", "firewall-remove-ip", "firewall-allow-country", "firewall-block-country", "firewall-remove-country", "firewall-open-all", "firewall-close-all", "firewall-enable-ping", "firewall-disable-ping", "firewall-enable-ddos", "firewall-disable-ddos")
	case "host_system_account_action":
		enum("action", "create", "set-password", "add-key", "delete-key", "set-role", "set-ssh-policy", "disable-root", "create-admin-disable-root", "delete")
	case "host_system_ssh_defense_action":
		enum("action", "enable", "disable", "uninstall", "unban-all", "set-profile", "add-trusted", "remove-trusted", "unban")
	case "host_system_tuning_action":
		enum("action", "apply")
	case "host_system_traffic_action":
		enum("action", "enable", "disable")
	case "host_system_partition_action":
		enum("action", "mount", "unmount", "format", "check", "repair")
	case "host_web_action":
		enum("action", "install", "protection.configure", "optimization.apply", "update.component", "update.all", "backup.create", "backup.delete", "restore", "uninstall")
		enum("operation", "fail2ban-install", "fail2ban-uninstall", "unban-all", "waf-on", "waf-off", "ddos-on", "ddos-off", "cloudflare-fail2ban", "cloudflare-shield", "standard", "high", "gzip-on", "gzip-off", "brotli-on", "brotli-off", "zstd-on", "zstd-off")
	}
}
