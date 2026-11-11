package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const twoMiB = 2 << 20

func TestStripFrontmatter_LineOverOneMiB_BodyKeptWhole(t *testing.T) {
	long := strings.Repeat("A", twoMiB)
	content := "---\nname: big\ndescription: d\n---\nintro line\n" + long + "\nfinal instructions\n"

	got := StripFrontmatter(content)

	want := "intro line\n" + long + "\nfinal instructions"
	if len(got) != len(want) {
		t.Fatalf("body length = %d, want %d (a line over 1 MiB must not cut the body)", len(got), len(want))
	}
	if got != want {
		t.Error("body differs from the input after the frontmatter")
	}
}

func TestStripFrontmatter_LongLineAtEndWithoutNewline(t *testing.T) {
	long := strings.Repeat("B", twoMiB)
	got := StripFrontmatter("---\nname: big\n---\nintro\n" + long)

	if got != "intro\n"+long {
		t.Errorf("body length = %d, want %d", len(got), len("intro\n"+long))
	}
}

func TestParseFrontmatter_FrontmatterLineOverOneMiB_StillSplits(t *testing.T) {
	long := strings.Repeat("C", twoMiB)
	fm, body := ParseFrontmatter("---\nname: big\nnotes: " + long + "\n---\nthe body\n")

	if !strings.HasSuffix(fm, long) {
		t.Errorf("frontmatter lost its long line (len %d)", len(fm))
	}
	if body != "the body" {
		t.Errorf("body length %d, want %q (fence must still be found after a long frontmatter line)", len(body), "the body")
	}
}

func TestParseMetadata_YAML_LineOverOneMiB_NotDropped(t *testing.T) {
	long := strings.Repeat("D", twoMiB)
	m := ParseMetadata("---\nname: big\ndescription: " + long + "\nversion: 2\n---\nbody\n")

	if len(m.Description) != twoMiB {
		t.Errorf("Description length = %d, want %d", len(m.Description), twoMiB)
	}
	if m.Version != "2" {
		t.Errorf("Version = %q, want %q (fields after the long line must still parse)", m.Version, "2")
	}
}

// End to end: a directory tier must hand back the full body.
func TestDirTier_Find_LineOverOneMiB_FullBody(t *testing.T) {
	long := strings.Repeat("E", twoMiB)
	root := t.TempDir()
	dir := filepath.Join(root, "big")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	raw := "---\nname: big\ndescription: d\n---\nintro\n" + long + "\nfinal instructions\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	_, body, ok := NewDirTier("t", root).Resolver.Find("big")
	if !ok || !strings.HasSuffix(body, "final instructions") {
		t.Fatalf("Find(big) = (len %d, %v); the instructions after the long line were lost", len(body), ok)
	}
}
