package logs

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Worker ログ本文の解析を置く（Issue #68）。Jobs タブの REPOSITORY / `_work` 列を
// 埋めるための情報源であり、Tail / Journal の「追従」とは独立した「1 度だけ読んで
// 済ませる」経路である。取り出し口は actions/runner のソース（HEAD 258d6c857）で
// 確認済みのものだけを使う。
//
// **抽出は 1 行ずつ行う。** マーカー以降をファイル全体から探すと、ログが途中で
// 切れている場合に次の行のログ本文まで巻き込み、改行を含む値が列に出る。行に
// 閉じることで、切れた行はその行だけが一致しないという形に収まる。
//
// **両方が見つかった時点で読むのをやめる。** 取り出し口はいずれもジョブ開始直後に
// 出るので、正常なログでは先頭のわずかな量しか読まない。
//
// **リポジトリ名の取り出し口は優先順に 3 つ試す。**
//  1. PipelineDirectoryManager の tracking config 探索行のパス
//     （`_PipelineMapping/<owner>/<repo>/PipelineFolder.json`）。最も単純で確実。
//  2. Job message の JSON ダンプ（`Worker` カテゴリ、毎ジョブ無条件で 1 回出る）に
//     含まれる `{"k":"repository","v":"owner/repo"}`。
//  3. multi-repo checkout 時の `Update repository <owner/repo>'s path to '<path>'`。
//
// **作業ディレクトリの取り出し口は優先順に 2 つ試す。**
//  1. `Update workspace to '<path>'`（PipelineDirectoryManager）。
//  2. `  Working directory: '<path>'`（ProcessInvokerWrapper の `Starting process:` ブロック）。
//
// どちらも見つからない場合のフォールバック（`<_work>/<repo>/<repo>`）は
// WorkspaceFallback が別に持つ（下記の doc）。

// 読み取りの上限。
//
// **2 つとも必要である。** Worker ログは Job message の JSON ダンプ（ジョブの設定を
// 丸ごと含む）を 1 行で書き出すため、行 1 本が数 MB になることがある。
//
//   - workerParseLimit はファイル全体から読む量の上限。取り出し口が 1 つも無いログ
//     （ジョブが checkout 前に落ちた場合など）で際限なく読まないための歯止めである。
//     **両方の値が見つかった時点で読むのをやめる**ので、正常なログでこの上限に
//     達することはない。1 MiB では JSON ダンプの後ろに出る取り出し口（作業
//     ディレクトリ）へ届かないことがあったため広げた。
//   - workerMaxLine は 1 行の上限。bufio.Scanner の既定（64 KiB）では JSON ダンプの
//     行で読み取りが止まり、その後ろの行を一切見られない。
const (
	workerParseLimit = 8 << 20  // 8 MiB
	workerMaxLine    = 4 << 20  // 4 MiB
	workerLineBuffer = 64 << 10 // 初期バッファ。長い行だけが workerMaxLine まで伸びる
)

// JobInfo は Worker ログから読み取ったジョブの素性。読み取れなかった項目は空文字。
type JobInfo struct {
	// Repository は "owner/repo" 形式。
	Repository string
	// Workspace はジョブの作業ディレクトリ。ログから直接取れた場合のみ入る。
	//
	// multi-repo チェックアウトのフォールバック（`<_work>/<repo>/<repo>`）はここでは
	// 組み立てない。組み立てには runner の `_work` ルート（Runner.WorkDir）が要り、
	// ログの中身だけを扱うこの型の外にある値だからである（WorkspaceFallback）。
	Workspace string
}

// ParseWorker は `_diag` 配下の Worker ログ 1 件を解析し、ジョブのリポジトリ名と
// 作業ディレクトリを取り出す（Issue #68）。
//
// **パスを 1 本の文字列で受けず、ディレクトリとファイル名を分けて受ける。** 名前は
// List / LatestWorker が列挙した logs.File.Name をそのまま渡す想定で、読み出しは
// os.DirFS で dir の中に閉じる。io/fs は `..` や絶対パスを含む名前を fs.ErrInvalid で
// 弾くため、**`_diag` の外を読ませない**ことをコメントの約束ではなく型と標準ライブラリ
// の側で担保できる。1 本のパスで受けると、name 側に何が入っていても filepath が
// 素直に解決してしまい、この関数だけでは防げない。
//
// **開けない・読めない・想定した形式が無い、いずれの場合もエラーにせず空の JobInfo を
// 返す。** Jobs タブの列を埋めるための解析であり、失敗しても一覧の描画自体は
// 止まってはならない（呼び出し側は空文字を "-" に縮退する。
// internal/ui/molecule/listrow.JobRow の dashCell）。「開けなかった」と
// 「開けたがこの 2 項目が書かれていなかった（ジョブが始まったばかりでまだ出て
// いない、など）」を呼び出し側が区別する必要は無く、どちらも同じ縮退をする。
// 戻り値に error を残しているのは List / Tail など他の logs パッケージの関数と
// 型を揃えるためで、今日の実装は常に nil を返す。
func ParseWorker(dir, name string) (JobInfo, error) {
	f, err := os.DirFS(dir).Open(name)
	if err != nil {
		return JobInfo{}, nil
	}
	defer func() { _ = f.Close() }()

	var info JobInfo
	sc := bufio.NewScanner(io.LimitReader(f, workerParseLimit))
	sc.Buffer(make([]byte, 0, workerLineBuffer), workerMaxLine)
	for sc.Scan() {
		line := sc.Text()
		if info.Repository == "" {
			info.Repository = firstMatch(line, repoFromMapping, repoFromJobMessage, repoFromMultiRepo)
		}
		if info.Workspace == "" {
			info.Workspace = firstMatch(line, workspaceFromUpdate, workspaceFromWorkingDir)
		}
		if info.Repository != "" && info.Workspace != "" {
			break
		}
	}
	// 読み取りの失敗（長すぎる行・途中で切れたファイル）はそこまでで打ち切る。
	// 取れた分は返す（呼び出し側は空を `-` に縮退する）。
	return info, nil
}

