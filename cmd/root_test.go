package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootAndDetailWithoutArgsPrintHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"detail"}} {
		got := runHelp(t, args, false)
		want := runHelp(t, append(append([]string{}, args...), "--help"), false)
		if got != want {
			t.Fatalf("args %v help differs\n got:\n%s\nwant:\n%s", args, got, want)
		}
	}
}

func TestDetailExtraArgsPrintsHelpAndError(t *testing.T) {
	out := &bytes.Buffer{}
	command := newRoot()
	command.SetOut(out)
	command.SetErr(out)
	command.SetArgs([]string{"detail", "one", "two"})
	command.SilenceUsage = true
	command.SilenceErrors = true
	err := command.Execute()
	if err == nil || err.Error() != "accepts 1 arg(s), received 2" {
		t.Fatalf("error = %v", err)
	}
	help := runHelp(t, []string{"detail", "--help"}, false)
	if !strings.Contains(out.String(), help) {
		t.Fatalf("help missing from arg error output:\n%s", out.String())
	}
}

func runHelp(t *testing.T, args []string, wantErr bool) string {
	t.Helper()
	out := &bytes.Buffer{}
	command := newRoot()
	command.SetOut(out)
	command.SetErr(out)
	command.SetArgs(args)
	command.SilenceUsage = true
	command.SilenceErrors = true
	err := command.Execute()
	if wantErr && err == nil {
		t.Fatal("expected error")
	}
	if !wantErr && err != nil {
		t.Fatal(err)
	}
	return out.String()
}
