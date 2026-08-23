{
  description = "Local file sharing with browser-capable devices";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixpkgs-unstable";
    utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      utils,
    }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
    in
    utils.lib.eachSystem systems (
      system:
      let
        pkgs = import nixpkgs { inherit system; };
        # The locked nixpkgs revision still ships the vulnerable Go 1.26.5.
        # Keep Nix builds aligned with the patched toolchain used by CI.
        go = pkgs.go_1_26.overrideAttrs (_: {
          version = "1.26.6";
          src = pkgs.fetchurl {
            url = "https://go.dev/dl/go1.26.6.src.tar.gz";
            hash = "sha256-oHIcVMaIkBRI13rZs+x+p8R0cwdV/4kTgukuy5P/LLE=";
          };
        });
        buildGoModule = pkgs.buildGoModule.override { inherit go; };
        packageVersion = "0.6.1";
        qshare = buildGoModule {
          pname = "qshare";
          version = packageVersion;

          src = self;
          vendorHash = "sha256-mPTvOPafgf7Q4f8INwqaVhMhNZRbiYIpKyTQnEaWdKo=";

          subPackages = [ "cmd/qshare" ];
          env.CGO_ENABLED = "0";
          ldflags = [ "-X main.version=v${packageVersion}" ];

          checkPhase = ''
            runHook preCheck
            go test ./...
            runHook postCheck
          '';

          meta = {
            description = "Local file sharing with browser-capable devices";
            homepage = "https://github.com/canta-9142/qshare";
            license = pkgs.lib.licenses.mit;
            mainProgram = "qshare";
            platforms = pkgs.lib.platforms.linux;
          };
        };
      in
      {
        packages.default = qshare;

        apps.default = {
          type = "app";
          program = pkgs.lib.getExe qshare;
          meta.description = "Run qshare";
        };

        checks = {
          package = qshare;
          vet = qshare.overrideAttrs (_: {
            pname = "qshare-vet";
            buildPhase = ''
              runHook preBuild
              go vet ./...
              runHook postBuild
            '';
            doCheck = false;
            installPhase = ''
              runHook preInstall
              touch "$out"
              runHook postInstall
            '';
          });
        };

        devShells.default = pkgs.mkShell {
          packages = [
            go
            pkgs.gopls
            pkgs.gotools
            pkgs.golangci-lint
            pkgs.actionlint
            pkgs.nixfmt
            pkgs.python3Packages.osc
          ];

          shellHook = ''
            export GOPATH="$PWD/.go"
            export PATH="$GOPATH/bin:$PATH"
            echo "GOPATH is set to $GOPATH"
            go version
          '';
        };

        formatter = pkgs.nixfmt;
      }
    );
}
