# Copr must have network access enabled so Go modules can be downloaded during
# the build. Module contents are verified against go.sum.

%global debug_package %{nil}

Name:           qshare
Version:        0.6.2
Release:        1%{?dist}
Summary:        Local file sharing with browser-capable devices

License:        MIT
URL:            https://github.com/canta-9142/qshare
Source0:        %{url}/archive/refs/tags/v%{version}/%{name}-%{version}.tar.gz

ExclusiveArch:  x86_64 aarch64
BuildRequires:  golang >= 1.24

Suggests:       wl-clipboard
Suggests:       xclip
Suggests:       xsel

%description
qshare is a local file-sharing command-line tool. It lets a computer exchange
files and text with a smartphone or another browser-capable device without a
dedicated receiving application, cloud storage, or an account.

%prep
%autosetup

%build
export CGO_ENABLED=0
export GOTOOLCHAIN=local
go build \
    -buildmode=pie \
    -buildvcs=false \
    -mod=readonly \
    -trimpath \
    -ldflags "-X main.version=v%{version}" \
    -o qshare \
    ./cmd/qshare

%check
export CGO_ENABLED=0
export GOTOOLCHAIN=local
go test -buildvcs=false -mod=readonly ./...

%install
install -Dpm0755 qshare %{buildroot}%{_bindir}/qshare

%files
%license LICENSE
%doc README.md
%{_bindir}/qshare

%changelog
* Fri Aug 28 2026 Kanta Imai <work@floating-gate.com> - 0.6.2-1
- Release version 0.6.2

* Tue Aug 18 2026 Kanta Imai <work@floating-gate.com> - 0.6.1-1
- Release version 0.6.1

* Tue Aug 18 2026 Kanta Imai <work@floating-gate.com> - 0.5.5-1
- Add the Copr package specification
