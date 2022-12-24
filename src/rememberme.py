"""
Draft of main script. This uses ripgrep on the background for speed.
"""

import argparse
import os
import re
import subprocess

from rich import print
from rich.columns import Columns
from rich.console import Console
from rich.text import Text

from git_tools import blame_lines

TAGS = ["TODO", "FIXME", "XXX", "NOTE", "BUG", "OPTIMIZE"]
# pcre2
REGEX = (
    "^\s*(?:(?:#+|\/\/+|<!--|--|\/\*|\"\"\"|''')\s*)*\s*"
    + f"(?:^|\\b)({'|'.join(TAGS)})[\s:;-]+(.+?)"
    + "(?=$|-->|#\}\}|\*\/|--\}\}|\}\}|#+|#\}|\"\"\"|''')"
)
RG_REGEX = (
    "^\s*(?:(?:#+|//+|<!--|--|/\*|\"\"\"|''')\s*)*\s*"
    + f"(?:^|\\b)({'|'.join(TAGS)})[\s:;-]+(.+?)"
    + "(?:$|-->|#\}\}|\*/|--\}\}|\}\}|#+|#\}|\"\"\"|''')"
)
# print(REGEX)
# REGEX = f"({'|'.join(TAGS)}) (.*)"
REGEX_TAGS = f"({'|'.join(TAGS)})"
INLINE_REGEX = (
    "^\s*(?:(?:#+|\/\/+|<!--|--|\/\*|\"\"\"|''')\s?)*|(?:-->|#}}|\*\/|--}}|}}|#+|#}|\"\"\"|''')*$"
)


console = Console()


def boldify(string: str) -> str:
    return re.sub(REGEX_TAGS, r"[bold]\1[/bold]", string)


# XXX this is just temporary
def emojify(string: str) -> str:
    return ":no_entry: " + string


# TODO summary per file
def parse_rg_output(output: str) -> dict[str, list]:
    lines = output.splitlines()
    by_file = {}
    for line in lines:
        match = re.findall("(.*):([0-9]*):(.*)", line)
        if not match:
            continue
        if len(match[0]) != 3:
            raise RuntimeError(f"something went wrong: {line} -> {match}")
        file, line, text = match[0]
        by_file.setdefault(file, {})
        by_file[file].setdefault("lines", [])
        by_file[file].setdefault("texts", [])
        by_file[file]["lines"].append(line)
        by_file[file]["texts"].append(text)
    return by_file


def pad_line_number(number: str, max_digits: int) -> str:
    return number + " " * (max_digits - len(number))


# TODO colorful + emojis
def print_parsed_output(by_file: dict[str, list]) -> None:
    files = sorted(by_file)
    for file in files:
        print(f"\n{file}")
        contents = by_file[file]
        lines = contents["lines"]
        texts = contents["texts"]
        blames = blame_lines(file, list(map(int, lines)))
        max_digits = max(len(line_n) for line_n in lines)
        for i, text in enumerate(texts):
            matches = re.search(REGEX, text)
            if not matches:
                raise ValueError(f"something went wrong! -> {text}")  # FIXME remove this
            groups = matches.groups()
            if len(groups) != 2:
                raise ValueError(f"something went wrong! -> {text}: {groups}")  # FIXME remove this
            tag, txt = groups
            git_author, git_date = blames[i]
            line = (
                pad_line_number(lines[i], max_digits)
                + ": "
                + emojify(boldify(tag))
                + ": "
                + re.sub(INLINE_REGEX, "", txt).strip()  # FIXME deactivate auto-highlighting
            )
            columns = Columns([line, git_author], width=console.width // 2 - 1, expand=True)
            print(columns)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("folder", nargs="?", type=str, default=os.getcwd())
    args = parser.parse_args()

    print(" ".join(["./bin/rg", REGEX, args.folder, "-n", "--pcre2"]))
    rg_output = subprocess.check_output(["./bin/rg", RG_REGEX, args.folder, "-n"])
    #rg_output = subprocess.check_output(["./bin/rg", REGEX, args.folder, "-n", "--pcre2"])
    rg_output = rg_output.decode("utf-8")
    by_file = parse_rg_output(rg_output)
    print_parsed_output(by_file)


if __name__ == "__main__":
    main()
