# listme

**Keep track of your code comments with ease.**

## Features

- ⚡ **Fast & Recursive**: quickly search your codebase, even through nested directories.
- 🌈 **Tag Summaries**: get a neat summary with the count of each type of comment tag.
- 🏷 **Customize Tags**: tailor your comment tags to match your workflow.
- 🖨 **Git Integration**: view the Git author of each comment, marking old commits.

## Why `listme`?

Managing code comments can be messy, especially in larger projects. `listme` helps you stay organized by summarizing your FIXME, TODO, XXX, and other tags, so nothing slips through the cracks.

This project was inspired by [`fixme`](https://github.com/JohnPostlethwait/fixme) but brings some additional features. Since `fixme` is no longer actively maintained, `listme` provides a fresh alternative.

Originally written in Python, `listme` was later rewritten in [Go](https://go.dev/) to eliminate dependencies and boost performance.


## Installation

Ensure you have a recent version of [git](https://git-scm.com/) available in your PATH.

### Option 1 (recommended): `go install`

```bash
go install github.com/mathpn/listme@latest
```

### Option 2: pre-compiled binaries

Visit the _releases_ section to download pre-compiled binaries. Once downloaded, place the binary in a directory on your PATH.


## Getting Started

Just call listme with the folder or file you want to inspect as the first argument.

```bash
listme .
```

You'll see an output like this:

![Example output screenshot](https://github.com/mathpn/listme/raw/main/screenshots/example_output.png?raw=true)

`listme` respects your project's `.gitignore` files to exclude specific directories and files. If you need additional filtering, use the `--glob (-g)` option.

Comments from commits older than a certain age (set with `--age-limit`) are tagged as old, indicating their age along with the author's name, e.g., `[OLD John Doe]`.


### Font and terminal support

Most modern terminals support the Unicode symbols used in listme. For the best experience, we recommend using a patched font (e.g., one from **[nerd fonts](https://www.nerdfonts.com/)**).


### Options

- **path**: path to folder or file to be searched. Search is recursive.
- **--tags (-T)**: Define the tags to search for, separated by spaces. Default tags include BUG, FIXME, XXX, TODO, HACK, OPTIMIZE, and NOTE.
- **--glob (-g)**: Use a single-quoted glob pattern to filter files during the search (e.g., *.go).
- **--age-limit (-l)**: Set the age limit for commits in days. Older commits are marked. Default: 60 days
- **--max-file-size (-f)**: Maximum file size to scan (in MB). Default: 5MB
- **--full-path (-F)**: Print the full absolute path of files.
- **--no-author (-A)**: Exclude Git author information.
- **--no-summary (-S)**: Skip the summary box for each file.
- **--workers (-w)**: Specify the number of search workers (usually not necessary to change).
- **--verbose (-v)**: Enable info logging level.
- **--debug (-d)**: Enable debug verbosity.

### Style options

Choose your preferred style for output: colored (default), black-and-white (`-b`), or plain (`-p`). We recommend using the default style for the best experience.

The plain style is designed for machine consumption, using a format like `file:tag:text`. If you redirect listme's output, it will automatically switch to plain style.

## Contributing

`listme` is currently maintained by a single person. Contributions are greatly appreciated.

### Contributing code

To contribute, fork the repository, make your changes, and submit a pull request.

### Filing issues

If you encounter a bug or want to discuss an improvement, feel free to [file an issue](https://github.com/mathpn/listme/issues).
