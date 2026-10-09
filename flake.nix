{
  description = "Local file sharing with browser-capable devices";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixpkgs-unstable";
  };

  outputs =
    {
      self,
      nixpkgs,
    }:
    let
      forAllSystems = nixpkgs.lib.genAttrs [
        "x86_64-linux"
        "aarch64-linux"
      ];
      pkgsFor = system: nixpkgs.legacyPackages.${system};
      packageVersion = "0.6.3";
      mkPackage =
        pkgs:
        pkgs.buildGoLatestModule {
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
      packages = forAllSystems (system: {
        default = mkPackage (pkgsFor system);
      });

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = nixpkgs.lib.getExe self.packages.${system}.default;
          meta.description = "Run qshare";
        };
      });

      checks = forAllSystems (
        system:
        let
          package = self.packages.${system}.default;
        in
        {
          inherit package;
          vet = package.overrideAttrs (_: {
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
        }
      );

      devShells = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
        in
        {
          default = pkgs.mkShell {
            packages = [
              pkgs.go_latest
              pkgs.gopls
              pkgs.gotools
              pkgs.golangci-lint
              pkgs.actionlint
              pkgs.nixfmt
            ];

            shellHook = ''
              export GOPATH="$PWD/.go"
              export PATH="$GOPATH/bin:$PATH"
              echo "GOPATH is set to $GOPATH"
              go version
            '';
          };
        }
      );

      formatter = forAllSystems (system: (pkgsFor system).nixfmt);
    };
}
