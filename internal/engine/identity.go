package engine

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/easel/anneal/internal/manifest"
)

// passwdLookupFunc checks if a user exists and returns their properties.
// Returns (shell, group, exists). Injectable for testing.
var passwdLookupFunc = passwdLookupReal

func passwdLookupReal(name string) (shell string, group string, exists bool) {
	cmd := exec.Command("getent", "passwd", name)
	out, err := cmd.Output()
	if err != nil {
		return "", "", false
	}
	// getent passwd format: name:x:uid:gid:gecos:home:shell
	fields := strings.Split(strings.TrimSpace(string(out)), ":")
	if len(fields) < 7 {
		return "", "", false
	}
	// Resolve gid to group name
	gid := fields[3]
	grpCmd := exec.Command("getent", "group", gid)
	grpOut, err := grpCmd.Output()
	grpName := gid
	if err == nil {
		grpFields := strings.Split(strings.TrimSpace(string(grpOut)), ":")
		if len(grpFields) > 0 {
			grpName = grpFields[0]
		}
	}
	return fields[6], grpName, true
}

// groupExistsFunc checks if a group exists. Injectable for testing.
var groupExistsFunc = groupExistsReal

func groupExistsReal(name string) bool {
	cmd := exec.Command("getent", "group", name)
	return cmd.Run() == nil
}

// userInGroupFunc checks if a user is a member of a group. Injectable for testing.
var userInGroupFunc = userInGroupReal

func userInGroupReal(user, group string) bool {
	cmd := exec.Command("id", "-nG", user)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, g := range strings.Fields(string(out)) {
		if g == group {
			return true
		}
	}
	return false
}

// getfaclFunc reads current ACLs for a path. Injectable for testing.
var getfaclFunc = getfaclReal

func getfaclReal(path string) (string, error) {
	cmd := exec.Command("getfacl", "-p", "--omit-header", path)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// userProvider creates system users.
type userProvider struct{}

func (userProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	name, ok := resource.Spec["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("user spec.name is required")
	}

	desiredShell, _ := resource.Spec["shell"].(string)
	desiredGroup, _ := resource.Spec["group"].(string)
	system, _ := resource.Spec["system"].(bool)

	currentShell, currentGroup, exists := passwdLookupFunc(name)

	if !exists {
		var args []string
		if system {
			args = append(args, "--system")
		}
		if desiredShell != "" {
			args = append(args, "--shell", shellQuote(desiredShell))
		}
		if desiredGroup != "" {
			args = append(args, "--gid", shellQuote(desiredGroup))
		}
		createArgs := ""
		if len(args) > 0 {
			createArgs = " " + strings.Join(args, " ")
		}
		return []string{
			fmt.Sprintf("# create user %s", name),
			fmt.Sprintf("stdlib_user_create %s%s", shellQuote(name), createArgs),
		}, nil
	}

	// User exists — check if properties need updating
	var ops []string
	var modArgs []string

	if desiredShell != "" && desiredShell != currentShell {
		ops = append(ops, fmt.Sprintf("# shell: %s → %s", currentShell, desiredShell))
		modArgs = append(modArgs, "--shell", shellQuote(desiredShell))
	}
	if desiredGroup != "" && desiredGroup != currentGroup {
		ops = append(ops, fmt.Sprintf("# group: %s → %s", currentGroup, desiredGroup))
		modArgs = append(modArgs, "--gid", shellQuote(desiredGroup))
	}

	if len(modArgs) > 0 {
		ops = append(ops, fmt.Sprintf("stdlib_user_modify %s %s", shellQuote(name), strings.Join(modArgs, " ")))
	}

	return ops, nil
}

// groupProvider creates system groups.
type groupProvider struct{}

func (groupProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	name, ok := resource.Spec["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("group spec.name is required")
	}

	system, _ := resource.Spec["system"].(bool)

	if groupExistsFunc(name) {
		return nil, nil // Already exists
	}

	args := ""
	if system {
		args = " --system"
	}
	return []string{
		fmt.Sprintf("# create group %s", name),
		fmt.Sprintf("stdlib_group_create %s%s", shellQuote(name), args),
	}, nil
}

// userInGroupProvider adds users to supplementary groups (additive only).
type userInGroupProvider struct{}

func (userInGroupProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	user, ok := resource.Spec["user"].(string)
	if !ok || user == "" {
		return nil, fmt.Errorf("user_in_group spec.user is required")
	}
	group, ok := resource.Spec["group"].(string)
	if !ok || group == "" {
		return nil, fmt.Errorf("user_in_group spec.group is required")
	}

	if userInGroupFunc(user, group) {
		return nil, nil // Already a member
	}

	return []string{
		fmt.Sprintf("# add %s to group %s", user, group),
		fmt.Sprintf("stdlib_user_add_group %s %s", shellQuote(user), shellQuote(group)),
	}, nil
}

// posixACLProvider sets POSIX ACLs on paths.
type posixACLProvider struct{}

func (posixACLProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	path, ok := resource.Spec["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("posix_acl spec.path is required")
	}

	rawEntries, ok := resource.Spec["entries"]
	if !ok {
		return nil, fmt.Errorf("posix_acl spec.entries is required")
	}
	entryList, ok := rawEntries.([]any)
	if !ok {
		return nil, fmt.Errorf("posix_acl spec.entries must be a list")
	}

	defaultACL, _ := resource.Spec["default"].(bool)

	// Build desired ACL entries
	var desired []string
	for _, e := range entryList {
		entry, ok := e.(string)
		if !ok || entry == "" {
			return nil, fmt.Errorf("posix_acl entries must be non-empty strings")
		}
		desired = append(desired, entry)
	}
	if len(desired) == 0 {
		return nil, nil
	}

	// Check current ACLs
	currentACL, err := getfaclFunc(path)
	if err != nil {
		// Path may not exist — that's an error at plan time
		return nil, fmt.Errorf("posix_acl: reading ACLs for %s: %w", path, err)
	}

	// Check if all desired entries are already present
	allPresent := true
	for _, entry := range desired {
		checkEntry := entry
		if defaultACL {
			checkEntry = "default:" + entry
		}
		if !strings.Contains(currentACL, checkEntry) {
			allPresent = false
			break
		}
	}

	if allPresent {
		return nil, nil
	}

	var ops []string
	for _, entry := range desired {
		aclSpec := entry
		if defaultACL {
			aclSpec = "d:" + entry
		}
		ops = append(ops, fmt.Sprintf("stdlib_setfacl -m %s %s", shellQuote(aclSpec), shellQuote(path)))
	}
	return ops, nil
}

// sudoersEntryProvider manages files in /etc/sudoers.d/.
type sudoersEntryProvider struct{}

func (sudoersEntryProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	filename, ok := resource.Spec["filename"].(string)
	if !ok || filename == "" {
		return nil, fmt.Errorf("sudoers_entry spec.filename is required")
	}
	content, ok := resource.Spec["content"].(string)
	if !ok {
		return nil, fmt.Errorf("sudoers_entry spec.content is required")
	}

	path := "/etc/sudoers.d/" + filename

	// Check if the file already has the correct content
	current, err := os.ReadFile(path)
	if err == nil && strings.TrimSpace(string(current)) == strings.TrimSpace(content) {
		return nil, nil
	}

	delim := uniqueHeredocDelimiter(content)
	return []string{
		fmt.Sprintf("# sudoers entry → %s", path),
		fmt.Sprintf("stdlib_sudoers_write %s <<'%s'\n%s\n%s", shellQuote(path), delim, content, delim),
	}, nil
}
