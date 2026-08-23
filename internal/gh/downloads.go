package gh

import (
	"context"
	"net/http"
	"runtime"
	"strings"

	"github.com/ousiassllc/gsr-helper/internal/runner/scope"
)

// Download は runner tarball の取得情報。
//
// SHA-256 は downloads エンドポイントが返す値をそのまま運ぶ。自前でハッシュ一覧を
// 持たない（docs/api/external-interfaces.md）。
type Download struct {
	OS       string
	Arch     string
	URL      string
	Filename string
	SHA256   string
}

// downloadItem は downloads エンドポイントのレスポンス 1 件。
type downloadItem struct {
	OS             string `json:"os"`
	Architecture   string `json:"architecture"`
	DownloadURL    string `json:"download_url"`
	Filename       string `json:"filename"`
	SHA256Checksum string `json:"sha256_checksum"`
}

// RunnerDownloads は OS / アーキテクチャごとの tarball 情報を取得する。
//
// GET {scope}/actions/runners/downloads（FR-13）
func (c *Client) RunnerDownloads(ctx context.Context, sc scope.Scope) ([]Download, error) {
	base, err := scopePath(sc)
	if err != nil {
		return nil, wrap("runner_downloads", sc, nil, err)
	}

	var items []downloadItem
	resp, err := c.do(ctx, http.MethodGet, base+"/actions/runners/downloads", &items)
	if err != nil {
		return nil, wrap("runner_downloads", sc, resp, err)
	}

	out := make([]Download, 0, len(items))
	for _, it := range items {
		out = append(out, Download{
			OS:       it.OS,
			Arch:     it.Architecture,
			URL:      it.DownloadURL,
			Filename: it.Filename,
			SHA256:   it.SHA256Checksum,
		})
	}
	return out, nil
}

// HostArch は実行中のホストに対応する downloads エンドポイントの architecture 値を返す。
//
// GitHub は amd64 を x64、arm64 を arm64 と表記する。対象は Linux のみ
// （docs/requirements/non-functional.md の可搬性）。
func HostArch() string {
	if runtime.GOARCH == "amd64" {
		return "x64"
	}
	return runtime.GOARCH
}

// PickDownload は os / arch に一致する 1 件を選ぶ。
//
// 一致するものが無ければ ErrNoDownload を返す。呼び出し側が一覧から自前で
// 探さずに済むよう、選ぶ規則をここに 1 つだけ置く。
func PickDownload(list []Download, os, arch string) (Download, error) {
	for _, d := range list {
		if strings.EqualFold(d.OS, os) && strings.EqualFold(d.Arch, arch) {
			return d, nil
		}
	}
	return Download{OS: "", Arch: "", URL: "", Filename: "", SHA256: ""}, ErrNoDownload
}

// release は releases/latest のレスポンスのうち必要な部分。
type release struct {
	TagName string `json:"tag_name"`
}

// LatestRunnerVersion は runner 本体の最新バージョンを返す（例: 2.311.0）。
//
// GET /repos/actions/runner/releases/latest（FR-20）。タグは v 付きで返るため
// 先頭の v を落として bin/runnerversion と同じ表記に揃える。
func (c *Client) LatestRunnerVersion(ctx context.Context) (string, error) {
	var rel release
	resp, err := c.do(ctx, http.MethodGet, "repos/actions/runner/releases/latest", &rel)
	if err != nil {
		return "", wrap("latest_runner_version", scope.Scope{Kind: scope.Unknown, Owner: "", Repo: ""}, resp, err)
	}

	v := strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
	if v == "" {
		return "", wrap(
			"latest_runner_version",
			scope.Scope{Kind: scope.Unknown, Owner: "", Repo: ""},
			resp,
			ErrNoVersion,
		)
	}
	return v, nil
}
