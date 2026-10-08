package git

import (
	"slices"
	"testing"

	"github.com/DavidkJun/revue/internal/diff"
)

func TestParseUnifiedTricky(t *testing.T) {
	out := "diff --git a/x.txt b/x.txt\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/x.txt\n" +
		"+++ b/x.txt\n" +
		"@@ -1,3 +1,3 @@ func header\n" +
		" one\r\n" +
		"--- not a header\n" + // a deleted line whose text is "-- not a header"
		"+++ not either\n" +
		"\n" + // diff.suppressBlankEmpty: empty context line
		"@@ -10,2 +10 @@\n" + // a second hunk
		" ten\n" +
		"-eleven\n" +
		"\\ No newline at end of file\n"
	secs := parseUnified([]byte(out))
	if len(secs) != 1 {
		t.Fatalf("got %d sections", len(secs))
	}
	s := secs[0]
	if s.oldPath != "x.txt" || s.newPath != "x.txt" || !s.hunks || s.binary || s.deleted {
		t.Errorf("section = %+v", s)
	}
	want := []diff.Line{
		ctxL(1, 1, "one"),
		delL(2, "-- not a header"),
		addL(2, "++ not either"),
		ctxL(3, 3, ""),
		ctxL(10, 10, "ten"),
		delL(11, "eleven"),
	}
	if !slices.Equal(s.lines, want) {
		t.Errorf("lines:\n got %s\nwant %s", fmtLines(s.lines), fmtLines(want))
	}
}

func TestParseUnifiedHeaders(t *testing.T) {
	out := "diff --git \"a/we\\\"ird\" \"b/we\\\"ird\"\n" +
		"deleted file mode 100644\n" +
		"--- \"a/we\\\"ird\"\n" +
		"+++ /dev/null\n" +
		"@@ -1 +0,0 @@\n" +
		"-x\n" +
		"diff --git a/a b.png b/a b.png\n" +
		"index 1111111..2222222 100644\n" +
		"Binary files a/a b.png and b/a b.png differ\n" +
		"diff --git a/sp ace.txt b/sp ace.txt\n" +
		"old mode 100644\n" +
		"new mode 100755\n" +
		"diff --git a/o.txt b/n.txt\n" +
		"similarity index 100%\n" +
		"rename from o.txt\n" +
		"rename to n.txt\n"
	secs := parseUnified([]byte(out))
	if len(secs) != 4 {
		t.Fatalf("got %d sections", len(secs))
	}
	if s := secs[0]; s.oldPath != `we"ird` || s.newPath != "" || !s.deleted || s.name() != `we"ird` {
		t.Errorf("quoted deletion = %+v", s)
	}
	if s := secs[1]; !s.binary || s.name() != "a b.png" {
		t.Errorf("binary = %+v, name %q", s, s.name())
	}
	if s := secs[2]; s.hunks || s.name() != "sp ace.txt" {
		t.Errorf("mode change = %+v, name %q", s, s.name())
	}
	if s := secs[3]; s.oldPath != "o.txt" || s.newPath != "n.txt" || s.hunks {
		t.Errorf("rename = %+v", s)
	}
}

func TestParseNumstat(t *testing.T) {
	out := "1\t2\ta.txt\x00-\t-\timg.png\x000\t0\t\x00old name.txt\x00new name.txt\x003\t0\tпапка/f.txt\x00"
	got := parseNumstat([]byte(out))
	want := map[string]numstatEntry{
		"a.txt":        {added: 1, deleted: 2},
		"img.png":      {binary: true},
		"new name.txt": {from: "old name.txt"},
		"папка/f.txt":  {added: 3},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %+v, want %+v", k, got[k], w)
		}
	}
}

func TestParsePorcelainHeaders(t *testing.T) {
	out := "# branch.oid (initial)\x00# branch.head (detached)\x00" +
		"# branch.upstream origin/x\x00# branch.ab +3 -4\x00" +
		"? dir/\x00? a b.txt\x00"
	p := parsePorcelain([]byte(out))
	if p.oid != "" || p.branch != "" || p.upstream != "origin/x" || p.ahead != 3 || p.behind != 4 {
		t.Errorf("headers = %+v", p)
	}
	if len(p.files) != 2 || p.files[0].Path != "dir" || !p.files[0].Submodule || p.files[1].Path != "a b.txt" {
		t.Errorf("files = %+v", p.files)
	}
}

func TestKindLetter(t *testing.T) {
	if Untracked.Letter() != "?" || Conflicted.Letter() != "U" || Kind(200).Letter() != " " {
		t.Error("unexpected letters")
	}
}
