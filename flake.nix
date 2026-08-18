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
        qshare = pkgs.buildGoModule {
          pname = "qshare";
          version = "0.5.0";

          src = self;
          vendorHash = "sha256-mPTvOPafgf7Q4f8INwqaVhMhNZRbiYIpKyTQnEaWdKo=";

          subPackages = [ "cmd/qshare" ];
          env.CGO_ENABLED = "0";

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
            pkgs.go
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
