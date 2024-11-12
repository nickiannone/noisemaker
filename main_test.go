package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
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

func TestMain_Create_WithoutContents(t *testing.T) {
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

func TestMain_Create_FileExists(t *testing.T) {
	// Precondition: ./README.md exists (it's in the repo!)
	assert.True(t, fileExists("./README.md"))

	args := []string{"./noisemaker", "create", "./README.md"}
	output := callMain(args)
	assert.Contains(t, output, "File ./README.md already exists, unable to write!")
	assert.Equal(t, activityLogEntry.activity, "create")
	assert.Equal(t, activityLogEntry.processCmd, "create ./README.md")
	assert.Equal(t, activityLogEntry.status, "exists")
}

func TestMain_Create_FileWithoutAccess(t *testing.T) {
	// Precondition: "${getRootDir()}root" doesn't exist, AND we don't have access to create it!
	filePathWithoutAccess := fmt.Sprintf("%s%s", getRootDir(), "root")
	assert.False(t, fileExists(filePathWithoutAccess))

	args := []string{"./noisemaker", "create", filePathWithoutAccess}
	output := callMain(args)
	assert.Contains(t, output, fmt.Sprintf("Error: open %s: Access is denied.", filePathWithoutAccess))
	assert.Equal(t, activityLogEntry.activity, "create")
	assert.Equal(t, activityLogEntry.processCmd, fmt.Sprintf("create %s", filePathWithoutAccess))
	assert.Equal(t, activityLogEntry.status, "error")
}

func TestMain_Create_NotEnoughArguments(t *testing.T) {
	args := []string{"./noisemaker", "create"}
	output := assertMainPanicsWithMessage(t, args, "not enough arguments for create! Args: []")
	assert.Empty(t, output)
	assert.Empty(t, activityLogEntry.status)
}

func TestMain_Create_WithContents(t *testing.T) {
	// Precondition: ./test.txt must not exist
	err := deleteTestFileIfExists("./test.txt")
	assert.Nil(t, err)

	contents := "Hello World!\n------------\n"
	escapedContents := strings.ReplaceAll(contents, "\n", "\\n")
	args := []string{"./noisemaker", "create", "./test.txt", contents}
	output := callMain(args)
	assert.Contains(t, output, fmt.Sprintf("%d bytes written to new file ./test.txt", len(contents)))
	assert.Equal(t, activityLogEntry.activity, "create")
	assert.Equal(t, activityLogEntry.processCmd, fmt.Sprintf("create ./test.txt %s", escapedContents))
	assert.Equal(t, activityLogEntry.status, "created")

	// Postcondition: ./test.txt should be deleted
	err = deleteTestFileIfExists("./test.txt")
	assert.Nil(t, err)
}

func TestMain_Update_WithoutContents(t *testing.T) {
	// Precondition: ./test.txt must exist
	contents := "Hello World!\n------------\n"
	err := createTestFileUnlessExists("./test.txt", contents)
	assert.Nil(t, err)

	args := []string{"./noisemaker", "update", "./test.txt"}
	output := callMain(args)
	assert.Contains(t, output, "0 bytes written to updated file ./test.txt")
	assert.Equal(t, activityLogEntry.activity, "update")
	assert.Equal(t, activityLogEntry.processCmd, "update ./test.txt")
	assert.Equal(t, activityLogEntry.status, "updated")

	// Postcondition: ./test.txt should be deleted
	err = deleteTestFileIfExists("./test.txt")
	assert.Nil(t, err)
}

func TestMain_Update_WithContents(t *testing.T) {
	// Precondition: ./test.txt must exist
	err := createTestFileUnlessExists("./test.txt", "")
	assert.Nil(t, err)

	contents := "Hello World!\n------------\n"
	escapedContents := strings.ReplaceAll(contents, "\n", "\\n")
	args := []string{"./noisemaker", "update", "./test.txt", contents}
	output := callMain(args)
	assert.Contains(t, output, fmt.Sprintf("%d bytes written to updated file ./test.txt", len(contents)))
	assert.Equal(t, activityLogEntry.activity, "update")
	assert.Equal(t, activityLogEntry.processCmd, fmt.Sprintf("update ./test.txt %s", escapedContents))
	assert.Equal(t, activityLogEntry.status, "updated")

	// Postcondition: ./test.txt should be deleted
	err = deleteTestFileIfExists("./test.txt")
	assert.Nil(t, err)
}

// TODO: Figure out a replicable way of achieving this for tests cross-platform!
// func TestMain_Update_FileWithoutAccess(t *testing.T) {
// 	// Precondition: a file exists, AND we don't have access to modify it!
// 	path, err := createOrGetInaccessibleTestFile()
// 	assert.Nil(t, err)
// 	// TODO: Check output and activity log!
// 	fmt.Printf("Inaccessible file at path %s", path)
// }

func TestMain_Update_NonExistentFile(t *testing.T) {
	// TODO: Finish!
}

// ==============================================================================
// Helpers:
// TODO: Extract test helpers to separate file!
// ==============================================================================

// TODO: Figure out a replicable way of achieving this for tests cross-platform!
// func fileExistsAndIsInaccessible(path string) bool {
// 	if _, err := os.Stat(path); err != nil {
// 		fmt.Printf("%#v\n", err)
// 		if os.IsNotExist(err) {
// 			return false
// 		} else if os.IsPermission(err) {
// 			return true
// 		} else {
// 			return false
// 		}
// 	} else {
// 		return false
// 	}	
// }

// Gets the system-native root directory
func getRootDir() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("SYSTEMROOT")
	} else {
		// darwin, linux, etc.
		return "/"
	}
}

// func createOrGetInaccessibleTestFile() (string, error) {
// 	path := "./root_file.txt"
// 	if fileExistsAndIsInaccessible(path) {
// 		return path, nil
// 	}

// 	file, err := os.Create(path)
// 	if err != nil {
// 		return "", err
// 	}
// 	defer file.Close()
	
// 	err = os.Chown(path, 0, 0)
// 	if err != nil {
// 		return "", err
// 	}

// 	return path, nil
// }

// Gets a system-native file which is owned by the root account
// func getInaccessibleFile() string {
// 	if runtime.GOOS == "windows" {
// 		return fmt.Sprintf("%s\\%s", os.Getenv("SYSTEMROOT"), "System32\\calc.exe")
// 	} else {
// 		// darwin, linux, etc.
// 		return "/init"
// 	}
// }

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

func createTestFileUnlessExists(path string, contents string) (err error) {
	if _, err := os.Stat(path); err == nil {
		// It already exists!
		return nil
	} else if os.IsNotExist(err) {
		// Create the file, passing back any errors
		file, err := os.Create(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Write the contents to the file
		_, err = file.WriteString(contents)
		if err != nil {
			return err
		}

		// Flush and return
		file.Sync()
		return nil
	} else {
		return err
	}
}
