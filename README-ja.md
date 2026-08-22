<div align="center">
  <img width="216" height="216" alt="QR_426172" src="https://github.com/user-attachments/assets/0d5315b5-696f-43ad-9dbb-2b03894bb7f7" />
  <h1 align="center">Qshare - インターネット不要なファイル共有ツール</h1>
</div>

[![GitHub release](https://img.shields.io/github/v/release/canta-9142/qshare?logo=github)](https://github.com/canta-9142/qshare/releases)
[![Go](https://img.shields.io/badge/Go-1.26-blue.svg?logo=go)](https://golang.org)
[![Fedora Copr](https://img.shields.io/badge/fedora-copr-blue.svg?logo=fedora)](https://copr.fedorainfracloud.org/coprs/canta-9142/qshare)
[![Nix Flake](https://img.shields.io/badge/nix-flake-5277C3?logo=nixos&logoColor=white)](flake.nix)
[![CI](https://github.com/canta-9142/qshare/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/canta-9142/qshare/actions/workflows/ci.yml)
[![Contributions welcome](https://img.shields.io/badge/contributions-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![GitHub stars](https://img.shields.io/github/stars/canta-9142/qshare.svg?style=social&label=Stars)](https://github.com/canta-9142/qshare/stargazers)

[ [English](README.md) | 日本語版 ]

Qshareは、QRコードを使用したPC↔スマートフォン間のファイル共有ツールです。

必要とする条件は、

> - 同じLAN上にPCとスマートフォンが接続されていること（テザリングなどでも可）
> - PC上でQshareを実行すること

この2つだけです。

スマートフォン側はアプリのインストールすら必要なく、ブラウザだけで完結します。

## 使用手順

<div align="center">
  <img width="400" alt="Screenshot_20260823-042838" src="https://github.com/user-attachments/assets/46a5f6c8-d77b-4f70-9a13-4abc92ba2a2b" />
</div>

対応するコマンドを入力して、表示されるQRコードをスマートフォンで読み取ってください。

各オプションの詳細については[ドキュメント](docs/cli.md)を参照してください。

### PC→スマートフォンへのファイル転送

```sh
qshare FILE...
```

`FILE`は、転送したいファイルのパスです。

複数ファイルまたは単一ディレクトリが指定可能で、その場合はすべてのファイルをまとめてZIP圧縮で転送することもできます。

### PC→スマートフォンへのテキスト転送

オプションから入力する場合

```sh
qshare --text "Hello, World!"
```

パイプにより入力する場合

```sh
printf "Hello, World!" | qshare
```

なおテキストとファイルの同時転送はv0.6現在使用できません。v0.7での対応を予定しています。

### スマートフォン→PCへの転送（ファイル・テキスト共通）

通常はオプション無しで起動すれば問題ありません。

```sh
qshare
```

`--clipboard auto`オプションが自動で選択されているため、スマートフォン側でコピーした内容は対応済みクリップボードアプリが存在すればPC側のクリップボードに転送されます。

## インストール

### Fedora COPR

QshareはFedora COPRにてパッケージ化されています。

```sh
sudo dnf copr enable canta-9142/qshare
sudo dnf install qshare
```

対応バージョンは[Coprリポジトリ](https://copr.fedorainfracloud.org/coprs/canta-9142/qshare/)を参照してください。

### Arch Linux

現在はAURからのインストールはできません。(2026年8月現在)

AURの新規アカウント登録が開放され次第の対応となります。

現時点では[その他のLinuxディストリビューション](#その他のlinuxディストリビューション)の手順でインストールを行うことができます。

### NixOS

QshareはNix Flakeに対応しています。

単に以下を実行するか、`flake.nix`の`inputs`に追加してください。

```sh
nix run github:canta-9142/qshare
```

### その他のLinuxディストリビューション

[GitHubのReleasesページ](https://github.com/canta-9142/qshare/releases)の最新のリリースから実行バイナリが入手できます。

[Open Build Service](https://build.opensuse.org/package/show/home:canta-9142/qshare)にて、Debian・Raspbian・Arch向けのパッケージが提供されております。

ソースコードからビルドを行うこともできます。

```sh
go build ./cmd/qshare
```

また近日中に、Linux向けのインストールスクリプトを提供する予定です。

### その他のOS (Windows・macOS) について

現時点ではWindows・macOS向けのパッケージは提供されておりません。

対応を予定しておりますが、具体的な日時は未定です。ご了承ください。

## 開発・貢献

QshareはGo言語で開発されているオープンソースプロジェクトです。

みなさまからの貢献を歓迎します。

[development.md](docs/development.md)および[CONTRIBUTING.md](CONTRIBUTING.md)を参照してください。

## ライセンス

[LICENSE](LICENSE)を参照してください。

## 作者

- [GitHub](https://github.com/canta-9142)
- [ブログ](https://floating-gate.com)

---

QRコードは株式会社デンソーウェーブの登録商標です。
