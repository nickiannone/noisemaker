package main

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ==============================================================================
// Test Cases:
// ==============================================================================

func TestMain_Execute_Success(t *testing.T) {
	args := []string{"./noisemaker", "execute", "go", "version"}
	output := callMain(args)
	assert.Contains(t, output, "go version go1.23.2")
	assert.Equal(t, activityLogEntry.activity, "execute")
	assert.Equal(t, activityLogEntry.processCmd, "go version")
}

func TestMain_Execute_InvalidPath(t *testing.T) {
	args := []string{"./noisemaker", "execute", "nonexistent-program"}
	output := assertMainPanicsWithMessage(t, args, "exec: \"nonexistent-program\": executable file not found in ")
	assert.Equal(t, output, "")
	assert.Equal(t, activityLogEntry.activity, "execute")
	assert.Equal(t, activityLogEntry.processCmd, "nonexistent-program ")
}

func TestMain_Create_TestFile(t *testing.T) {
	// Precondition: ./test.txt must not exist
	err := deleteTestFileIfExists("./test.txt")
	assert.Nil(t, err)

	args := []string{"./noisemaker", "create", "./test.txt"}
	output := callMain(args)
	defer deleteTestFileIfExists("./test.txt")

	assert.Contains(t, output, "0 bytes written to new file ./test.txt")
	assert.Equal(t, activityLogEntry.activity, "create")
	assert.Equal(t, activityLogEntry.processCmd, "create ./test.txt")
	assert.Equal(t, activityLogEntry.status, "created")
}

// ==============================================================================
// Helpers:
// TODO: Extract test helpers to separate file!
// ==============================================================================

// Calls main(), and returns the console output as a string.
func callMain(args []string) string {
	output, _ := captureOutput(func() error {
		oldArgs := os.Args
		os.Args = args
		main()
		os.Args = oldArgs
		return nil
	})
	return output
}

// https://stackoverflow.com/a/77151975/410342
func captureOutput(f func() error) (string, error) {
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := f()
	os.Stdout = orig
	w.Close()
	out, _ := io.ReadAll(r)
	return string(out), err
}

// https://stackoverflow.com/a/31596110/410342
func assertPanic(t *testing.T, f func([]string) string, args []string) (output string) {
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("The code did not panic")
		}
		fmt.Printf("Recovered variable: %#v\n", r)
	}()
	return f(args)
}

// https://stackoverflow.com/a/31596110/410342
func assertPanicWithMessage(t *testing.T, f func([]string) string, args []string, msg string) (output string) {
	defer func() {
		r := recover()
		if r != nil {
			switch x := r.(type) {
			case string:
				assert.Contains(t, x, msg)
			case error:
				assert.ErrorContains(t, x, msg)
			default:
				// ???
				t.Errorf("unknown panic: %#v", r)
			}
		} else {
			t.Errorf("The code did not panic!")
		}
	}()
	return f(args)
}

// Asserts that calling main() with the given os.Args panics. Returns the console output as a string.
func assertMainPanics(t *testing.T, args []string) string {
	return assertPanic(t, callMain, args)
}

// Asserts that main() with the given os.Args panics, with the given message in the recovered value or error. Returns the console output as a string.
func assertMainPanicsWithMessage(t *testing.T, args []string, msg string) string {
	return assertPanicWithMessage(t, callMain, args, msg)
}

func deleteTestFileIfExists(path string) (err error) {
	if _, err := os.Stat(path); err == nil {
		// Delete file, passing along perm errors, etc.
		return os.Remove(path)
	} else if os.IsNotExist(err) {
		// Suppress file does not exist errors
		return nil
	} else {
		return err
	}
}
