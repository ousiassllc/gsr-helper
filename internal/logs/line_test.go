package logs

import "testing"

// 重大度は語として現れる場合だけ拾う（FR-25 の強調表示）。
func TestClassify(t *testing.T) {
	cases := map[string]struct {
		text string
		want Level
	}{
		"ERROR 行":      {"[2026-08-21 12:05:44Z ERROR JobRunner] failed", LevelError},
		"WARN 行":       {"[2026-08-21 12:05:01Z WARN  StepRunner] slow", LevelWarn},
		"INFO 行":       {"[2026-08-21 12:04:35Z INFO  Worker] Job started", LevelPlain},
		"両方あれば重い方":     {"WARN and ERROR", LevelError},
		"語の一部は拾わない":    {"ERRORLEVEL=0", LevelPlain},
		"直前が英数なら拾わない":  {"NOWARN", LevelPlain},
		"記号に挟まれていれば拾う": {"level=[ERROR]", LevelError},
		"小文字は拾わない":     {"an error occurred", LevelPlain},
		"空行":           {"", LevelPlain},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Classify(c.text); got != c.want {
				t.Errorf("Classify(%q) = %v, want %v", c.text, got, c.want)
			}
		})
	}
}

// NewLine は本文と判定した重大度を組にして返す。
func TestNewLineCarriesLevel(t *testing.T) {
	got := NewLine("[WARN] x")
	if got.Text != "[WARN] x" || got.Level != LevelWarn {
		t.Errorf("NewLine = %+v, want {Text:\"[WARN] x\" Level:LevelWarn}", got)
	}
}
