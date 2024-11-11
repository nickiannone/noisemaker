package main

import (
	"io"
	"os"
	"github.com/stretchr/testify/assert"
	"testing"
)

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

func TestMain_Execute_Success(t *testing.T) {
	expectedOutput := "go version go1.23.2"
	os.Args = []string{"./noisemaker", "execute", "go", "version"}
	actual, _ := captureOutput(func() error {
		main()
		return nil
	})

	assert.Contains(t, actual, expectedOutput)
}