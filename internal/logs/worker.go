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

// workerParseLimit は ParseWorker が読む最大バイト数。
//
// Worker ログは Job message の JSON ダンプ（ジョブの設定を丸ごと含む）を **1 行**で
// 書き出すため、行 1 本が数 MB になることがある。上限を置くのは、取り出し口が 1 つも
// 無いログ（ジョブが checkout 前に落ちた場合など）で際限なく読まないための歯止めで
// ある。**最優先の取り出し口が両方そろった時点で読むのをやめる**ので、正常なログで
// この上限に達することはない。
//
// bufio.Scanner ではなく bufio.Reader を使うのは、Scanner が 1 行の上限を超えると
// bufio.ErrTooLong で**その行以降を一切読まなくなる**ためである。JSON ダンプが上限を
// 超えた瞬間に、その後ろに出る取り出し口へ届かなくなる（この関数の主目的そのものが
// 静かに失われる）。Reader なら行の長さに関わらず読み進められ、使う量は
// workerParseLimit で頭打ちになる。
const workerParseLimit = 8 << 20 // 8 MiB

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

	var repo, work ranked
	r := bufio.NewReader(io.LimitReader(f, workerParseLimit))
	for {
		line, rerr := r.ReadString('\n')
		repo = pick(repo, line, repoFromMapping, repoFromJobMessage, repoFromMultiRepo)
		work = pick(work, line, workspaceFromUpdate, workspaceFromWorkingDir)
		// **最優先の取り出し口がそろったときだけ打ち切る。** 低い優先度で埋まった値は
		// 後の行に出る高い優先度の一致で上書きされうるので、そこで止めてはならない。
		if repo.rank == bestRank && work.rank == bestRank {
			break
		}
		if rerr != nil {
			// EOF・上限到達・読み取り失敗。取れた分を返す（呼び出し側は空を `-` に縮退）。
			break
		}
	}
	return JobInfo{Repository: repo.value, Workspace: work.value}, nil
}

// bestRank は最も優先度の高い取り出し口の順位。
const bestRank = 1

// ranked は取り出した値と、それを取った取り出し口の優先順位（1 が最優先）。
//
// **順位を値と一緒に持つのが要点である。** 抽出は 1 行ずつ行う（行境界を越えないため）
// ので、優先順は同じ行の中でしか自然には効かない。順位を覚えずに「先に見つかった方を
// 採る」と、行をまたいだ瞬間に**文書順が優先順に勝つ**。たとえば multi-repo
// チェックアウトでは副リポジトリの `Update repository ...`（順位 3）が
// `_PipelineMapping`（順位 1）より前に出ることがあり、副リポジトリ名が REPOSITORY 列に
// 出る。作業ディレクトリ側はさらに起きやすく、チェックアウト前のプロセス起動が書く
// `Working directory:`（順位 2）が `Update workspace to`（順位 1）より前に出ると、
// ジョブの作業ディレクトリではないパスが確定値として `_work` 列に出る。
type ranked struct {
	value string
	// rank は 0 なら未取得。1 が最優先で、数字が大きいほど優先度が低い。
	rank int
}

// pick は 1 行に対して抽出器を優先順に試し、**今より優先度の高い一致だけ**で更新する。
//
// 既に順位 n で埋まっているなら、順位 n 以降の抽出器は試さない（結果が変わらない）。
func pick(cur ranked, line string, extractors ...func(string) (string, bool)) ranked {
	for i, extract := range extractors {
		rank := i + 1
		if cur.rank != 0 && cur.rank <= rank {
			break
		}
		if v, ok := extract(line); ok {
			return ranked{value: v, rank: rank}
		}
	}
	return cur
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
