package runtimeconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitEnvPrecedenceAndLiteralValues(t *testing.T) {
	p := filepath.Join(t.TempDir(), "wiki.env")
	os.WriteFile(p, []byte("WIKI_TEST_EXISTING=from-file\nWIKI_TEST_LITERAL='$(never-run)'\n"), 0600)
	t.Setenv("WIKI_TEST_EXISTING", "from-process")
	t.Cleanup(func() { os.Unsetenv("WIKI_TEST_LITERAL") })
	if e := LoadEnvFile(p); e != nil {
		t.Fatal(e)
	}
	if os.Getenv("WIKI_TEST_EXISTING") != "from-process" || os.Getenv("WIKI_TEST_LITERAL") != "$(never-run)" {
		t.Fatal("env precedence or literal handling incorrect")
	}
	os.Chmod(p, 0644)
	if e := LoadEnvFile(p); e == nil {
		t.Fatal("public env file accepted")
	}
}
