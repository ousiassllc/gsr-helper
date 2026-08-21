package audit

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// jst は監査ログの実例（docs/architecture/data-model.md）と同じ +09:00 のタイムゾーン。
func jst() *time.Location {
	return time.FixedZone("JST", 9*60*60)
}

// fixedClock は常に同じ時刻を返す clock を組み立てる。
func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestTimestampMarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "ローカルオフセット付きで出力する",
			in:   time.Date(2026, 8, 21, 12, 0, 0, 0, jst()),
			want: `"2026-08-21T12:00:00+09:00"`,
		},
		{
			name: "UTC は Z になる",
			in:   time.Date(2026, 8, 21, 3, 0, 0, 0, time.UTC),
			want: `"2026-08-21T03:00:00Z"`,
		},
		{
			name: "小数秒は出力しない",
			in:   time.Date(2026, 8, 21, 12, 0, 0, 123456789, jst()),
			want: `"2026-08-21T12:00:00+09:00"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(Timestamp(tt.in))
			if err != nil {
				t.Fatalf("Marshal がエラーを返した: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRecordKeyOrder(t *testing.T) {
	rec := Record{
		TS:         Timestamp(time.Date(2026, 8, 21, 12, 0, 0, 0, jst())),
		UID:        0,
		SudoUser:   "ousiass",
		Action:     "svc.stop",
		Runner:     "build01-2",
		Dir:        "/opt/runners/build01-2",
		Command:    []string{"systemctl", "stop"},
		ExitCode:   1,
		DurationMS: 412,
		Error:      "失敗",
	}

	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal がエラーを返した: %v", err)
	}

	want := `{"ts":"2026-08-21T12:00:00+09:00","uid":0,"sudo_user":"ousiass",` +
		`"action":"svc.stop","runner":"build01-2","dir":"/opt/runners/build01-2",` +
		`"command":["systemctl","stop"],"exit_code":1,"duration_ms":412,"error":"失敗"}`
	if string(b) != want {
		t.Errorf("キー順が仕様と一致しない\n got: %s\nwant: %s", b, want)
	}
}

func TestTimestampUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "オフセット付きを読み戻す", in: `"2026-08-21T12:00:00+09:00"`, want: "2026-08-21T12:00:00+09:00"},
		{name: "Z を読み戻す", in: `"2026-08-21T03:00:00Z"`, want: "2026-08-21T03:00:00Z"},
		{name: "null はゼロ値のまま", in: `null`, want: "0001-01-01T00:00:00Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ts Timestamp
			if err := json.Unmarshal([]byte(tt.in), &ts); err != nil {
				t.Fatalf("Unmarshal がエラーを返した: %v", err)
			}
			if got := time.Time(ts).Format(time.RFC3339); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}

	var ts Timestamp
	if err := json.Unmarshal([]byte(`"2026-08-21 12:00:00"`), &ts); err == nil {
		t.Error("RFC 3339 でない文字列を受け付けてしまった")
	}
}

func TestRecordJSONRoundTrip(t *testing.T) {
	// 監査ログを読み戻す機能を将来作ったときに Record への Unmarshal が通ること。
	want := Record{
		TS:         Timestamp(time.Date(2026, 8, 21, 12, 0, 0, 0, jst())),
		UID:        0,
		SudoUser:   "ousiass",
		Action:     "runner.add",
		Runner:     "build01-4",
		Dir:        "/opt/runners/build01-4",
		Command:    []string{"./config.sh", "--token", "***"},
		ExitCode:   1,
		DurationMS: 412,
		Error:      "失敗",
	}

	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal がエラーを返した: %v", err)
	}

	var got Record
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal がエラーを返した: %v", err)
	}

	// ts は *time.Location の同一性までは保たれないため、書式で比べる。
	if gotTS, wantTS := time.Time(got.TS).Format(time.RFC3339), time.Time(want.TS).Format(time.RFC3339); gotTS != wantTS {
		t.Errorf("ts = %s, want %s", gotTS, wantTS)
	}
	got.TS, want.TS = Timestamp{}, Timestamp{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("読み戻した Record が一致しない\n got: %+v\nwant: %+v", got, want)
	}
}
