pkgname=qshare
pkgver=0.6.1
pkgrel=1
pkgdesc='Local file sharing with browser-capable devices'
arch=('x86_64')
url='https://github.com/canta-9142/qshare'
license=('MIT')
makedepends=('go>=1.24')
optdepends=(
  'wl-clipboard: clipboard integration on Wayland'
  'xclip: clipboard integration on X11'
  'xsel: alternative clipboard integration on X11'
)
source=(
  "${pkgname}-${pkgver}.tar.gz"
  'vendor.tar.gz'
)
noextract=('vendor.tar.gz')
sha256sums=('SKIP'
            'SKIP')

prepare() {
  tar -xzf "$srcdir/vendor.tar.gz" -C "$srcdir/$pkgname-$pkgver"
}

build() {
  cd "$srcdir/$pkgname-$pkgver"

  export CGO_ENABLED=0
  export GOPROXY=off
  export GOSUMDB=off
  export GOTOOLCHAIN=local

  go build \
    -buildmode=pie \
    -buildvcs=false \
    -mod=vendor \
    -trimpath \
    -ldflags "-X main.version=v${pkgver}" \
    -o qshare \
    ./cmd/qshare
}

check() {
  cd "$srcdir/$pkgname-$pkgver"

  export CGO_ENABLED=0
  export GOPROXY=off
  export GOSUMDB=off
  export GOTOOLCHAIN=local

  go test -mod=vendor ./...
}

package() {
  cd "$srcdir/$pkgname-$pkgver"

  install -Dm755 qshare "$pkgdir/usr/bin/qshare"
  install -Dm644 LICENSE "$pkgdir/usr/share/licenses/qshare/LICENSE"
  install -Dm644 README.md "$pkgdir/usr/share/doc/qshare/README.md"
}
