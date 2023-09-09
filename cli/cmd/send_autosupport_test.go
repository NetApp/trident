package cmd

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestSendAutosupportPrompt(t *testing.T) {
	b := bytes.NewBufferString("")
	RootCmd.SetOut(b)
	RootCmd.SetIn(strings.NewReader("N\n"))
	RootCmd.SetArgs([]string{"send", "autosupport", "--server=dont_exist"})
	if err := RootCmd.Execute(); err != nil {
		t.Log(err)
	}
	out, err := io.ReadAll(b)
	if err != nil {
		t.Fatal(err)
	}
	expected_output := "You must accept the agreement."
	if !strings.Contains(string(out), expected_output) {
		t.Fatalf("output \"%s\" does not contain \"%s\"", string(out), expected_output)
	}

	b = bytes.NewBufferString("")
	RootCmd.SetOut(b)
	RootCmd.SetIn(strings.NewReader("no\n"))
	RootCmd.SetArgs([]string{"send", "autosupport", "--server=dont_exist"})
	if err := RootCmd.Execute(); err != nil {
		t.Log(err)
	}
	out, err = io.ReadAll(b)
	if err != nil {
		t.Fatal(err)
	}
	expected_output = "You must accept the agreement"
	if !strings.Contains(string(out), expected_output) {
		t.Fatalf("output \"%s\" does not contain \"%s\"", string(out), expected_output)
	}

	b = bytes.NewBufferString("")
	RootCmd.SetOut(b)
	RootCmd.SetIn(strings.NewReader("Y\n"))
	RootCmd.SetArgs([]string{"send", "autosupport", "--server=dont_exist"})
	if err := RootCmd.Execute(); err != nil {
		t.Log(err)
	}
	out, err = io.ReadAll(b)
	if err != nil {
		t.Fatal(err)
	}
	unexpected_output := "You must accept the agreement"
	if strings.Contains(string(out), unexpected_output) {
		t.Fatalf("We should continue on if confirmation is provided to the prompt")
	}
}

func TestSendAutosupportAcceptAgreementFlag(t *testing.T) {
	b := bytes.NewBufferString("")
	RootCmd.SetOut(b)
	RootCmd.SetIn(strings.NewReader("no\n"))
	RootCmd.SetArgs([]string{"send", "autosupport", "--accept-agreement=false", "--server=dont_exist"})
	if err := RootCmd.Execute(); err != nil {
		t.Log(err)
	}
	out, err := io.ReadAll(b)
	if err != nil {
		t.Fatal(err)
	}
	expected_output := "You must accept the agreement"
	if !strings.Contains(string(out), expected_output) {
		t.Fatalf("output \"%s\" does not contain \"%s\"", string(out), expected_output)
	}

	b = bytes.NewBufferString("")
	RootCmd.SetOut(b)
	RootCmd.SetArgs([]string{"send", "autosupport", "--accept-agreement", "--server=dont_exist"})
	if err := RootCmd.Execute(); err != nil {
		t.Log(err)
	}
	out, err = io.ReadAll(b)
	if err != nil {
		t.Fatal(err)
	}
	unexpected_output := "You must accept the agreement"
	if strings.Contains(string(out), unexpected_output) {
		t.Fatalf("We should not prompt with the --accept-agreement flag")
	}
}
