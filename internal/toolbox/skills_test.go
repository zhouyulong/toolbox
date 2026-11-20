package toolbox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testService(t *testing.T) *Service {
	t.Helper()
	root := t.TempDir()
	return &Service{config: Config{SchemaVersion: 1, Providers: []Provider{}, Tools: []ToolRef{}}, configDir: filepath.Join(root, "config"), workspaceDir: filepath.Join(root, "workspaces"), active: map[string]activeRequest{}, version: "test"}
}
func createTestTool(t *testing.T, s *Service, ids ...string) Tool {
	t.Helper()
	tool := toolFromJSON(t, "")
	tool.SkillIDs = ids
	saved, err := s.CreateTool(tool)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}
func saveTestSkill(t *testing.T, s *Service, name, content string) Skill {
	t.Helper()
	skill, err := s.SaveSkill(Skill{Name: name, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	return skill
}

func TestLibraryImportCopiesTextAndDoesNotGrantTools(t *testing.T) {
	s := testService(t)
	tool := createTestTool(t, s)
	source := filepath.Join(t.TempDir(), "SKILL.md")
	text := "---\nname: polish\ndescription: test\n---\nOnly polish the supplied text.\n"
	if err := os.WriteFile(source, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	imported, err := s.ImportSkill(source)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Name != "polish" || imported.Content != text {
		t.Fatalf("wrong import: %#v", imported)
	}
	stored, err := os.ReadFile(filepath.Join(s.configDir, "skills", imported.ID, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != text {
		t.Fatal("library text differs")
	}
	original, _ := os.ReadFile(source)
	if string(original) != text {
		t.Fatal("source overwritten")
	}
	loaded, err := s.tool(tool.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.SkillIDs) != 0 {
		t.Fatal("install automatically granted a tool")
	}
	imported.Content = "updated text"
	if _, err = s.SaveSkill(imported); err != nil {
		t.Fatal(err)
	}
	original, _ = os.ReadFile(source)
	if string(original) != text {
		t.Fatal("editing changed source")
	}
	if err = s.DeleteSkill(imported.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(source); err != nil {
		t.Fatal("deleting removed source")
	}
	if _, err = s.GetSkill(imported.ID); err == nil {
		t.Fatal("deleted skill still accessible")
	}
}

func TestToolGrantsPersistAndOnlyAuthorizedBodiesAreLoaded(t *testing.T) {
	s := testService(t)
	a := saveTestSkill(t, s, "Polish", "POLISH_ONLY")
	b := saveTestSkill(t, s, "Secret", "DO_NOT_LOAD")
	first := createTestTool(t, s, a.ID)
	first.System = "BASE"
	first.SendReasoningEffort = true
	first.ReasoningEffort = "high"
	if _, err := s.SaveTool(first); err != nil {
		t.Fatal(err)
	}
	second := createTestTool(t, s)
	reloaded, err := s.tool(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.SkillIDs) != 1 || reloaded.SkillIDs[0] != a.ID || !reloaded.SendReasoningEffort || reloaded.ReasoningEffort != "high" {
		t.Fatalf("settings lost: %#v", reloaded)
	}
	instructions, err := s.instructions(reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instructions, "BASE") || !strings.Contains(instructions, "POLISH_ONLY") || strings.Contains(instructions, "DO_NOT_LOAD") {
		t.Fatalf("wrong permissions: %q", instructions)
	}
	other, err := s.instructions(second)
	if err != nil || other != "" {
		t.Fatalf("ungranted tool received skills: %q %v", other, err)
	}
	if reloaded.System != "BASE" {
		t.Fatal("original system mutated")
	}
	reloaded.SkillIDs = []string{a.ID, b.ID}
	if _, err = s.SaveTool(reloaded); err != nil {
		t.Fatal(err)
	}
	reloaded.SkillIDs = nil
	if _, err = s.SaveTool(reloaded); err != nil {
		t.Fatal(err)
	}
	reloaded, _ = s.tool(first.ID)
	instructions, err = s.instructions(reloaded)
	if err != nil || instructions != "BASE" {
		t.Fatalf("revoked skill still loaded: %q %v", instructions, err)
	}
	if _, err = s.GetSkill(a.ID); err != nil {
		t.Fatal("revocation removed shared file")
	}
}

func TestSharedSkillUpdatesReachAllGrantedToolsAndDeleteChecksReferences(t *testing.T) {
	s := testService(t)
	skill := saveTestSkill(t, s, "Shared", "old")
	a := createTestTool(t, s, skill.ID)
	b := createTestTool(t, s, skill.ID)
	if err := s.DeleteSkill(skill.ID); err == nil || !strings.Contains(err.Error(), a.Name) {
		t.Fatalf("referenced deletion accepted: %v", err)
	}
	skill.Content = "new"
	if _, err := s.SaveSkill(skill); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []Tool{a, b} {
		text, err := s.instructions(tool)
		if err != nil || !strings.Contains(text, "new") {
			t.Fatalf("update not shared: %q %v", text, err)
		}
	}
	s.active[a.ID] = activeRequest{ID: strings.Repeat("f", 32)}
	if _, err := s.SaveSkill(skill); err == nil {
		t.Fatal("edited a skill used by an active request")
	}
	if _, err := s.SaveTool(a); err == nil {
		t.Fatal("changed grants during an active request")
	}
	delete(s.active, a.ID)
	a.SkillIDs = nil
	b.SkillIDs = nil
	if _, err := s.SaveTool(a); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveTool(b); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSkill(skill.ID); err != nil {
		t.Fatal(err)
	}
}

func TestGrantsRejectMissingDuplicateAndInvalidIDs(t *testing.T) {
	s := testService(t)
	skill := saveTestSkill(t, s, "one", "text")
	tool := createTestTool(t, s)
	for _, ids := range [][]string{{strings.Repeat("f", 32)}, {"../../escape"}, {skill.ID, skill.ID}} {
		tool.SkillIDs = ids
		if _, err := s.SaveTool(tool); err == nil {
			t.Fatalf("invalid grant saved: %v", ids)
		}
		if _, err := s.CreateTool(tool); err == nil {
			t.Fatalf("invalid grant created: %v", ids)
		}
	}
}

func TestLibraryRejectsPathEscapeSymlinksAndOversizedText(t *testing.T) {
	s := testService(t)
	skill := saveTestSkill(t, s, "Safe", "ok")
	for _, id := range []string{"../../outside", "", strings.Repeat("g", 32)} {
		if _, err := s.GetSkill(id); err == nil {
			t.Fatalf("read accepted %q", id)
		}
		if err := s.DeleteSkill(id); err == nil {
			t.Fatalf("delete accepted %q", id)
		}
	}
	if _, err := s.SaveSkill(Skill{Name: "Large", Content: strings.Repeat("x", (256<<10)+1)}); err == nil {
		t.Fatal("oversized text accepted")
	}
	if _, err := s.SaveSkill(Skill{Name: "Binary", Content: "a\x00b"}); err == nil {
		t.Fatal("binary content accepted")
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	os.WriteFile(outside, []byte("secret"), 0600)
	file := filepath.Join(s.configDir, "skills", skill.ID, "SKILL.md")
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, file); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := s.GetSkill(skill.ID); err == nil {
		t.Fatal("followed skill file symlink")
	}
	if _, err := s.SaveSkill(skill); err == nil {
		t.Fatal("wrote via a skill file symlink")
	}
	link := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportSkill(link); err == nil {
		t.Fatal("imported symbolic link")
	}
}

func TestLibraryReportsCorruptSkillWithoutHidingHealthyEntries(t *testing.T) {
	s := testService(t)
	a := saveTestSkill(t, s, "Healthy", "ok")
	b := saveTestSkill(t, s, "Broken", "bad")
	os.Remove(filepath.Join(s.configDir, "skills", b.ID, "SKILL.md"))
	state := s.State()
	if len(state.Skills) != 2 {
		t.Fatalf("skills disappeared: %#v", state.Skills)
	}
	foundHealthy, foundBroken := false, false
	for _, skill := range state.Skills {
		if skill.ID == a.ID && skill.LoadError == "" {
			foundHealthy = true
		}
		if skill.ID == b.ID && skill.LoadError != "" {
			foundBroken = true
		}
	}
	if !foundHealthy || !foundBroken {
		t.Fatalf("bad library state: %#v", state.Skills)
	}
}

func TestSendRejectsMissingGrantBeforeSavingUserMessage(t *testing.T) {
	s := testService(t)
	skill := saveTestSkill(t, s, "one", "text")
	tool := createTestTool(t, s, skill.ID)
	s.config.Providers = []Provider{{ID: "provider", Kind: "openai", BaseURL: "http://127.0.0.1:1", TimeoutSeconds: 10}}
	tool.ProviderID = "provider"
	if _, err := s.SaveTool(tool); err != nil {
		t.Fatal(err)
	}
	session, err := s.NewSession(tool.ID)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(s.configDir, "skills", skill.ID, "SKILL.md"))
	if _, err := s.Send(context.Background(), tool.ID, session.ID, "hello", strings.Repeat("e", 32), nil); err == nil {
		t.Fatal("missing grant was silently skipped")
	}
	chat, err := s.GetSession(tool.ID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chat.Messages) != 0 {
		t.Fatalf("invalid request polluted history: %#v", chat.Messages)
	}
}

func TestLegacyWorkspaceRemainsReadableWithoutNewFields(t *testing.T) {
	s := testService(t)
	tool := createTestTool(t, s)
	file := filepath.Join(tool.Path, workspaceFile)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]any
	json.Unmarshal(data, &legacy)
	delete(legacy, "skillIds")
	delete(legacy, "reasoningEffort")
	delete(legacy, "sendReasoningEffort")
	if err := writeJSON(file, legacy); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.tool(tool.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SendReasoningEffort || len(loaded.SkillIDs) != 0 {
		t.Fatal("legacy defaults changed")
	}
}
