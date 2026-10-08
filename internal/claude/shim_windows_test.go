//go:build windows

package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// npm installs claude as a claude.cmd shim, which Windows runs through
// cmd.exe. Every argument must either reach claude exactly or make Ask
// refuse; cmd.exe must never split, expand or execute any of it.
func TestAskThroughCmdShim(t *testing.T) {
	req := fakeRequest(t, "args")
	shim := filepath.Join(t.TempDir(), "claude.cmd")
	script := "@echo off\r\n\"" + req.Binary + "\" %*\r\n"
	if err := os.WriteFile(shim, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	req.Binary = shim

	tests := []struct {
		name string
		mod  func(*Request)
		safe bool
	}{
		{"defaults", func(r *Request) {
			r.AddDirs = []string{`C:\repos\front end`, `C:\repos\back\`}
			r.Model, r.Effort, r.SessionID = "sonnet", "low", "0b6c-session"
		}, true},
		{"ampersand in path", func(r *Request) { r.AddDirs = []string{`C:\work\R&D`} }, false},
		{"percent in path", func(r *Request) { r.AddDirs = []string{`C:\work\%PATH%`} }, false},
		{"newline in system prompt", func(r *Request) { r.SystemPrompt = "one\ntwo" }, false},
		{"pipe in extra args", func(r *Request) { r.ExtraArgs = []string{"--x", "a|b"} }, false},
		// Go escapes the quote as \", which ends cmd.exe's quoted region.
		{"odd quote in system prompt", func(r *Request) { r.SystemPrompt = `odd " quote & d` }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := req
			tt.mod(&r)
			ch, err := Ask(context.Background(), r)
			if err != nil {
				if tt.safe {
					t.Fatalf("Ask refused safe arguments: %v", err)
				}
				return
			}
			evs := collect(t, ch, 30*time.Second)
			last := evs[len(evs)-1]
			var got struct{ Args []string }
			if last.Kind != EventDone || json.Unmarshal([]byte(last.Result), &got) != nil {
				t.Fatalf("final event = %+v", last)
			}
			if want := buildArgs(r); !reflect.DeepEqual(got.Args, want) {
				t.Errorf("cmd.exe changed the arguments\n got %q\nwant %q", got.Args, want)
			}
		})
	}
}
