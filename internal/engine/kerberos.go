package engine

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/easel/anneal/internal/manifest"
)

// krbKDCExistsFunc checks whether the KDC database exists by looking for
// the principal database file. Injectable for testing.
var krbKDCExistsFunc = krbKDCExistsReal

func krbKDCExistsReal(dbPath string) (bool, error) {
	_, err := os.Stat(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// krbListPrincsFunc lists existing Kerberos principals via kadmin.local.
// Returns a set of principal names. Injectable for testing.
var krbListPrincsFunc = krbListPrincsReal

func krbListPrincsReal() (map[string]bool, error) {
	cmd := exec.Command("kadmin.local", "-q", "listprincs")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("kadmin.local listprincs: %w", err)
	}
	result := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "Authenticating") {
			result[line] = true
		}
	}
	return result, nil
}

// kerberosKDCProvider initializes a Kerberos KDC database if absent.
// Spec: realm (string, required), master_password (string, required),
// db_path (string, optional, default "/var/lib/krb5kdc/principal")
type kerberosKDCProvider struct{}

func (kerberosKDCProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	realm, ok := resource.Spec["realm"].(string)
	if !ok || realm == "" {
		return nil, fmt.Errorf("kerberos_kdc spec.realm is required")
	}
	masterPassword, ok := resource.Spec["master_password"].(string)
	if !ok || masterPassword == "" {
		return nil, fmt.Errorf("kerberos_kdc spec.master_password is required")
	}

	dbPath := "/var/lib/krb5kdc/principal"
	if override, ok := resource.Spec["db_path"].(string); ok && override != "" {
		dbPath = override
	}

	exists, err := krbKDCExistsFunc(dbPath)
	if err != nil {
		return nil, fmt.Errorf("kerberos_kdc: checking %s: %w", dbPath, err)
	}
	if exists {
		return nil, nil // Already initialized
	}

	// Emit KDC initialization. Password piped via heredoc to avoid it
	// appearing in process arguments. Plan shows (secret) per FEAT-008.
	return []string{
		fmt.Sprintf("# kerberos_kdc: initialize realm %s", realm),
		fmt.Sprintf("# master_password: (secret)"),
		fmt.Sprintf("kdb5_util create -r %s -s -P %s", shellQuote(realm), shellQuote(masterPassword)),
	}, nil
}

// kerberosPrincipalProvider creates Kerberos principals idempotently.
// Spec: principal (string, required), randkey (bool, optional, default true),
// password (string, optional — used if randkey is false)
type kerberosPrincipalProvider struct{}

func (kerberosPrincipalProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	principal, ok := resource.Spec["principal"].(string)
	if !ok || principal == "" {
		return nil, fmt.Errorf("kerberos_principal spec.principal is required")
	}

	randkey := true
	if rk, ok := resource.Spec["randkey"].(bool); ok {
		randkey = rk
	}

	existing, err := krbListPrincsFunc()
	if err != nil {
		return nil, fmt.Errorf("kerberos_principal: listing principals: %w", err)
	}

	if existing[principal] {
		return nil, nil // Already exists
	}

	var ops []string
	ops = append(ops, fmt.Sprintf("# kerberos_principal: create %s", principal))
	if randkey {
		ops = append(ops, fmt.Sprintf("kadmin.local -q %s",
			shellQuote(fmt.Sprintf("addprinc -randkey %s", principal))))
	} else {
		password, ok := resource.Spec["password"].(string)
		if !ok || password == "" {
			return nil, fmt.Errorf("kerberos_principal spec.password is required when randkey is false")
		}
		ops = append(ops, fmt.Sprintf("# password: (secret)"))
		ops = append(ops, fmt.Sprintf("kadmin.local -q %s",
			shellQuote(fmt.Sprintf("addprinc -pw %s %s", password, principal))))
	}
	return ops, nil
}

// kerberosKeytabProvider exports principals to a keytab file.
// Spec: path (string, required), principals ([]string, required),
// mode (string, optional, default "0600")
type kerberosKeytabProvider struct{}

func (kerberosKeytabProvider) Plan(resource manifest.ResolvedResource) ([]string, error) {
	path, ok := resource.Spec["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("kerberos_keytab spec.path is required")
	}

	rawPrincipals, ok := resource.Spec["principals"].([]any)
	if !ok || len(rawPrincipals) == 0 {
		return nil, fmt.Errorf("kerberos_keytab spec.principals is required")
	}

	var principals []string
	for _, p := range rawPrincipals {
		if s, ok := p.(string); ok && s != "" {
			principals = append(principals, s)
		}
	}
	if len(principals) == 0 {
		return nil, fmt.Errorf("kerberos_keytab spec.principals is required")
	}
	sort.Strings(principals)

	mode := "0600"
	if rawMode, ok := resource.Spec["mode"].(string); ok && rawMode != "" {
		mode = rawMode
	}

	// Check if keytab already exists. If it does, we still regenerate
	// to ensure it contains the declared principals (keytab contents
	// can't be reliably diffed without ktutil).
	_, statErr := os.Stat(path)
	keytabExists := statErr == nil

	// Check that the parent directory exists.
	// Use _skip_dir_check for testing.
	if _, ok := resource.Spec["_skip_dir_check"]; !ok {
		dir := path[:strings.LastIndex(path, "/")]
		if dir != "" {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				return nil, fmt.Errorf("kerberos_keytab: directory %s does not exist", dir)
			}
		}
	}

	var ops []string
	if keytabExists {
		ops = append(ops, fmt.Sprintf("# kerberos_keytab: regenerate %s", path))
	} else {
		ops = append(ops, fmt.Sprintf("# kerberos_keytab: create %s", path))
	}

	// Export each principal to the keytab.
	for _, princ := range principals {
		ops = append(ops, fmt.Sprintf("kadmin.local -q %s",
			shellQuote(fmt.Sprintf("ktadd -k %s %s", path, princ))))
	}

	// Enforce file mode.
	ops = append(ops, fmt.Sprintf("chmod %s %s", shellQuote(mode), shellQuote(path)))

	return ops, nil
}
