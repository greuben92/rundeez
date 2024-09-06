
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    templ-flake.url = "github:a-h/templ/?ref=refs/tags/v0.2.771";
  };

  outputs = { self, nixpkgs, templ-flake }:
    let
      forAllSystems = nixpkgs.lib.genAttrs nixpkgs.lib.systems.flakeExposed;
      pkgsFor = forAllSystems (system: import nixpkgs {
        inherit system;
        overlays = [ self.overlays.default ];
      });
      templ = system: templ-flake.packages.${system}.templ;
    in
    {
      overlays.default = final: prev: { };

      devShells = forAllSystems (system:
        let
          pkgs = pkgsFor.${system};
        in
        {
          default = pkgs.mkShell {
            buildInputs = with pkgs; [
              go
              gopls
              gotools
              go-tools
              (templ system)
            ];
          };
        });
    };
}
# vim: expandtab shiftwidth=2 tabstop=2
