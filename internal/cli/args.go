package cli

import "time"

type arguments struct {
	LAN        bool          `arg:"--lan,-l" help:"use an existing local network"`
	Expire     time.Duration `arg:"--expire,-e" default:"10m" help:"session lifetime"`
	ReceiveDir string        `arg:"--receive-dir,-r" placeholder:"DIR" help:"directory for received files"`
	Text       *string       `arg:"--text,-t" placeholder:"TEXT" help:"share UTF-8 text"`
	Clipboard  *string       `arg:"--clipboard,-c" placeholder:"BACKEND" help:"clipboard backend (default in receive mode: auto)"`
	Version    bool          `arg:"--version" help:"print version and exit"`
	Files      []string      `arg:"positional" placeholder:"FILE"`
}
