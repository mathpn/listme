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
from rich.padding import Padding
from rich.text import Text

from git_tools import blame_lines

INLINE_REGEX = (
    "^\s*(?:(?:#+|\/\/+|<!--|--|\/\*|\"\"\"|''')\s?)*|(?:-->|#}}|\*\/|--}}|}}|#+|#}|\"\"\"|''')*$"
)


console = Console()


def boldify(string: str) -> str:
    return f"[bold]{string}[/bold]"


# XXX this is just temporary
def emojify(string: str) -> str:
    return ":no_entry: " + string


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


def prettify_summary(file_summary: dict[str, int], padding: int = 0) -> str:
    return Padding(
        " ".join(
            f"{emojify(boldify(tag))}: {count}" for tag, count in file_summary.items() if count > 0
        ),
        pad=(0, 0, 0, padding + 2),
    )


# TODO colorful + emojis
def print_parsed_output(
    by_file: dict[str, list], tags: list[str], regex: re.Pattern, args: argparse.Namespace
) -> None:
    files = sorted(by_file)
    for file in files:
        print_lines = []
        tag_counter = {tag: 0 for tag in tags}
        contents = by_file[file]
        lines = contents["lines"]
        texts = contents["texts"]
        blames = blame_lines(file, list(map(int, lines)))
        max_digits = max(len(line_n) for line_n in lines)
        print(f"\n{file} ({len(lines)} comments):")
        for i, text in enumerate(texts):
            matches = re.search(regex, text)
            if not matches:
                raise ValueError(f"something went wrong! -> {text}")  # FIXME remove this
            groups = matches.groups()
            if len(groups) != 2:
                raise ValueError(f"something went wrong! -> {text}: {groups}")  # FIXME remove this
            tag, txt = groups
            tag_counter[tag] += 1
            git_author, git_date = blames[i]
            line = (
                pad_line_number(lines[i], max_digits)
                + ": "
                + emojify(boldify(tag))
                + ": "
                + re.sub(INLINE_REGEX, "", txt).strip()  # FIXME deactivate auto-highlighting
            )
            columns = Columns([line, git_author], width=console.width // 2 - 1, expand=True)
            print_lines.append(columns)
        if args.summary:
            print(prettify_summary(tag_counter, padding=max_digits))
        for line in print_lines:
            print(line)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("folder", nargs="?", type=str, default=os.getcwd())
    parser.add_argument(
        "--tags", "-T", nargs="+", default=["TODO", "FIXME", "XXX", "NOTE", "BUG", "OPTIMIZE"]
    )
    summary_group = parser.add_mutually_exclusive_group()
    summary_group.add_argument(
        "--summary", "-s", action="store_const", dest="summary", const=True, default=True
    )
    summary_group.add_argument(
        "--no-summary", "-S", action="store_const", dest="summary", const=False
    )
    args = parser.parse_args()

    TAGS = sorted(args.tags)
    # pcre2
    REGEX = re.compile(
        "^\s*(?:(?:#+|\/\/+|<!--|--|\/\*|\"\"\"|''')\s*)*\s*"
        + f"(?:^|\\b)({'|'.join(TAGS)})[\s:;-]+(.+?)"
        + "(?=$|-->|#\}\}|\*\/|--\}\}|\}\}|#+|#\}|\"\"\"|''')"
    )
    RG_REGEX = (
        "^\s*(?:(?:#+|//+|<!--|--|/\*|\"\"\"|''')\s*)*\s*"
        + f"(?:^|\\b)({'|'.join(TAGS)})[\s:;-]+(.+?)"
        + "(?:$|-->|#\}\}|\*/|--\}\}|\}\}|#+|#\}|\"\"\"|''')"
    )

    rg_output = subprocess.check_output(["./bin/rg", RG_REGEX, args.folder, "-n"])
    # print(" ".join(["./bin/rg", RG_REGEX, args.folder, "-n"]))
    rg_output = rg_output.decode("utf-8")
    by_file = parse_rg_output(rg_output)
    print_parsed_output(by_file, TAGS, REGEX, args)


if __name__ == "__main__":
    main()
