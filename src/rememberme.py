"""
Draft of main script. This uses ripgrep on the background for speed.
"""

import argparse
import os
import re
import subprocess
from collections import defaultdict

from rich import print
from rich.columns import Columns
from rich.console import Console
from rich.text import Text

TAGS = ["TODO", "FIXME", "XXX", "NOTE", "BUG", "OPTIMIZE"]
REGEX = f"({'|'.join(TAGS)}) "
INLINE_REGEX = "^(#+|//+|<!--|--|/\*|\"\"\")|-->$"
console = Console()


def boldify(string: str) -> str:
    return re.sub(REGEX_TAGS, r"[bold]\1[/bold]", string)


# XXX this is just temporary
def emojify(string: str) -> str:
    return ":no_entry: " + string


# TODO summary per file
def parse_rg_output(output: str) -> dict[str, list]:
    lines = output.splitlines()
    by_file = defaultdict(list)
    for line in lines:
        match = re.search("(.*):(.*):(.*)", line)
        if not match:
            continue
        file, *content = match.groups()
        by_file[file].append(content)
    return by_file


# TODO colorful + emojis
def print_parsed_output(by_file: dict[str, list]) -> None:
    files = sorted(by_file)
    for file in files:
        print(f"\n{file}")
        contents = by_file[file]
        for content in contents:
            git_blame = Text("git user")
            line = (
                content[0] + ": " + emojify(boldify(re.sub(INLINE_REGEX, "", content[1]).strip()))
            )
            columns = Columns([line, git_blame], width=console.width // 2 - 1, expand=True)
            print(columns)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("folder", nargs="?", type=str, default=os.getcwd())
    args = parser.parse_args()

    # TODO better regex
    rg_output = subprocess.check_output(["./bin/rg", REGEX, args.folder, "-n"])
    rg_output = rg_output.decode("utf-8")
    by_file = parse_rg_output(rg_output)
    print_parsed_output(by_file)


if __name__ == "__main__":
    main()
