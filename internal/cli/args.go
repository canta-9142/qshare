package cli

import "time"

type arguments struct {
	LAN    bool          `arg:"--lan" help:"use an existing local network"`
	Expire time.Duration `arg:"--expire" default:"10m" help:"session lifetime"`
	Files  []string      `arg:"positional" placeholder:"FILE"`
}
