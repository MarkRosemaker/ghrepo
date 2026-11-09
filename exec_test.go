package ghrepo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExecError_Error(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  ExecError
		want string
	}{
		{
			"with_output",
			ExecError{Cmd: "go test", Out: "FAIL", Err: errors.New("exit 1")},
			`command "go test" failed:` + "\nFAIL\nexit 1",
		},
		{
			"without_output",
			ExecError{Cmd: "go build", Out: "", Err: errors.New("exit 2")},
			`command "go build" failed: exit 2`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.err.Error(); got != tc.want {
				t.Fatalf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExecError_Unwrap(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("sentinel")
	err := ExecError{Err: sentinel}
	if !errors.Is(err, sentinel) {
		t.Fatal("errors.Is(ExecError, sentinel) = false, want true")
	}
}

func TestExecCommand(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	r := &Repository{path: dir}

	for _, tc := range []struct {
		name    string
		cmd     string
		args    []string
		wantErr bool
		check   func(t *testing.T, out []byte, err error)
	}{
		{
			name: "echo_output",
			cmd:  "echo",
			args: []string{"hello"},
			check: func(t *testing.T, out []byte, err error) {
				t.Helper()

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if got := string(out); len(got) == 0 {
					t.Fatal("expected output, got empty string")
				}
			},
		},
		{
			name: "true_success",
			cmd:  "true",
			check: func(t *testing.T, out []byte, err error) {
				t.Helper()

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			},
		},
		{
			name:    "false_exit",
			cmd:     "false",
			wantErr: true,
			check: func(t *testing.T, out []byte, err error) {
				t.Helper()

				var execErr ExecError
				if !errors.As(err, &execErr) {
					t.Fatalf("expected ExecError, got %T: %v", err, err)
				}

				if execErr.Cmd != "false" {
					t.Fatalf("ExecError.Cmd = %q, want %q", execErr.Cmd, "false")
				}
			},
		},
		{
			name:    "nonexistent_command",
			cmd:     "__no_such_cmd_xyz__",
			wantErr: true,
			check: func(t *testing.T, out []byte, err error) {
				t.Helper()

				if _, ok := errors.AsType[ExecError](err); !ok {
					t.Fatalf("expected ExecError, got %T: %v", err, err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := r.ExecCommand(t.Context(), tc.cmd, tc.args...)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ExecCommand() error = %v, wantErr = %v", err, tc.wantErr)
			}

			if tc.check != nil {
				tc.check(t, out, err)
			}
		})
	}
}

func TestExecCommand_InRepository(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Repository{path: dir}

	out, err := r.ExecCommand(t.Context(), "ls")
	if err != nil || string(out) != "marker\n" {
		t.Fatalf("ls = %q, %v; want it run in the repository", out, err)
	}

	_, err = r.ExecCommand(t.Context(), "sh", "-c", "echo out; echo err >&2; exit 3")

	execErr, ok := errors.AsType[ExecError](err)
	if !ok {
		t.Fatalf("expected ExecError, got %T: %v", err, err)
	}

	if execErr.Cmd != "sh -c echo out; echo err >&2; exit 3" || execErr.Out != "out\nerr" {
		t.Fatalf("ExecError = %+v, want the full command and trimmed combined output", execErr)
	}
}
