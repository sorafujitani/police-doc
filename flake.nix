{
  description = "Check CLI examples in Markdown using installed executables";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

  outputs = { self, nixpkgs }:
    let
      systems = [ "aarch64-darwin" "x86_64-darwin" "aarch64-linux" "x86_64-linux" ];
    in
    {
      packages = nixpkgs.lib.genAttrs systems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          policedoc = pkgs.buildGo127Module {
            pname = "policedoc";
            version = "unstable-${self.shortRev or "dirty"}";
            src = pkgs.lib.fileset.toSource {
              root = ./.;
              fileset = pkgs.lib.fileset.unions [ ./go.mod ./go.sum ./cmd ./internal ];
            };
            vendorHash = "sha256-HRj/icLduEOwbg+KN5Dk24xljVh9WEZ7HUJxG0jih5s=";
            subPackages = [ "cmd/policedoc" ];
            env.CGO_ENABLED = "0";
            ldflags = [ "-s" "-w" "-X main.version=unstable-${self.shortRev or "dirty"}" ];

            postPatch = ''
              substituteInPlace internal/app/discover_test.go internal/spec/generic_test.go internal/spec/spec_test.go \
                --replace-fail '#!/bin/sh' '#!${pkgs.runtimeShell}'
              substituteInPlace internal/app/discover_test.go \
                --replace-fail 'exec /bin/sleep' 'exec ${pkgs.coreutils}/bin/sleep'
            '';

            checkPhase = ''
              runHook preCheck
              go test ./...
              runHook postCheck
            '';

            doInstallCheck = true;
            installCheckPhase = ''
              runHook preInstallCheck
              "$out/bin/policedoc" version
              runHook postInstallCheck
            '';

            meta = {
              description = "Check CLI examples in Markdown using installed executables";
              homepage = "https://github.com/sorafujitani/police-doc";
              license = pkgs.lib.licenses.mit;
              mainProgram = "policedoc";
              platforms = systems;
            };
          };
        in
        {
          inherit policedoc;
          default = policedoc;
        });
    };
}
