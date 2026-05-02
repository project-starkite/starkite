package modules_test

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/modules/fs"
	"github.com/project-starkite/starkite/libkite/modules/http"
	iomod "github.com/project-starkite/starkite/libkite/modules/io"
	osmod "github.com/project-starkite/starkite/libkite/modules/os"
)

// TestFSModulePermissions verifies that fs module functions check permissions.
func TestFSModulePermissions(t *testing.T) {
	// Create a sandboxed thread that blocks fs operations
	thread := &starlark.Thread{Name: "test"}
	checker, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow:   []string{"strings.*"}, // Only allow strings, not fs
		Default: libkite.DefaultDeny,
	})
	if err != nil {
		t.Fatalf("NewPermissionChecker error: %v", err)
	}
	libkite.SetPermissions(thread, checker)

	// Load the fs module
	fsModule := &fs.Module{}
	exports, err := fsModule.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("fs.Load error: %v", err)
	}

	fsVal := exports["fs"].(starlark.HasAttrs)

	// Get the path factory and create a Path object
	pathFn, err := fsVal.Attr("path")
	if err != nil || pathFn == nil {
		t.Fatalf("fs.path not found: err=%v", err)
	}
	pathBuiltin := pathFn.(*starlark.Builtin)
	pathObj, err := pathBuiltin.CallInternal(thread, starlark.Tuple{starlark.String("/etc/passwd")}, nil)
	if err != nil {
		t.Fatalf("fs.path('/etc/passwd') error: %v", err)
	}
	pathHA := pathObj.(starlark.HasAttrs)

	tests := []struct {
		name     string
		funcName string
		args     starlark.Tuple
		kwargs   []starlark.Tuple
	}{
		{"read_text", "read_text", nil, nil},
		{"write_text", "write_text", starlark.Tuple{starlark.String("data")}, nil},
		{"exists", "exists", nil, nil},
		{"is_file", "is_file", nil, nil},
		{"is_dir", "is_dir", nil, nil},
		{"mkdir", "mkdir", nil, nil},
		{"remove", "remove", nil, nil},
		{"listdir", "listdir", nil, nil},
		{"glob", "glob", starlark.Tuple{starlark.String("*")}, nil},
		{"chmod", "chmod", starlark.Tuple{starlark.MakeInt(0644)}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, err := pathHA.Attr(tt.funcName)
			if err != nil || fn == nil {
				t.Skipf("method %s not found on Path object", tt.funcName)
				return
			}

			builtin, ok := fn.(*starlark.Builtin)
			if !ok {
				t.Skipf("%s is not a builtin function", tt.funcName)
				return
			}

			_, err = builtin.CallInternal(thread, tt.args, tt.kwargs)
			if err == nil {
				t.Errorf("fs.path.%s should have been blocked by permissions", tt.funcName)
				return
			}

			if !libkite.IsPermissionError(err) {
				t.Errorf("fs.path.%s returned non-permission error: %v", tt.funcName, err)
			}
		})
	}
}

// TestOSModulePermissions verifies that os module functions check permissions.
func TestOSModulePermissions(t *testing.T) {
	// Create a sandboxed thread
	thread := &starlark.Thread{Name: "test"}
	checker, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow:   []string{"strings.*"},
		Default: libkite.DefaultDeny,
	})
	if err != nil {
		t.Fatalf("NewPermissionChecker error: %v", err)
	}
	libkite.SetPermissions(thread, checker)

	// Load the os module
	osModule := &osmod.Module{}
	exports, err := osModule.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("os.Load error: %v", err)
	}

	// Get the exec function from aliases (it's a global alias)
	aliases := osModule.Aliases()

	tests := []struct {
		name     string
		funcName string
		source   starlark.StringDict
		args     starlark.Tuple
	}{
		{"exec", "exec", aliases, starlark.Tuple{starlark.String("echo test")}},
		{"env", "env", aliases, starlark.Tuple{starlark.String("HOME")}},
		{"setenv", "setenv", aliases, starlark.Tuple{starlark.String("TEST"), starlark.String("value")}},
	}

	// Also test functions from the module struct
	osVal := exports["os"].(starlark.HasAttrs)
	moduleTests := []struct {
		name     string
		funcName string
		args     starlark.Tuple
	}{
		{"which", "which", starlark.Tuple{starlark.String("ls")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, ok := tt.source[tt.funcName]
			if !ok {
				t.Skipf("function %s not found", tt.funcName)
				return
			}

			builtin, ok := fn.(*starlark.Builtin)
			if !ok {
				t.Skipf("%s is not a builtin function", tt.funcName)
				return
			}

			_, err := builtin.CallInternal(thread, tt.args, nil)
			if err == nil {
				t.Errorf("os.%s should have been blocked by permissions", tt.funcName)
				return
			}

			if !libkite.IsPermissionError(err) {
				t.Errorf("os.%s returned non-permission error: %v", tt.funcName, err)
			}
		})
	}

	for _, tt := range moduleTests {
		t.Run(tt.name, func(t *testing.T) {
			fn, err := osVal.Attr(tt.funcName)
			if err != nil || fn == nil {
				t.Skipf("function %s not found in os module", tt.funcName)
				return
			}

			builtin, ok := fn.(*starlark.Builtin)
			if !ok {
				t.Skipf("%s is not a builtin function", tt.funcName)
				return
			}

			_, err = builtin.CallInternal(thread, tt.args, nil)
			if err == nil {
				t.Errorf("os.%s should have been blocked by permissions", tt.funcName)
				return
			}

			if !libkite.IsPermissionError(err) {
				t.Errorf("os.%s returned non-permission error: %v", tt.funcName, err)
			}
		})
	}
}

