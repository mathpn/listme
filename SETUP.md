# Setup

This file provides simple instructions to set up a working Go environment. Follow these steps to install and configure Go.

## Install Go

Install Go if it's not already installed. You can download manually from [the website](https://go.dev/doc/install) and follow the instructions there or install using a package manager (recommended). If you're on Linux, your distribution probably has a Go package easily available. Some examples are provided below.

- Fedora

```bash
sudo dnf install golang
```

- Arch

```bash
sudo pacman -S go
```

- Ubuntu

```bash
sudo apt install golang
```

You can check if the installation is successful with `go version`.


## Update your PATH variable

If you have Go installed, you may have already configured GOPATH and may skip this step.

- Unix-like systems (Linux and macOS)

  Since Go 1.8, Go will use a default value of `$HOME/go/` for the GOPATH environment variable. You can choose another path as well. Then, simply add the lines to your shell profile (e.g. `~/.bashrc` or `~/.zshrc`):

  ```bash
  export GOPATH=$HOME/go
  export PATH=$PATH:$GOPATH/bin
  ```

  You'll need to source your shell profile or restart the terminal window.

- Windows

  Run the following commands at the command prompt:

  ```
  setx GOPATH %USERPROFILE%\go
  setx path "%path%;%GOPATH%\bin"
  ```

  Close your current command prompt and open a new one.

