# rememberme

Summarize you FIXME, TODO, XXX (and other tags) comments so you don't forget them.

## Features

- ⚡ Blazingly fast recursive search! Thanks to [ripgrep](https://github.com/BurntSushi/ripgrep), massive codebases can be scanned very quickly.
- 🌈 A nice and pretty summary with comment counts per tag is printed for each file.
- 🏷 Tags can be customized by the user.
- 🖨 Print the git author of each message, highlighting old commits.

This project was inspired by practical needs and by [fixme](https://github.com/JohnPostlethwait/fixme). Since fixme is no longer maintained, it makes sense to have a new package (this time written in Python using ripgrep written in Rust) with some new features.

## Installation

This package requires Python 3.8+, a recent version of [git](https://git-scm.com/) and [ripgrep](https://github.com/BurntSushi/ripgrep) (13.0+). Both git and ripgrep should be available in your PATH. Then install with pip:

> pip install rememberme


## Usage

Simply call rememberme with the folder or file of interest as the first argument.

```bash
rememberme .
```

You should see an output like this:

![Example output screenshot](https://github.com/mathpn/rememberme/raw/main/screenshots/example_output.png?raw=true)

Ripgrep uses ``.gitignore`` files present in your folders to ignore certain directories and files. If you want to add filters on top of ripgrep, use the ``--glob (-g)`` option.

Comments that were commited too long ago (limit set be the ``--age-limit`` parameter) are maked as old before the author's name: ``[☠ OLD John Doe]``


### Configuration

Most terminals should support all the Unicode symbols used in this project. If not, a patched font (e.g. one of [nerd fonts](https://www.nerdfonts.com/font-downloads)) is higly recommended. Still, to ensure compatibility across many terminal emulators and fonts, a configuration wizard will pop up the first time you run rememberme asking if some symbols are rendered correctly.

To run the configuration wizard again, simply run:

> rememberme-config


### Options

- **path**: Path to folder or file to scan for comments with tags.
- **--tags (-T)**: The tags that will be searched, input should be separated by spaces. Tags are shown in the file summary in the order defined here. Default tags: BUG, FIXME, XXX, TODO, HACK, OPTIMIZE, NOTE.
- **--glob (-g)**: Glob pattern to include/exclude files or folders in the search. Must be a single-quoted string. This argument is passed to ripgrep, so check [ripgrep documentation](https://github.com/BurntSushi/ripgrep/blob/master/GUIDE.md#manual-filtering-globs) for syntax details.
- **--age-limit (-l)**: Age limit (commit age) for comments. Comments older than this limit are marked.
- **--relative-path (-r)**: Use relative paths in the output. This is the default.
- **--full-path (-R)**: Use full absolute paths in the output.
- **--author (-a)**: Print git commit author names. This is the default.
- **--no-author (-A)**: Do not print git commit author names.
- **--summary (-s)**: Print a nice file summary. This is the default.
- **--no-summary (-S)**: Do not print a nice file summary.
- **--verbose (-v)**: print files which were ignored due to parsing errors.

### Style options

The output can be printed in full (``--full``, the default), black-and-white (``-b``) or plain style (``-p``). Full style is recommended, and the only difference to black-and-white is (of course) colors.

Plain style is aimed towards machine consumption. The format is basically a filename line followed by all the comments in the format ``TAG:text``. All comment lines are indented with one tab.
