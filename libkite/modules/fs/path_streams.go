package fs

import (
	"fmt"
	"io"
	"os"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	iomod "github.com/project-starkite/starkite/libkite/modules/io"
)

// getReaderMethod returns an io.reader Starlark value for reading from the path.
func (p *Path) getReaderMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("get_reader", args, kwargs); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "read", "read_file", checkPath(p.path)); err != nil {
		return nil, err
	}

	file, err := os.Open(p.path)
	if err != nil {
		return nil, err
	}

	return iomod.NewReader(file, p.path), nil
}

// getWriterMethod returns an io.writer Starlark value for writing to the path.
func (p *Path) getWriterMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("get_writer", args, kwargs); err != nil {
		return nil, err
	}
	if err := libkite.Check(p.thread, "fs", "write", "write", checkPath(p.path)); err != nil {
		return nil, err
	}

	if p.isDryRun() {
		return iomod.NewWriter(io.Discard, p.path), nil
	}

	file, err := os.OpenFile(p.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	return iomod.NewWriter(file, p.path), nil
}

// writeToMethod writes the contents of this path to the provided writer.
func (p *Path) writeToMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var writerVal starlark.Value
	if err := starlark.UnpackArgs("write_to", args, kwargs, "writer", &writerVal); err != nil {
		return nil, err
	}

	sw, ok := writerVal.(libkite.StarlarkWriter)
	if !ok {
		return nil, fmt.Errorf("fs.path.write_to: expected io.writer, got %s", writerVal.Type())
	}

	if err := libkite.Check(p.thread, "fs", "read", "read_file", checkPath(p.path)); err != nil {
		return nil, err
	}

	file, err := os.Open(p.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	n, err := io.Copy(sw.Writer(), file)
	if err != nil {
		return nil, fmt.Errorf("fs.path.write_to: %w", err)
	}

	return starlark.MakeInt64(n), nil
}

// readFromMethod reads data from the provided reader and writes it to the path (truncating the file).
func (p *Path) readFromMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var readerVal starlark.Value
	if err := starlark.UnpackArgs("read_from", args, kwargs, "reader", &readerVal); err != nil {
		return nil, err
	}

	sr, ok := readerVal.(libkite.StarlarkReader)
	if !ok {
		return nil, fmt.Errorf("fs.path.read_from: expected io.reader, got %s", readerVal.Type())
	}

	if err := libkite.Check(p.thread, "fs", "write", "write", checkPath(p.path)); err != nil {
		return nil, err
	}

	src := sr.Reader()
	defer func() {
		if rc, ok := src.(io.Closer); ok {
			rc.Close()
		} else if closer, ok := sr.(io.Closer); ok {
			closer.Close()
		}
	}()

	if p.isDryRun() {
		return starlark.MakeInt64(0), nil
	}

	file, err := os.OpenFile(p.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	n, err := io.Copy(file, src)
	if err != nil {
		return nil, fmt.Errorf("fs.path.read_from: %w", err)
	}

	return starlark.MakeInt64(n), nil
}

// appendFromMethod reads data from the provided reader and appends it to the path.
func (p *Path) appendFromMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var readerVal starlark.Value
	if err := starlark.UnpackArgs("append_from", args, kwargs, "reader", &readerVal); err != nil {
		return nil, err
	}

	sr, ok := readerVal.(libkite.StarlarkReader)
	if !ok {
		return nil, fmt.Errorf("fs.path.append_from: expected io.reader, got %s", readerVal.Type())
	}

	if err := libkite.Check(p.thread, "fs", "write", "write", checkPath(p.path)); err != nil {
		return nil, err
	}

	src := sr.Reader()
	defer func() {
		if rc, ok := src.(io.Closer); ok {
			rc.Close()
		} else if closer, ok := sr.(io.Closer); ok {
			closer.Close()
		}
	}()

	if p.isDryRun() {
		return starlark.MakeInt64(0), nil
	}

	file, err := os.OpenFile(p.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	n, err := io.Copy(file, src)
	if err != nil {
		return nil, fmt.Errorf("fs.path.append_from: %w", err)
	}

	return starlark.MakeInt64(n), nil
}
