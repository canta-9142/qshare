#
# spec file for package qshare
#

%if 0%{?fedora}
%global debug_package %{nil}
%endif

Name:           qshare
Version:        0.5.0
Release:        0
Summary:        Local file sharing with browser-capable devices
License:        MIT
URL:            https://github.com/canta-9142/qshare
Source0:        %{name}-%{version}.tar.gz
Source1:        vendor.tar.gz
ExclusiveArch:  x86_64 aarch64
%if 0%{?suse_version}
BuildRequires:  golang(API) >= 1.24
BuildRequires:  go >= 1.24
%else
BuildRequires:  golang >= 1.24
%endif

%description
qshare is a local file-sharing command-line tool. It lets a computer exchange
files and text with a smartphone or another browser-capable device without a
dedicated receiving application, cloud storage, or an account.

%prep
%autosetup -a 1

%build
export CGO_ENABLED=0
export GOPROXY=off
export GOSUMDB=off
export GOTOOLCHAIN=local
go build -buildvcs=false -mod=vendor -trimpath -o qshare ./cmd/qshare

%check
export CGO_ENABLED=0
export GOPROXY=off
export GOSUMDB=off
export GOTOOLCHAIN=local
go test -mod=vendor ./...

%install
install -D -m 0755 qshare %{buildroot}%{_bindir}/qshare

%files
%license LICENSE
%doc README.md
%{_bindir}/qshare

%changelog
