package test_runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tensorhero-cn/tester-utils/tester_definition"
)

// ============== compileJava ==============

func javacAvailable() bool {
	_, err := exec.LookPath("javac")
	return err == nil
}

func TestCompileJava_Success(t *testing.T) {
	if !javacAvailable() {
		t.Skip("javac not available")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Hello.java"), []byte(`
public class Hello {
    public static void main(String[] args) {
        System.out.println("hello");
    }
}
`), 0644)

	cs := &tester_definition.CompileStep{Source: "Hello.java"}
	err := compileJava(dir, cs)
	require.NoError(t, err)

	// .class file should exist
	assert.FileExists(t, filepath.Join(dir, "Hello.class"))
}

func TestCompileJava_SyntaxError(t *testing.T) {
	if !javacAvailable() {
		t.Skip("javac not available")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Bad.java"), []byte(`public class Bad { invalid`), 0644)

	cs := &tester_definition.CompileStep{Source: "Bad.java"}
	err := compileJava(dir, cs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Output:")
}

func TestCompileJava_WithFlags(t *testing.T) {
	if !javacAvailable() {
		t.Skip("javac not available")
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "tests"), 0755)
	os.WriteFile(filepath.Join(dir, "Greeter.java"), []byte(`
public class Greeter {
    public String greet() { return "hi"; }
}
`), 0644)
	os.WriteFile(filepath.Join(dir, "tests/TestGreeter.java"), []byte(`
public class TestGreeter {
    public static void main(String[] args) {
        System.out.println(new Greeter().greet());
    }
}
`), 0644)

	cs := &tester_definition.CompileStep{
		Source: "Greeter.java",
		Flags:  []string{"tests/TestGreeter.java"},
	}
	err := compileJava(dir, cs)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "Greeter.class"))
	assert.FileExists(t, filepath.Join(dir, "TestGreeter.class"))
}

// ============== checkPythonSyntax ==============

func python3Available() bool {
	_, err := exec.LookPath("python3")
	return err == nil
}

func TestCheckPythonSyntax_Success(t *testing.T) {
	if !python3Available() {
		t.Skip("python3 not available")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "good.py"), []byte("def hello():\n    return 'hi'\n"), 0644)

	cs := &tester_definition.CompileStep{Source: "good.py"}
	err := checkPythonSyntax(dir, cs)
	assert.NoError(t, err)
}

func TestCheckPythonSyntax_Error(t *testing.T) {
	if !python3Available() {
		t.Skip("python3 not available")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.py"), []byte("def bad(\n"), 0644)

	cs := &tester_definition.CompileStep{Source: "bad.py"}
	err := checkPythonSyntax(dir, cs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Output:")
}
