package main

import (
	"errors"

	"os/exec"
	"path/filepath"

	"testing"
)

func TestBoolMatchExecution(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang is required for the native execution test")
	}
	output := filepath.Join(t.TempDir(), "match-bool")
	buildFile(buildConfig{src: "../../test/fixtures/match_bool.gsl", out: output})
	result, err := exec.Command(output).CombinedOutput()
	if err != nil {
		t.Fatalf("bool match executable failed: %v: %s", err, result)
	}
}

func TestTaggedMatchAndPropagationExecution(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang is required for the native execution test")
	}
	output := filepath.Join(t.TempDir(), "match-variants")
	buildFile(buildConfig{src: "../../test/fixtures/match_variants.gsl", out: output})
	result, err := exec.Command(output).CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 22 {
		t.Fatalf("expected exit status 22 from tagged variants, got %v: %s", err, result)
	}
}
