package notify

import (
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"
)

// hostile are titles and bodies that would break out of a script if they were
// interpolated into it.
var hostile = []struct{ name, title, body string }{
	{"plain", "Me at the zoo", "Downloaded"},
	{"double quotes", `say "hi" " & do shell script "id`, `"); id; ("`},
	{"single quotes", `it's '; Remove-Item C:\ -Recurse; '`, `'; exit 1; '`},
	{"backslashes", `C:\path\to\"file\\`, `\" & \\ \n`},
	{"command substitution", "$(id) `id` ${HOME} $env:PATH", "$(Remove-Item x)"},
	{"newlines", "first\nend run\ndo shell script \"id\"", "a\r\nb"},
	{"leading dash", "--help", "-urgency=critical"},
	{"empty body", "Title only", ""},
}

func TestBuildPassesTextAsArguments(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "freebsd"} {
		for _, tt := range hostile {
			t.Run(goos+"/"+tt.name, func(t *testing.T) {
				c := Build(goos, tt.title, tt.body)
				if len(c.Env) != 0 {
					t.Errorf("env = %q, want none", c.Env)
				}
				// Title and body are the last two arguments, verbatim, after "--".
				n := len(c.Args)
				if n < 3 || c.Args[n-3] != "--" || c.Args[n-2] != tt.title || c.Args[n-1] != tt.body {
					t.Fatalf("args = %q, want ... -- %q %q", c.Args, tt.title, tt.body)
				}
				assertNotIn(t, c.Args[:n-3], tt.title, tt.body)
			})
		}
	}
}

func TestBuildWindowsPassesTextThroughEnvironment(t *testing.T) {
	for _, tt := range hostile {
		t.Run(tt.name, func(t *testing.T) {
			c := Build("windows", tt.title, tt.body)
			if c.Name != "powershell" {
				t.Errorf("name = %q, want powershell", c.Name)
			}
			for _, kv := range []string{envTitle + "=" + tt.title, envBody + "=" + tt.body} {
				if !slices.Contains(c.Env, kv) {
					t.Errorf("env = %q, missing %q", c.Env, kv)
				}
			}
			i := slices.Index(c.Args, "-EncodedCommand")
			if i < 0 || i+2 != len(c.Args) {
				t.Fatalf("args = %q, want -EncodedCommand <script> last", c.Args)
			}
			script := decodeUTF16(t, c.Args[i+1])
			for _, v := range []string{"$env:" + envTitle, "$env:" + envBody, "ToastNotificationManager"} {
				if !strings.Contains(script, v) {
					t.Errorf("script lacks %q:\n%s", v, script)
				}
			}
			assertNotIn(t, append(slices.Clone(c.Args), script), tt.title, tt.body)
		})
	}
}

func TestBuildCommandPerOS(t *testing.T) {
	for _, tt := range []struct{ goos, name string }{
		{"darwin", "osascript"},
		{"linux", "notify-send"},
		{"freebsd", "notify-send"},
		{"windows", "powershell"},
	} {
		if got := Build(tt.goos, "t", "b").Name; got != tt.name {
			t.Errorf("Build(%q).Name = %q, want %q", tt.goos, got, tt.name)
		}
	}
}

func TestNotifyMissingBinaryIsUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := System{}.Notify("title", "body")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

// assertNotIn fails when any of texts (non-empty) occurs inside args, i.e. was
// interpolated into script text instead of passed on its own.
func assertNotIn(t *testing.T, args []string, texts ...string) {
	t.Helper()
	for _, a := range args {
		for _, s := range texts {
			if s != "" && strings.Contains(a, s) {
				t.Errorf("%q interpolated into %q", s, a)
			}
		}
	}
}

func decodeUTF16(t *testing.T, s string) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(b)%2 != 0 {
		t.Fatalf("odd UTF-16 length %d", len(b))
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u))
}
