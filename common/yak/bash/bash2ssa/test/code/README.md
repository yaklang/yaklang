# Bash syntax fixtures

Unmodified third-party shell scripts used only as parser and SSA fixtures.

| File | Source | Upstream license |
| --- | --- | --- |
| `nvm.sh` | https://github.com/nvm-sh/nvm | MIT |
| `homebrew-install.sh` | https://github.com/Homebrew/install | BSD-2-Clause |
| `git-subtree.sh` | https://github.com/git/git (`contrib/subtree`) | GPL-2.0 |
| `git-completion.bash` | https://github.com/git/git (`contrib/completion`) | GPL-2.0 |
| `rustup-init.sh` | https://github.com/rust-lang/rustup | MIT OR Apache-2.0 |
| `get-docker.sh` | https://get.docker.com | Apache-2.0 |
| `ohmyzsh-install.sh` | https://github.com/ohmyzsh/ohmyzsh | MIT |
| `bash-preexec.sh` | https://github.com/rcaloras/bash-preexec | MIT |
| `pyenv-installer.sh` | https://github.com/pyenv/pyenv-installer | MIT |
| `sdkman-init.sh` | https://github.com/sdkman/sdkman-cli | Apache-2.0 |

`git-completion.bash` and `sdkman-init.sh` contain zsh-only parameter
expansions (`${(M)...}` and `${(s:,:)...}`) in runtime-guarded branches.
bash itself accepts the files, but mvdan's bash parser rejects them
statically, so the fixture test asserts a parser error instead of a panic.