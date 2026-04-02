package engine

import (
	"strings"
	"testing"
)

// TestInterpConformance verifies that the mvdan/sh interpreter (EmbeddedShell)
// correctly handles shell features used by anneal providers and stdlib functions.
// These tests serve as a regression net: if mvdan/sh changes behavior on any
// of these patterns, the test will catch it before it breaks real plans.
func TestInterpConformance(t *testing.T) {
	tests := []struct {
		name       string
		script     string
		wantOutput string // substring expected in output
		wantErr    bool   // true if script should fail (non-zero exit)
	}{
		// 1. Heredoc — used by stdlib_file_write for content injection
		{
			name: "heredoc basic",
			script: `cat <<'EOF'
hello world
EOF`,
			wantOutput: "hello world",
		},
		// 2. Heredoc with single-quote delimiter — prevents variable expansion
		{
			name: "heredoc no expansion",
			script: `_var="should not expand"
cat <<'EOF'
$_var
EOF`,
			wantOutput: "$_var",
		},
		// 3. Command substitution — used in stdlib for dirname, mktemp, etc.
		{
			name:       "command substitution",
			script:     `_result="$(echo hello)"; echo "$_result"`,
			wantOutput: "hello",
		},
		// 4. Conditional — used for directory existence checks in stdlib
		{
			name:       "conditional test -d",
			script:     `[ -d /tmp ] && echo "exists" || echo "missing"`,
			wantOutput: "exists",
		},
		// 5. Function definition and call — stdlib functions are defined this way
		{
			name: "function definition and call",
			script: `greet() {
  echo "hello $1"
}
greet world`,
			wantOutput: "hello world",
		},
		// 6. Positional parameters — stdlib functions use $1, $2, shift, $@
		{
			name: "positional parameters and shift",
			script: `handle() {
  _first="$1"; shift
  echo "first=$_first rest=$@"
}
handle a b c`,
			wantOutput: "first=a rest=b c",
		},
		// 7. Single-quote escaping — shellQuote produces 'val'\''ue' for embedded quotes
		{
			name:       "single quote escaping",
			script:     `echo 'it'\''s quoted'`,
			wantOutput: "it's quoted",
		},
		// 8. Pipe — used by stdlib_debconf_set, stdlib_apt_key_add
		{
			name:       "pipe",
			script:     `echo "hello world" | tr ' ' '_'`,
			wantOutput: "hello_world",
		},
		// 9. Redirect stderr to /dev/null with || true — used for chown fallback
		{
			name:       "stderr suppress with or-true",
			script:     `ls /nonexistent 2>/dev/null || true; echo "ok"`,
			wantOutput: "ok",
		},
		// 10. set -e exit on error — plan scripts run with set -e
		{
			name:    "set -e stops on failure",
			script:  `set -e; false; echo "should not reach"`,
			wantErr: true,
		},
		// 11. Arithmetic expansion — used by stdlib_docker_health_check
		{
			name:       "arithmetic expansion",
			script:     `_a=5; _b=3; _c=$((_a + _b)); echo "$_c"`,
			wantOutput: "8",
		},
		// 12. Printf — used by stdlib_file_write, stdlib_apt_source_add
		{
			name:       "printf preserves content",
			script:     `printf '%s' "no trailing newline"`,
			wantOutput: "no trailing newline",
		},
		// 13. While loop — used by stdlib_docker_health_check
		{
			name: "while loop with counter",
			script: `_i=0
while [ "$_i" -lt 3 ]; do
  _i=$((_i + 1))
done
echo "$_i"`,
			wantOutput: "3",
		},
		// 14. Variable default value — ${var:-default} pattern
		{
			name:       "variable default value",
			script:     `echo "${_unset:-fallback}"`,
			wantOutput: "fallback",
		},
		// 15. Grep with regex — used by stdlib_hosts_entry, stdlib_crypttab_entry
		{
			name: "grep filtering",
			script: `printf 'alpha\nbeta\ngamma\n' | grep -v beta
`,
			wantOutput: "alpha",
		},
		// 16. Multi-line function with local-style variables — stdlib pattern
		{
			name: "stdlib-style function with locals",
			script: `my_func() {
  _path="$1"; _mode="$2"
  echo "path=$_path mode=$_mode"
}
my_func /etc/hosts 0644`,
			wantOutput: "path=/etc/hosts mode=0644",
		},
		// 17. eval — used by stdlib_command_creates
		{
			name:       "eval executes string as command",
			script:     `_cmd='echo evaluated'; eval "$_cmd"`,
			wantOutput: "evaluated",
		},
		// 18. Conditional with ! negation
		{
			name:       "negated test",
			script:     `if [ ! -f /nonexistent ]; then echo "absent"; fi`,
			wantOutput: "absent",
		},
		// 19. Return from function — stdlib functions use return for error signaling
		{
			name: "function return code",
			script: `check() {
  return 0
}
check && echo "success"`,
			wantOutput: "success",
		},
		// 20. Nested command substitution
		{
			name:       "nested command substitution",
			script:     `echo "$(echo "$(echo nested)")"`,
			wantOutput: "nested",
		},
	}

	shell := EmbeddedShell{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := shell.Execute(tt.script)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got none; output: %s", output)
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute() error = %v; output: %s", err, output)
			}

			if tt.wantOutput != "" && !strings.Contains(output, tt.wantOutput) {
				t.Fatalf("output missing %q:\n%s", tt.wantOutput, output)
			}
		})
	}
}
