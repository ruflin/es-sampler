package sampler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeEnv struct {
	existing map[string]string
	set      map[string]string
}

func newFakeEnv(existing map[string]string) *fakeEnv {
	if existing == nil {
		existing = map[string]string{}
	}
	return &fakeEnv{existing: existing, set: map[string]string{}}
}

func (f *fakeEnv) Lookup(k string) (string, bool) {
	v, ok := f.existing[k]
	return v, ok
}

func (f *fakeEnv) Set(k, v string) error {
	f.set[k] = v
	f.existing[k] = v
	return nil
}

func parseDotEnvString(t *testing.T, body string, env *fakeEnv, logCapture *[]string) {
	t.Helper()
	var log Logger
	if logCapture != nil {
		log = func(m string) { *logCapture = append(*logCapture, m) }
	}
	if err := loadDotEnvFrom(strings.NewReader(body), ".env", log, env); err != nil {
		t.Fatalf("loadDotEnvFrom: %v", err)
	}
}

func TestLoadDotEnv_BasicKVs(t *testing.T) {
	env := newFakeEnv(nil)
	parseDotEnvString(t, strings.Join([]string{
		"FOO=bar",
		"BAZ=hello world",
		"EMPTY=",
		"EXPORTED=yes", // no `export` prefix on this one
		"export WITH_EXPORT=ok",
	}, "\n"), env, nil)

	want := map[string]string{
		"FOO":         "bar",
		"BAZ":         "hello world",
		"EMPTY":       "",
		"EXPORTED":    "yes",
		"WITH_EXPORT": "ok",
	}
	for k, v := range want {
		if got := env.set[k]; got != v {
			t.Errorf("%s: got %q want %q", k, got, v)
		}
	}
}

func TestLoadDotEnv_CommentsAndBlanks(t *testing.T) {
	env := newFakeEnv(nil)
	parseDotEnvString(t, strings.Join([]string{
		"# top comment",
		"",
		"FOO=bar",
		"  # indented comment",
		"BAZ=qux  # trailing comment",
		"QUOTED=\"hash # stays\"",
	}, "\n"), env, nil)

	if env.set["FOO"] != "bar" {
		t.Errorf("FOO=%q", env.set["FOO"])
	}
	if env.set["BAZ"] != "qux" {
		t.Errorf("BAZ=%q", env.set["BAZ"])
	}
	if env.set["QUOTED"] != "hash # stays" {
		t.Errorf("QUOTED=%q", env.set["QUOTED"])
	}
}

func TestLoadDotEnv_Quotes(t *testing.T) {
	env := newFakeEnv(nil)
	parseDotEnvString(t, strings.Join([]string{
		`DOUBLE="  spaced  "`,
		`SINGLE='  spaced  '`,
		`EQUALS_IN_VALUE="a=b=c"`,
	}, "\n"), env, nil)

	if env.set["DOUBLE"] != "  spaced  " {
		t.Errorf("DOUBLE=%q", env.set["DOUBLE"])
	}
	if env.set["SINGLE"] != "  spaced  " {
		t.Errorf("SINGLE=%q", env.set["SINGLE"])
	}
	if env.set["EQUALS_IN_VALUE"] != "a=b=c" {
		t.Errorf("EQUALS_IN_VALUE=%q", env.set["EQUALS_IN_VALUE"])
	}
}

func TestLoadDotEnv_DoesNotOverrideExistingEnv(t *testing.T) {
	env := newFakeEnv(map[string]string{"FOO": "from-shell"})
	parseDotEnvString(t, "FOO=from-file\nBAR=from-file\n", env, nil)

	if v, ok := env.set["FOO"]; ok {
		t.Errorf("FOO should not have been set, but got %q", v)
	}
	if env.set["BAR"] != "from-file" {
		t.Errorf("BAR=%q", env.set["BAR"])
	}
}

func TestLoadDotEnv_WarnsOnMalformedLines(t *testing.T) {
	env := newFakeEnv(nil)
	var msgs []string
	parseDotEnvString(t, strings.Join([]string{
		"=no-key",
		"not-a-pair",
		"1BAD=x",
		`BROKEN="unterminated`,
		"GOOD=ok",
	}, "\n"), env, &msgs)

	if env.set["GOOD"] != "ok" {
		t.Errorf("valid lines should still load; got %+v", env.set)
	}
	if _, ok := env.set["BROKEN"]; ok {
		t.Errorf("BROKEN should have been skipped")
	}
	if len(msgs) < 3 {
		t.Errorf("expected at least 3 warnings, got %d: %v", len(msgs), msgs)
	}
}

func TestLoadDotEnv_MissingFileIsOK(t *testing.T) {
	dir := t.TempDir()
	if err := LoadDotEnv(filepath.Join(dir, "nope.env"), nil); err != nil {
		t.Fatalf("expected nil for missing file, got %v", err)
	}
}

func TestLoadDotEnv_RealFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("LOAD_DOTENV_TEST_KEY=xyz\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("LOAD_DOTENV_TEST_KEY", "")
	os.Unsetenv("LOAD_DOTENV_TEST_KEY")

	if err := LoadDotEnv(path, nil); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if got := os.Getenv("LOAD_DOTENV_TEST_KEY"); got != "xyz" {
		t.Fatalf("expected %q, got %q", "xyz", got)
	}
}

func TestIsValidEnvKey(t *testing.T) {
	cases := map[string]bool{
		"FOO":     true,
		"FOO_BAR": true,
		"_FOO":    true,
		"foo":     true,
		"FOO123":  true,
		"":        false,
		"1FOO":    false,
		"FOO-BAR": false,
		"FOO BAR": false,
		"FOO.BAR": false,
	}
	for input, want := range cases {
		if got := isValidEnvKey(input); got != want {
			t.Errorf("isValidEnvKey(%q) = %v, want %v", input, got, want)
		}
	}
}