// WorkspaceFallback は ParseWorker がジョブの作業ディレクトリを取れなかった場合の
// 既定値を組み立てる。
//
// actions/runner の TrackingConfig.cs は WorkspaceDirectory を `<repo>/<repo>`
// （owner を含まない）と定めている。runner の `_work` ルート（workDir）はログの
// 中身から取れないため、呼び出し側が Runner.WorkDir を渡す。repository が
// "owner/repo" の形でなければ組み立てられないので空文字を返す。
func WorkspaceFallback(workDir, repository string) string {
	_, repo, ok := strings.Cut(repository, "/")
	if !ok || repo == "" {
		return ""
	}
	return filepath.Join(workDir, repo, repo)
}

// firstMatch は extractors を優先順に試し、最初に見つかった値を返す。
// どれも一致しなければ空文字を返す。
func firstMatch(content string, extractors ...func(string) (string, bool)) string {
	for _, extract := range extractors {
		if v, ok := extract(content); ok {
			return v
		}
	}
	return ""
}

// pipelineMappingMarker は PipelineDirectoryManager が tracking config を探すときに
// 書く行に含まれる、runner の `_work` からの固定セグメント。この直後の 2 つの
// パスセグメントが owner と repo になる。
const pipelineMappingMarker = "_PipelineMapping/"

// repoFromMapping は 1) tracking config のパスから "owner/repo" を取り出す。
func repoFromMapping(content string) (string, bool) {
	i := strings.Index(content, pipelineMappingMarker)
	if i < 0 {
		return "", false
	}
	parts := strings.SplitN(content[i+len(pipelineMappingMarker):], "/", 3)
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return parts[0] + "/" + parts[1], true
}

// jobMessageRepoKey は Job message の JSON ダンプに現れる、リポジトリ名を運ぶ
// エントリのキー部分。
//
// トップレベルのプロパティ名（`t` / `d` など）の大小文字は actions/runner の
// ソースから確認できなかった（scratchpad/worker-log-format.md）。そのため JSON を
// キー名で構造的に辿るのではなく、"k":"repository" という並びとその直後に現れる
// 最初の "v":"..." を素直に拾う。
const jobMessageRepoKey = `"k":"repository"`

// repoFromJobMessage は 2) Job message の JSON ダンプから "owner/repo" を取り出す。
func repoFromJobMessage(content string) (string, bool) {
	i := strings.Index(content, jobMessageRepoKey)
	if i < 0 {
		return "", false
	}
	return extractJSONValue(content[i+len(jobMessageRepoKey):])
}

// extractJSONValue は "k":"..." の直後に続く最初の "v":"<value>" の value を返す。
func extractJSONValue(rest string) (string, bool) {
	const marker = `"v":"`
	i := strings.Index(rest, marker)
	if i < 0 {
		return "", false
	}
	rest = rest[i+len(marker):]
	end := strings.IndexByte(rest, '"')
	if end <= 0 {
		return "", false
	}
	return rest[:end], true
}

const (
	multiRepoMarker = "Update repository "
	multiRepoInfix  = "'s path to '"
)

// repoFromMultiRepo は 3) multi-repo checkout 時の行から "owner/repo" を取り出す。
func repoFromMultiRepo(content string) (string, bool) {
	i := strings.Index(content, multiRepoMarker)
	if i < 0 {
		return "", false
	}
	rest := content[i+len(multiRepoMarker):]
	j := strings.Index(rest, multiRepoInfix)
	if j <= 0 {
		return "", false
	}
	repo := rest[:j]
	if !strings.Contains(repo, "/") {
		return "", false
	}
	return repo, true
}

// workspaceUpdateMarker は PipelineDirectoryManager が作業ディレクトリを決めたときに
// 書く行の先頭部分。
const workspaceUpdateMarker = "Update workspace to '"

// workspaceFromUpdate は 1) PipelineDirectoryManager が書く行から作業ディレクトリを
// 取り出す。
func workspaceFromUpdate(content string) (string, bool) {
	return extractQuoted(content, workspaceUpdateMarker)
}

// workingDirMarker は ProcessInvokerWrapper の `Starting process:` ブロックに現れる
// 行の先頭部分。
const workingDirMarker = "Working directory: '"

// workspaceFromWorkingDir は 2) ステップ起動時のブロックから作業ディレクトリを
// 取り出す。
func workspaceFromWorkingDir(content string) (string, bool) {
	return extractQuoted(content, workingDirMarker)
}

// extractQuoted は marker の直後から次の ' までを取り出す。
func extractQuoted(content, marker string) (string, bool) {
	i := strings.Index(content, marker)
	if i < 0 {
		return "", false
	}
	rest := content[i+len(marker):]
	end := strings.IndexByte(rest, '\'')
	if end <= 0 {
		return "", false
	}
	return rest[:end], true
}
