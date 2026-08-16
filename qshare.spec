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
ExclusiveArch:  x86_64 aarch64
%if 0%{?suse_version}
BuildRequires:  golang(API) >= 1.25
BuildRequires:  go >= 1.25
%else
BuildRequires:  golang >= 1.25
%endif

%description
qshare is a local file-sharing command-line tool. It lets a computer exchange
files and text with a smartphone or another browser-capable device without a
dedicated receiving application, cloud storage, or an account.

%prep
%autosetup -T -c -n %{name}-%{version}
cp -a %{_sourcedir}/cmd .
cp -a %{_sourcedir}/internal .
cp -a %{_sourcedir}/go.mod %{_sourcedir}/go.sum .
cp -a %{_sourcedir}/LICENSE %{_sourcedir}/README.md .

%build
export CGO_ENABLED=0
export GOPROXY=file://%{_sourcedir}/build-gomodcache
export GOSUMDB=off
export GOTOOLCHAIN=local
go build -mod=readonly -trimpath -o qshare ./cmd/qshare

%check
export CGO_ENABLED=0
export GOPROXY=file://%{_sourcedir}/build-gomodcache
export GOSUMDB=off
export GOTOOLCHAIN=local
go test -mod=readonly ./...

%install
install -D -m 0755 qshare %{buildroot}%{_bindir}/qshare

%files
%license LICENSE
%doc README.md
%{_bindir}/qshare

%changelog
