package skills_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anatolykoptev/go-kit/skills"
)

// writeSkill creates <dir>/SKILL.md with a minimal valid skill whose body is body.
func writeSkill(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	raw := "---\nname: x\ndescription: d\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

// traversalFixture lays out
//
//	base/root/good/SKILL.md      (legitimate skill)
//	base/root/SKILL.md           (reachable via "" and ".")
//	base/root/abs/SKILL.md       (reachable via "/abs" after filepath.Join)
//	base/root/a/b/SKILL.md       (reachable via "a/b")
//	base/SKILL.md                (reachable via "..")
//	base/private/SKILL.md        (reachable via "../private")
//	base/x/SKILL.md              (reachable via "a/../../x")
func traversalFixture(t *testing.T) (root string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "root")
	writeSkill(t, filepath.Join(root, "good"), "good body")
	writeSkill(t, root, "root-level body")
	writeSkill(t, filepath.Join(root, "abs"), "abs body")
	writeSkill(t, filepath.Join(root, "a", "b"), "nested body")
	writeSkill(t, base, "base-level body")
	writeSkill(t, filepath.Join(base, "private"), "PRIVATE")
	writeSkill(t, filepath.Join(base, "x"), "X-OUTSIDE")
	return root
}

func TestDirTier_Find_RejectsNamesThatLeaveTheSkillDir(t *testing.T) {
	root := traversalFixture(t)
	tier := skills.NewDirTier("t", root)

	for _, name := range []string{
		"../private",
		"a/../../x",
		"/abs",
		"a/b",
		"..",
		".",
		"",
		"good/../good",
		`..\private`,
	} {
		if info, body, ok := tier.Resolver.Find(name); ok {
			t.Errorf("Find(%q) returned ok=true (path %q, body %q), want false", name, info.Path, body)
		}
	}
}

func TestDirTier_Find_NormalNameStillLoads(t *testing.T) {
	root := traversalFixture(t)
	tier := skills.NewDirTier("t", root)

	_, body, ok := tier.Resolver.Find("good")
	if !ok || body != "good body" {
		t.Fatalf("Find(\"good\") = (%q, %v), want (\"good body\", true)", body, ok)
	}
}

func TestDirTier_Find_SymlinkedSkillDirPointingOutsideIsRejected(t *testing.T) {
	root := traversalFixture(t)
	outside := filepath.Join(filepath.Dir(root), "private")
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	tier := skills.NewDirTier("t", root)

	if _, _, ok := tier.Resolver.Find("linked"); ok {
		t.Error("Find(\"linked\") returned ok=true for a skill dir symlinked outside the root")
	}
}

// A SKILL.md symlink that stays inside a tier root which is itself reached via
// a symlink must still load: the containment check has to compare resolved
// paths on both sides.
func TestDirTier_Find_InRootSymlinkViaSymlinkedRootLoads(t *testing.T) {
	root := traversalFixture(t)
	real := filepath.Join(root, "good", "SKILL.md")
	other := filepath.Join(root, "good", "real.md")
	if err := os.Rename(real, other); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.md", real); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	rootLink := filepath.Join(filepath.Dir(root), "root-link")
	if err := os.Symlink(root, rootLink); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	tier := skills.NewDirTier("t", rootLink)

	_, body, ok := tier.Resolver.Find("good")
	if !ok || body != "good body" {
		t.Fatalf("Find(\"good\") via symlinked root = (%q, %v), want (\"good body\", true)", body, ok)
	}
}

// EmbedFS and Plugin tiers resolve names through maps, never through the
// filesystem, so traversal-shaped names simply miss.
func TestOtherTiers_Find_TraversalShapedNamesMiss(t *testing.T) {
	embed := skills.NewEmbedFSTier("e", embedTestFS, "testdata/tier/embedfs")
	plugin := skills.NewPluginTier("p", []skills.PluginEntry{{PluginName: "pl", SkillName: "foo", Body: "body"}})

	for _, name := range []string{"../private", "a/../../x", "/abs", "a/b", ".."} {
		if _, _, ok := embed.Resolver.Find(name); ok {
			t.Errorf("EmbedFSTier.Find(%q) = ok, want miss", name)
		}
		if _, _, ok := plugin.Resolver.Find(name); ok {
			t.Errorf("PluginTier.Find(%q) = ok, want miss", name)
		}
	}
}