// TestHTTPModulePermissions verifies that http module functions check permissions.
func TestHTTPModulePermissions(t *testing.T) {
	// Create a sandboxed thread
	thread := &starlark.Thread{Name: "test"}
	checker, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow:   []string{"strings.*"},
		Default: libkite.DefaultDeny,
	})
	if err != nil {
		t.Fatalf("NewPermissionChecker error: %v", err)
	}
	libkite.SetPermissions(thread, checker)

	// Load the http module
	httpModule := &http.Module{}
	exports, err := httpModule.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("http.Load error: %v", err)
	}

	httpVal := exports["http"].(starlark.HasAttrs)

	// Test config permission check (direct module function)
	t.Run("config", func(t *testing.T) {
		fn, err := httpVal.Attr("config")
		if err != nil || fn == nil {
			t.Skipf("function config not found in http module")
			return
		}

		builtin, ok := fn.(*starlark.Builtin)
		if !ok {
			t.Skipf("config is not a builtin function")
			return
		}

		_, err = builtin.CallInternal(thread, starlark.Tuple{}, nil)
		if err == nil {
			t.Errorf("http.config should have been blocked by permissions")
			return
		}

		if !libkite.IsPermissionError(err) {
			t.Errorf("http.config returned non-permission error: %v", err)
		}
	})

	// Test url().get() permission check (permission checked on URL method)
	t.Run("url_get", func(t *testing.T) {
		urlFn, err := httpVal.Attr("url")
		if err != nil || urlFn == nil {
			t.Skipf("function url not found in http module")
			return
		}

		builtin, ok := urlFn.(*starlark.Builtin)
		if !ok {
			t.Skipf("url is not a builtin function")
			return
		}

		// Create URL object (no permission check on factory)
		urlObj, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String("https://example.com")}, nil)
		if err != nil {
			t.Fatalf("http.url() should not fail: %v", err)
		}

		// Call .get() — should fail with permission error
		urlHA := urlObj.(starlark.HasAttrs)
		getFn, err := urlHA.Attr("get")
		if err != nil || getFn == nil {
			t.Fatalf("get method not found on url object")
		}
		getBuiltin := getFn.(*starlark.Builtin)
		_, err = getBuiltin.CallInternal(thread, starlark.Tuple{}, nil)
		if err == nil {
			t.Errorf("http.url().get() should have been blocked by permissions")
			return
		}
		if !libkite.IsPermissionError(err) {
			t.Errorf("http.url().get() returned non-permission error: %v", err)
		}
	})
}

// TestIOModulePermissions verifies that io module functions check permissions.
func TestIOModulePermissions(t *testing.T) {
	// Create a sandboxed thread
	thread := &starlark.Thread{Name: "test"}
	checker, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow:   []string{"strings.*"},
		Default: libkite.DefaultDeny,
	})
	if err != nil {
		t.Fatalf("NewPermissionChecker error: %v", err)
	}
	libkite.SetPermissions(thread, checker)

	// Load the io module
	ioModule := &iomod.Module{}
	exports, err := ioModule.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("io.Load error: %v", err)
	}

	ioVal := exports["io"].(starlark.HasAttrs)

	ioTests := []struct {
		name     string
		funcName string
		args     starlark.Tuple
	}{
		{"confirm", "confirm", starlark.Tuple{starlark.String("Are you sure?")}},
		{"prompt", "prompt", starlark.Tuple{starlark.String("Enter value:")}},
	}

	for _, tt := range ioTests {
		t.Run(tt.name, func(t *testing.T) {
			fn, err := ioVal.Attr(tt.funcName)
			if err != nil || fn == nil {
				t.Skipf("function %s not found in io module", tt.funcName)
				return
			}

			builtin, ok := fn.(*starlark.Builtin)
			if !ok {
				t.Skipf("%s is not a builtin function", tt.funcName)
				return
			}

			_, err = builtin.CallInternal(thread, tt.args, nil)
			if err == nil {
				t.Errorf("io.%s should have been blocked by permissions", tt.funcName)
				return
			}

			if !libkite.IsPermissionError(err) {
				t.Errorf("io.%s returned non-permission error: %v", tt.funcName, err)
			}
		})
	}
}

