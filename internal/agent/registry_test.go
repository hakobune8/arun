package agent

import "testing"

func TestDefaultRegistryIncludesExpandedBuiltIns(t *testing.T) {
	t.Parallel()

	reg := DefaultRegistry()
	for _, name := range []string{
		"go-backend",
		"frontend",
		"reviewer",
		"ci-fixer",
		"docs",
		"security",
		"release-manager",
		"dependency-updater",
		"qa",
	} {
		if !reg.Has(name) {
			t.Fatalf("DefaultRegistry missing %q", name)
		}
	}

	infos := reg.List()
	if len(infos) != 9 {
		t.Fatalf("got %d built-in agents, want 9", len(infos))
	}
	for i := range infos {
		if infos[i].Description == "" {
			t.Fatalf("%s missing description", infos[i].Name)
		}
		if len(infos[i].RequiredTools) == 0 {
			t.Fatalf("%s missing required tools", infos[i].Name)
		}
		if len(infos[i].Domains) == 0 {
			t.Fatalf("%s missing domains", infos[i].Name)
		}
		if len(infos[i].TriggerKeywords) == 0 {
			t.Fatalf("%s missing trigger keywords", infos[i].Name)
		}
		if len(infos[i].TriggerFiles) == 0 {
			t.Fatalf("%s missing trigger files", infos[i].Name)
		}
		if len(infos[i].ArchitectureGuidance) == 0 {
			t.Fatalf("%s missing architecture guidance", infos[i].Name)
		}
		if len(infos[i].OutputExpectations) == 0 {
			t.Fatalf("%s missing output expectations", infos[i].Name)
		}
	}
}
