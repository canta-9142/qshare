package cli

import "time"

type arguments struct {
	LAN        bool          `arg:"--lan" help:"use an existing local network"`
	Expire     time.Duration `arg:"--expire" default:"10m" help:"session lifetime"`
	ReceiveDir string        `arg:"--receive-dir" placeholder:"DIR" help:"directory for received files"`
	Text       *string       `arg:"--text" placeholder:"TEXT" help:"share UTF-8 text"`
	Clipboard  *string       `arg:"--clipboard" placeholder:"BACKEND" help:"clipboard backend (default in receive mode: auto)"`
	Files      []string      `arg:"positional" placeholder:"FILE"`
}
