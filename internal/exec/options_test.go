package exec

import (
	"context"
	"slices"
	"testing"
)

func TestOptionsRoundTrip(t *testing.T) {
	want := Options{
		Action: "runner.add",
		Runner: "build01-4",
		Dir:    "/opt/runners/build01-4",
		Env:    []string{"RUNNER_ALLOW_RUNASROOT=1"},
	}

	got := OptionsFrom(WithOptions(context.Background(), want))
	if got.Action != want.Action || got.Runner != want.Runner || got.Dir != want.Dir {
		t.Errorf("OptionsFrom() = %+v, want %+v", got, want)
	}
	if !slices.Equal(got.Env, want.Env) {
		t.Errorf("Env = %q, want %q", got.Env, want.Env)
	}
}

func TestOptionsFromUnsetContext(t *testing.T) {
	if got := OptionsFrom(context.Background()); got.Action != "" || got.Runner != "" || got.Dir != "" || got.Env != nil {
		t.Errorf("未設定の ctx でゼロ値を返していない: %+v", got)
	}
}

func TestOptionsDerivedContextDoesNotAffectParent(t *testing.T) {
	parent := WithOptions(context.Background(), Options{Action: "svc.stop", Runner: "build01-2"})
	child := WithOptions(parent, Options{Action: "svc.start"})

	if got := OptionsFrom(child).Action; got != "svc.start" {
		t.Errorf("派生 ctx の Action = %q, want svc.start", got)
	}
	if got := OptionsFrom(parent).Action; got != "svc.stop" {
		t.Errorf("親 ctx の Action = %q, want svc.stop（上書きされている）", got)
	}
	if got := OptionsFrom(parent).Runner; got != "build01-2" {
		t.Errorf("親 ctx の Runner = %q, want build01-2", got)
	}
}
