package exec

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCommand string
		wantArgs    []string
		wantErr     bool
	}{
		// Empty and Whitespace
		{name: "empty", input: "", wantCommand: "", wantArgs: nil, wantErr: false},
		{name: "only spaces", input: "   \t  \n  ", wantCommand: "", wantArgs: nil, wantErr: false},
		{name: "leading and trailing space", input: "  echo hello  ", wantCommand: "echo", wantArgs: []string{"hello"}},
		{name: "multiple internal spaces", input: "cmd    arg1   arg2", wantCommand: "cmd", wantArgs: []string{"arg1", "arg2"}},
		{name: "single word command", input: "uname", wantCommand: "uname", wantArgs: nil},

		// Single Quotes
		{name: "single quote with spaces", input: "git commit -m 'initial commit'", wantCommand: "git", wantArgs: []string{"commit", "-m", "initial commit"}},
		{name: "single quote with specials", input: "sh -c 'echo $VAR && ls | grep foo'", wantCommand: "sh", wantArgs: []string{"-c", "echo $VAR && ls | grep foo"}},
		{name: "single quote with double quote inside", input: `echo '{"key": "value"}'`, wantCommand: "echo", wantArgs: []string{`{"key": "value"}`}},
		{name: "empty single quotes", input: "cmd '' arg", wantCommand: "cmd", wantArgs: []string{"", "arg"}},
		{name: "adjacent single quotes", input: "cmd 'foo''bar'", wantCommand: "cmd", wantArgs: []string{"foobar"}},

		// Double Quotes
		{name: "double quote with spaces", input: `git commit -m "initial commit"`, wantCommand: "git", wantArgs: []string{"commit", "-m", "initial commit"}},
		{name: "double quote with escaped double quote", input: `echo "hello \"world\""`, wantCommand: "echo", wantArgs: []string{`hello "world"`}},
		{name: "double quote with escaped backslash", input: `echo "path\\to\\file"`, wantCommand: "echo", wantArgs: []string{`path\to\file`}},
		{name: "double quote with single quote inside", input: `echo "don't fail"`, wantCommand: "echo", wantArgs: []string{"don't fail"}},
		{name: "empty double quotes", input: `cmd "" arg`, wantCommand: "cmd", wantArgs: []string{"", "arg"}},
		{name: "adjacent double quotes", input: `cmd "foo""bar"`, wantCommand: "cmd", wantArgs: []string{"foobar"}},
		{name: "mixed adjacent quotes", input: `cmd "foo"'bar'`, wantCommand: "cmd", wantArgs: []string{"foobar"}},

		// Escaped Characters
		{name: "escaped space", input: `cat file\ with\ spaces.txt`, wantCommand: "cat", wantArgs: []string{"file with spaces.txt"}},
		{name: "escaped backslash", input: `cmd \\`, wantCommand: "cmd", wantArgs: []string{`\`}},
		{name: "escaped quote outside quotes", input: `cmd \"arg\"`, wantCommand: "cmd", wantArgs: []string{`"arg"`}},
		{name: "line continuation outside quotes", input: "echo hello \\\n world", wantCommand: "echo", wantArgs: []string{"hello", "world"}},

		// Quoted Executable / Path
		{name: "quoted binary with spaces", input: `"/opt/my tools/bin" --flag`, wantCommand: "/opt/my tools/bin", wantArgs: []string{"--flag"}},
		{name: "windows path with spaces", input: `"C:\Program Files\App\bin.exe" run`, wantCommand: `C:\Program Files\App\bin.exe`, wantArgs: []string{"run"}},

		// Unicode & Multibyte Characters
		{name: "unicode arguments", input: `echo "Hello 世界 🚀"`, wantCommand: "echo", wantArgs: []string{"Hello 世界 🚀"}},
		{name: "accented characters", input: `grep 'café' menu.txt`, wantCommand: "grep", wantArgs: []string{"café", "menu.txt"}},

		// Error Conditions
		{name: "unclosed single quote", input: "echo 'unterminated", wantErr: true},
		{name: "unclosed double quote", input: `echo "unterminated`, wantErr: true},
		{name: "trailing solitary escape", input: `cmd arg\`, wantErr: true},
		{name: "trailing escape inside double quote", input: `cmd "arg\`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCommand, gotArgs, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if gotCommand != tt.wantCommand {
				t.Errorf("Parse(%q) command = %q, want %q", tt.input, gotCommand, tt.wantCommand)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("Parse(%q) args = %#v, want %#v", tt.input, gotArgs, tt.wantArgs)
			}
		})
	}
}

func TestJoinAndQuote(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		args        []string
		wantJoined  string
		wantReParse bool
	}{
		{
			name:        "simple",
			command:     "echo",
			args:        []string{"hello", "world"},
			wantJoined:  "echo hello world",
			wantReParse: true,
		},
		{
			name:        "with spaces",
			command:     "git",
			args:        []string{"commit", "-m", "initial commit"},
			wantJoined:  `git commit -m "initial commit"`,
			wantReParse: true,
		},
		{
			name:        "empty argument",
			command:     "cmd",
			args:        []string{"", "arg"},
			wantJoined:  `cmd "" arg`,
			wantReParse: true,
		},
		{
			name:        "special characters",
			command:     "sh",
			args:        []string{"-c", "echo $VAR && ls | grep foo"},
			wantJoined:  `sh -c "echo \$VAR && ls | grep foo"`,
			wantReParse: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			joined := Join(tt.command, tt.args)
			if joined != tt.wantJoined {
				t.Errorf("Join() = %q, want %q", joined, tt.wantJoined)
			}

			if tt.wantReParse {
				cmd, args, err := Parse(joined)
				if err != nil {
					t.Fatalf("Re-Parse(%q) failed: %v", joined, err)
				}
				if cmd != tt.command {
					t.Errorf("Re-Parse command = %q, want %q", cmd, tt.command)
				}
				if !reflect.DeepEqual(args, tt.args) {
					t.Errorf("Re-Parse args = %#v, want %#v", args, tt.args)
				}
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	// Seed corpus with representative valid and malformed inputs
	f.Add("")
	f.Add("echo hello")
	f.Add("git commit -m 'initial commit'")
	f.Add(`curl -H "Content-Type: application/json" -d '{"k":"v"}'`)
	f.Add(`cat file\ with\ space.txt`)
	f.Add(`"/opt/bin path/tool" --flag="a b c"`)
	f.Add("echo 'unclosed")
	f.Add(`echo "unclosed`)
	f.Add(`cmd \`)
	f.Add("echo '\"\"''\"'\\")
	f.Add("echo \x00\xff\xfe")
	f.Add("echo \n \t \r")

	f.Fuzz(func(t *testing.T, input string) {
		// Invariant 1: Parser must never panic on arbitrary input
		command, args, err := Parse(input)

		if err != nil {
			// Invariant 2: On error, command must be empty and args must be nil
			if command != "" || args != nil {
				t.Fatalf("expected empty command/args on error, got cmd=%q, args=%v", command, args)
			}
			return
		}

		// Invariant 3: Deterministic execution
		command2, args2, err2 := Parse(input)
		if err2 != nil || command != command2 || len(args) != len(args2) {
			t.Fatalf("non-deterministic parse: (%q, %v) vs (%q, %v)", command, args, command2, args2)
		}

		// Invariant 4: For unquoted whitespace-only input, command must be empty and args nil
		if strings.TrimSpace(input) == "" && !strings.ContainsAny(input, `'"`) {
			if command != "" || args != nil {
				t.Fatalf("expected empty result for unquoted whitespace input %q, got cmd=%q, args=%v", input, command, args)
			}
		}
	})
}
