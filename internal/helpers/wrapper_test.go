package helpers

import (
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShellQuote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"plain", "hello", "'hello'"},
		{"with spaces", "/opt/My App/bin", "'/opt/My App/bin'"},
		{"single quote", "it's", `'it'"'"'s'`},
		{"multiple quotes", "a'b'c", `'a'"'"'b'"'"'c'`},
		{"empty", "", "''"},
		{"double quotes stay literal", `say "hi"`, `'say "hi"'`},
		{"dollar and backtick literal", "$HOME`whoami`", "'$HOME`whoami`'"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ShellQuote(tt.value))
		})
	}
}

// TestCreateWrapper_ExecutesWithHostileArgs verifies BUG-008 end to end: an
// executable whose path contains spaces and a single quote is invoked correctly
// through the generated wrapper, and arguments containing shell metacharacters
// are forwarded verbatim via "$@" without being evaluated.
func TestCreateWrapper_ExecutesWithHostileArgs(t *testing.T) {
	t.Parallel()
	if _, err := osexec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	tmp := t.TempDir()
	// Executable directory and name with a space and a single quote.
	execDir := filepath.Join(tmp, "weird 'app'")
	require.NoError(t, os.MkdirAll(execDir, 0755))
	execPath := filepath.Join(execDir, "run tool")
	argsFile := filepath.Join(tmp, "args.txt")

	// The target records each argv entry on its own line so we can assert exact
	// argument boundaries.
	script := "#!/bin/bash\n: > " + ShellQuote(argsFile) + "\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> " + ShellQuote(argsFile) + "; done\n"
	require.NoError(t, os.WriteFile(execPath, []byte(script), 0755))

	wrapperPath := filepath.Join(tmp, "wrapper.sh")
	require.NoError(t, CreateWrapper(afero.NewOsFs(), WrapperConfig{
		WrapperPath: wrapperPath,
		ExecPath:    execPath,
	}))

	// Hostile arguments: spaces, single quotes, and a command substitution that
	// must NOT be evaluated.
	wantArgs := []string{"plain", "two words", "quote's here", "$(touch pwned)"}
	cmd := osexec.Command("bash", append([]string{wrapperPath}, wantArgs...)...)
	cmd.Dir = tmp
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "wrapper output: %s", out)

	data, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	gotArgs := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	assert.Equal(t, wantArgs, gotArgs)

	// The command substitution must not have run.
	_, statErr := os.Stat(filepath.Join(tmp, "pwned"))
	assert.True(t, os.IsNotExist(statErr), "command substitution should not execute")
}

// TestCreateWrapper_ElectronQuotesDirAndName verifies the Electron branch quotes
// both the cd target and the executable name, keeping any --no-sandbox flag
// outside the quoted word.
func TestCreateWrapper_ElectronQuotesDirAndName(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	execDir := "/opt/electron 'quirky'"
	execPath := execDir + "/launcher"
	// Make it look like an Electron app.
	require.NoError(t, afero.WriteFile(fs, execDir+"/resources/app.asar", []byte("x"), 0644))

	wrapperPath := "/home/user/.local/bin/app"
	require.NoError(t, CreateWrapper(fs, WrapperConfig{
		WrapperPath:    wrapperPath,
		ExecPath:       execPath,
		DisableSandbox: true,
	}))

	data, err := afero.ReadFile(fs, wrapperPath)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "cd "+ShellQuote(execDir))
	assert.Contains(t, content, "exec "+ShellQuote("./launcher")+" --no-sandbox \"$@\"")
	// --no-sandbox must be outside the quotes.
	assert.False(t, strings.Contains(content, ShellQuote("./launcher --no-sandbox")))
}

// TestCreateWrapper_StandardBranchQuotes verifies the non-Electron wrapper
// single-quotes the executable path (with spaces and metacharacters) and never
// emits it as a bare token. MemMapFs keeps IsElectronApp deterministic.
func TestCreateWrapper_StandardBranchQuotes(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	execPath := "/opt/weird app/run$x"
	wrapperPath := "/home/user/.local/bin/app"
	require.NoError(t, CreateWrapper(fs, WrapperConfig{
		WrapperPath: wrapperPath,
		ExecPath:    execPath,
	}))

	data, err := afero.ReadFile(fs, wrapperPath)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "exec "+ShellQuote(execPath)+" \"$@\"")
	assert.NotContains(t, content, "exec "+execPath+" ")
}

// TestCreateWrapper_ElectronFallbackDetection exercises the recursive .asar
// fallback (no resources/app.asar fast path) via the injected MemMapFs, proving
// afero.Walk drives detection deterministically.
func TestCreateWrapper_ElectronFallbackDetection(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	execDir := "/opt/app/bin"
	execPath := execDir + "/launcher"
	// No resources/app.asar; place a .asar in a sibling subdir of the parent.
	require.NoError(t, afero.WriteFile(fs, "/opt/app/extra/bundle.asar", []byte("x"), 0644))

	wrapperPath := "/home/user/.local/bin/app"
	require.NoError(t, CreateWrapper(fs, WrapperConfig{
		WrapperPath: wrapperPath,
		ExecPath:    execPath,
	}))

	data, err := afero.ReadFile(fs, wrapperPath)
	require.NoError(t, err)
	content := string(data)

	// Electron branch emits a cd to the exec directory.
	assert.Contains(t, content, "cd "+ShellQuote(execDir))
	assert.Contains(t, content, "exec "+ShellQuote("./launcher")+" \"$@\"")
}
