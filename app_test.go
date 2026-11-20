package main

import (
	"context"
	"testing"
	"toolboxself/internal/toolbox"
)

func TestSkillDesktopBindingsPersistAndDelete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	app := NewApp("test")
	app.startup(context.Background())
	defer app.shutdown(context.Background())
	skill, err := app.SaveSkill(toolbox.Skill{Name: "Test", Content: "Only edit text"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := app.GetSkill(skill.ID)
	if err != nil || got.Content != "Only edit text" {
		t.Fatalf("binding read failed: %#v %v", got, err)
	}
	state, err := app.GetState()
	if err != nil || len(state.Skills) != 1 {
		t.Fatalf("library not in state: %#v %v", state, err)
	}
	if err = app.DeleteSkill(skill.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = app.GetSkill(skill.ID); err == nil {
		t.Fatal("deleted skill readable through desktop binding")
	}
}