// TestModulePermissionsAllowed verifies that modules work when permissions allow.
func TestModulePermissionsAllowed(t *testing.T) {
	// Create a trusted thread
	thread := &starlark.Thread{Name: "test"}
	checker, err := libkite.NewPermissionChecker(libkite.AllowAllPermissions())
	if err != nil {
		t.Fatalf("NewPermissionChecker error: %v", err)
	}
	libkite.SetPermissions(thread, checker)

	// Test that os.env works with trusted permissions
	osModule := &osmod.Module{}
	_, err = osModule.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("os.Load error: %v", err)
	}

	aliases := osModule.Aliases()
	envFn := aliases["env"].(*starlark.Builtin)

	// This should succeed with trusted permissions
	result, err := envFn.CallInternal(thread, starlark.Tuple{starlark.String("HOME")}, nil)
	if err != nil {
		t.Errorf("env('HOME') should succeed with trusted permissions: %v", err)
	}
	if result == nil || result == starlark.None {
		t.Log("HOME not set, but call succeeded (no permission error)")
	}
}

// TestModulePermissionsNilChecker verifies backward compatibility with no checker.
func TestModulePermissionsNilChecker(t *testing.T) {
	// Create a thread with NO permissions set (nil checker = trusted mode)
	thread := &starlark.Thread{Name: "test"}
	// Note: NOT setting any permissions

	// Test that os.env works with nil checker (backward compatible)
	osModule := &osmod.Module{}
	_, err := osModule.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("os.Load error: %v", err)
	}

	aliases := osModule.Aliases()
	envFn := aliases["env"].(*starlark.Builtin)

	// This should succeed with nil checker (backward compatible)
	result, err := envFn.CallInternal(thread, starlark.Tuple{starlark.String("HOME")}, nil)
	if err != nil {
		t.Errorf("env('HOME') should succeed with nil checker (trusted mode): %v", err)
	}
	if result == nil || result == starlark.None {
		t.Log("HOME not set, but call succeeded (no permission error)")
	}
}

// TestSelectivePermissions verifies that selective allow rules work correctly.
func TestSelectivePermissions(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}
	checker, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow: []string{
			"os.env",            // Allow env category (env + setenv); not exec
			"fs.read(exists:*)", // Allow only the exists function in fs.read; not read_file
		},
		Default: libkite.DefaultDeny,
	})
	if err != nil {
		t.Fatalf("NewPermissionChecker error: %v", err)
	}
	libkite.SetPermissions(thread, checker)

	// Load modules
	osModule := &osmod.Module{}
	osModule.Load(&libkite.ModuleConfig{})
	aliases := osModule.Aliases()

	fsModule := &fs.Module{}
	fsExports, _ := fsModule.Load(&libkite.ModuleConfig{})
	fsVal := fsExports["fs"].(starlark.HasAttrs)

	// Get path factory and create Path objects for testing
	pathFn, err := fsVal.Attr("path")
	if err != nil || pathFn == nil {
		t.Fatalf("fs.path not found: err=%v", err)
	}
	pathBuiltin := pathFn.(*starlark.Builtin)

	t.Run("env allowed", func(t *testing.T) {
		envFn := aliases["env"].(*starlark.Builtin)
		_, err := envFn.CallInternal(thread, starlark.Tuple{starlark.String("HOME")}, nil)
		if libkite.IsPermissionError(err) {
			t.Errorf("env should be allowed: %v", err)
		}
	})

	t.Run("exec blocked", func(t *testing.T) {
		execFn := aliases["exec"].(*starlark.Builtin)
		_, err := execFn.CallInternal(thread, starlark.Tuple{starlark.String("echo test")}, nil)
		if !libkite.IsPermissionError(err) {
			t.Errorf("exec should be blocked, got: %v", err)
		}
	})

	t.Run("exists allowed", func(t *testing.T) {
		pathObj, err := pathBuiltin.CallInternal(thread, starlark.Tuple{starlark.String("/tmp")}, nil)
		if err != nil {
			t.Fatalf("fs.path('/tmp') error: %v", err)
		}
		pathHA := pathObj.(starlark.HasAttrs)
		fn, _ := pathHA.Attr("exists")
		existsFn := fn.(*starlark.Builtin)
		_, err = existsFn.CallInternal(thread, nil, nil)
		if libkite.IsPermissionError(err) {
			t.Errorf("exists should be allowed: %v", err)
		}
	})

	t.Run("read_text blocked", func(t *testing.T) {
		pathObj, err := pathBuiltin.CallInternal(thread, starlark.Tuple{starlark.String("/etc/passwd")}, nil)
		if err != nil {
			t.Fatalf("fs.path('/etc/passwd') error: %v", err)
		}
		pathHA := pathObj.(starlark.HasAttrs)
		fn, _ := pathHA.Attr("read_text")
		readFn := fn.(*starlark.Builtin)
		_, err = readFn.CallInternal(thread, nil, nil)
		if !libkite.IsPermissionError(err) {
			t.Errorf("read_text should be blocked, got: %v", err)
		}
	})
}
