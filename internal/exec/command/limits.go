package command

// 上限バイト数の方針。**どこに何バイトの上限を置くかはこのパッケージが決め、
// 収める仕組みは limit パッケージが持つ**（判断は docs/ui/atomic-design.md の
// 「`internal/exec/command` から切り詰めの道具を `limit` へ切り出した判断」）。
const (
	// maxRecordedErrorBytes は監査レコードの error に載せる上限。
	//
	// 監査ログは 1 レコード 1 行の JSONL なので、上限を置かないと 1 回の失敗が
	// 数 MB の 1 行になり、ローテーションも grep も破綻する。
	maxRecordedErrorBytes = 4 << 10 // 4 KiB

	// maxStderrExcerptBytes は ExitError.Stderr に残す上限。
	// 画面に出す抜粋なので、原因の書かれた末尾だけあれば足りる。
	maxStderrExcerptBytes = 4 << 10 // 4 KiB

	// maxStderrCaptureBytes は標準エラー出力を取り込む上限。
	//
	// 暴走した子が標準エラー出力を吐き続けてもメモリを食い潰さないための保険。
	// 上限を超えたときに残るのは末尾（limit.Buffer が古い先頭を捨てる）で、
	// 抜粋を作る limit.Head と向きを揃えてある。
	// 標準出力に同じ上限を置かないのは、呼び出し側が stdout を解析する
	// （systemctl show / list-units）ため、切ると解析が黙って壊れるからである。
	maxStderrCaptureBytes = 1 << 20 // 1 MiB
)
