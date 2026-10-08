package access

import (
	"encoding/json"
	"regexp"
	"strings"
)

// VMUsernameValue is the chart value that names the account on a VM (charts/research-vm).
// KubeTRE sets it once, when the VM is created, so it never changes and never forces Azure
// to rebuild the VM.
const VMUsernameValue = "username"

// WithVMUsername returns values with the owner's VM account name added.
func WithVMUsername(values []byte, owner string) ([]byte, error) {
	m := map[string]any{}
	if len(values) > 0 {
		if err := json.Unmarshal(values, &m); err != nil {
			return nil, err
		}
	}
	m[VMUsernameValue] = VMUsername(owner)
	return json.Marshal(m)
}

// FallbackVMUsername is used when a user's email gives no valid VM account name.
const FallbackVMUsername = "researcher"

// Names Azure refuses as a VM's admin account.
var reservedVMUsernames = map[string]bool{
	"1": true, "123": true, "a": true, "actuser": true, "adm": true, "admin": true, "admin1": true, "admin2": true,
	"administrator": true, "aspnet": true, "azureuser": true, "backup": true, "console": true, "david": true,
	"guest": true, "john": true, "owner": true, "root": true, "server": true, "sql": true, "support": true,
	"support_388945a0": true, "sys": true, "test": true, "test1": true, "test2": true, "test3": true,
	"user": true, "user1": true, "user2": true, "user3": true, "user4": true, "user5": true,
}

var vmUsernameInvalid = regexp.MustCompile(`[^a-z0-9_-]+`)

// VMUsername names a user's account on their own VM after them: the local part of their
// email, made valid for both Windows and Linux on Azure (^[a-z_][a-z0-9_-]{0,19}$, not
// reserved). rita.smith@example.org becomes rita-smith.
func VMUsername(principal string) string {
	local, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(principal)), "@")
	name := strings.Trim(vmUsernameInvalid.ReplaceAllString(local, "-"), "-")
	if name != "" && (name[0] < 'a' || name[0] > 'z') && name[0] != '_' {
		name = "u" + name
	}
	if len(name) > 20 {
		name = strings.TrimRight(name[:20], "-")
	}
	if name == "" || reservedVMUsernames[name] {
		return FallbackVMUsername
	}
	return name
}
