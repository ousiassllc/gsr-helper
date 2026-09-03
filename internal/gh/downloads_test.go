package gh_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/ousiassllc/gsr-helper/internal/gh"
)

func TestRunnerDownloadsAndPick(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orgs/foo/actions/runners/downloads" {
			t.Errorf("パス = %q", r.URL.Path)
		}
		fmt.Fprint(w, `[
		 {"os":"linux","architecture":"x64","download_url":"https://example.test/l.tar.gz","filename":"l.tar.gz","sha256_checksum":"ABC"},
		 {"os":"linux","architecture":"arm64","download_url":"https://example.test/a.tar.gz","filename":"a.tar.gz","sha256_checksum":"DEF"}
		]`)
	}))

	list, err := c.RunnerDownloads(context.Background(), orgScope())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("件数 = %d, want 2", len(list))
	}

	d, err := gh.PickDownload(list, "Linux", "X64")
	if err != nil {
		t.Fatalf("PickDownload: %v（大文字小文字を無視すること）", err)
	}
	if d.SHA256 != "ABC" || d.Filename != "l.tar.gz" {
		t.Errorf("選択結果 = %+v", d)
	}

	if _, err := gh.PickDownload(list, "windows", "x64"); !errors.Is(err, gh.ErrNoDownload) {
		t.Errorf("err = %v, want ErrNoDownload", err)
	}
}

func TestHostArchMapsAmd64ToX64(t *testing.T) {
	t.Parallel()

	got := gh.HostArch()
	if got != "x64" && got != "arm64" {
		t.Errorf("HostArch = %q, want x64 か arm64（対象は Linux amd64 / arm64）", got)
	}
}

func TestLatestRunnerVersionTrimsTagPrefix(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/actions/runner/releases/latest" {
			t.Errorf("パス = %q", r.URL.Path)
		}
		fmt.Fprint(w, `{"tag_name":"v2.311.0"}`)
	}))

	got, err := c.LatestRunnerVersion(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "2.311.0" {
		t.Errorf("バージョン = %q, want %q（bin/runnerversion と同じ表記に揃える）", got, "2.311.0")
	}
}

func TestLatestRunnerVersionRejectsEmptyTag(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"tag_name":""}`)
	}))

	if _, err := c.LatestRunnerVersion(context.Background()); !errors.Is(err, gh.ErrNoVersion) {
		t.Errorf("err = %v, want ErrNoVersion", err)
	}
}
