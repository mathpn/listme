"""
Draft of main script. This uses ripgrep on the background for speed.
"""

import argparse
from datetime import datetime, timedelta
import os
import re
import subprocess

from rich.console import Console
from rich.padding import Padding
from rich.panel import Panel
from rich.table import Table

from git_tools import blame_lines

INLINE_REGEX = (
    r"^\s*(?:(?:#+|\/\/+|<!--|--|\/\*|\"\"\"|''')\s?)*|(?:-->|#}}|\*\/|--}}|}}|#+|#}|\"\"\"|''')*$"
)
console = Console(highlight=False)


def boldify(string: str) -> str:
    return f"[bold]{string}[/bold]"


def colorize(string: str, tag: str) -> str:
    if tag == "TODO":
        return f"[magenta]{string}[/magenta]"
    if tag == "XXX":
        return f"[black on yellow]{string}[/black on yellow]"
    if tag == "FIXME":
        return f"[red]{string}[/red]"
    if tag == "OPTIMIZE":
        return f"[blue]{string}[/blue]"
    if tag == "BUG":
        return f"[white on red]{string}[/white on red]"
    if tag == "NOTE":
        return f"[green]{string}[/green]"
    if tag == "HACK":
        return f"[yellow]{string}[/yellow]"
    return string


def emojify(tag: str) -> str:
    if tag == "TODO":
        return "✓ TODO"
    if tag == "XXX":
        return "✘ XXX"
    if tag == "FIXME":
        return "⚠ FIXME"
    if tag == "OPTIMIZE":
        return " OPTIMIZE"
    if tag == "BUG":
        return "☢ BUG"
    if tag == "NOTE":
        return "✐ NOTE"
    if tag == "HACK":
        return "✄ HACK"
    return "⚠ " + tag


# FIXME when 1 file is provided, the parsing doesn't work due to missing filename
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
    return "\[Line " + " " * (max_digits - len(number)) + number + "] "
    # return " " * (max_digits - len(number)) + number


def prettify_summary(file_summary: dict[str, int]) -> str:
    return Padding(
        Panel(
            " ".join(
                colorize(f" {boldify(emojify(tag))}: {count} ", tag)
                for tag, count in file_summary.items()
                if count > 0
            ),
            expand=False,
        ),
        pad=(0, 0, 0, 2),
    )


def prettify_summary_bw(file_summary: dict[str, int]) -> str:
    return Padding(
        Panel(
            " ".join(
                f" {boldify(emojify(tag))}: {count} "
                for tag, count in file_summary.items()
                if count > 0
            ),
            expand=False,
        ),
        pad=(0, 0, 0, 2),
    )


def stylize_filename(file: str, n_lines: int, style: str):
    if style == "full":
        return (
            f"\n[bold cyan]• {file}[/bold cyan] [bright_white]({n_lines} comments):[/bright_white]"
        )
    if style == "bw":
        return f"\n[bold]• {file}[/bold] ({n_lines} comments):"  # XXX same
    return f"\n{file}"


def tag_git_author(git_author: str, git_date: datetime, tag: str, age_limit: int, bw: bool = False) -> str:
    if git_date < datetime.now() - timedelta(days=age_limit):
        git_author = f"☠ OLD {git_author}"
        if bw:
            return f"[bold] {git_author} [/]"
        else:
            return f"[bold black on red] {git_author} [/]"
    return colorize(git_author, tag)


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
        filename_line = stylize_filename(file, len(lines), args.style)
        console.print(filename_line)
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
            if args.style == "plain":
                line = re.sub(INLINE_REGEX, "", txt).strip()
                git_author = ""
            elif args.style == "bw":
                text = (
                    " " + boldify(emojify(tag)) + ": " + re.sub(INLINE_REGEX, "", txt).strip() + " "
                )
                git_author = tag_git_author(git_author, git_date, args.age_limit, bw=True)
            else:
                text = colorize(
                    " "
                    + boldify(emojify(tag))
                    + ": "
                    + re.sub(INLINE_REGEX, "", txt).strip()
                    + " ",
                    tag,
                )
                git_author = tag_git_author(git_author, git_date, tag, args.age_limit)

            if args.author:
                grid = Table.grid(expand=False, pad_edge=True)
                grid.add_column(justify="left", width=max_digits + 8)
                grid.add_column(justify="left", width=console.width // 2 - max_digits)
                grid.add_column(justify="left")
                grid.add_row(pad_line_number(lines[i], max_digits), text, git_author)
                grid = Padding(grid, (0, 0, 0, 2))
            else:
                grid = line
            print_lines.append(grid)
        if len(lines) >= args.min_summary_count and args.summary:
            if args.style == "full":
                console.print(prettify_summary(tag_counter))
            elif args.style == "bw":
                console.print(prettify_summary_bw(tag_counter))
        for line in print_lines:
            console.print(line)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("folder", nargs="?", type=str, default=os.getcwd())
    parser.add_argument(
        "--tags",
        "-T",
        nargs="+",
        default=["BUG", "FIXME", "XXX", "TODO", "HACK", "OPTIMIZE", "NOTE"],
    )
    parser.add_argument("--min-summary-count", type=int, default=3)
    parser.add_argument(
        "--age-limit",
        "-l",
        type=int,
        default=60,
        help="Age limit for comments. Comments older than this limit are marked.",
    )
    author_group = parser.add_mutually_exclusive_group()
    author_group.add_argument(
        "--author", "-a", action="store_const", dest="author", const=True, default=True
    )
    author_group.add_argument("--no-author", "-A", action="store_const", dest="author", const=False)
    summary_group = parser.add_mutually_exclusive_group()
    summary_group.add_argument(
        "--summary", "-s", action="store_const", dest="summary", const=True, default=True
    )
    summary_group.add_argument(
        "--no-summary", "-S", action="store_const", dest="summary", const=False
    )
    style_group = parser.add_mutually_exclusive_group()
    style_group.add_argument(
        "--full", action="store_const", dest="style", const="full", default="full"
    )
    style_group.add_argument("--bw", "-b", action="store_const", dest="style", const="bw")
    style_group.add_argument("--plain", "-p", action="store_const", dest="style", const="plain")
    args = parser.parse_args()

    TAGS = args.tags
    # pcre2
    REGEX = re.compile(
        "^.*(?:(?:#+|\/\/+|<!--|--|\/\*|\"\"\"|''')\s*)*\s*"
        + f"(?:^|\\b)({'|'.join(TAGS)})[\s:;-]+(.+?)"
        + "(?=$|-->|#\}\}|\*\/|--\}\}|\}\}|#+|#\}|\"\"\"|''')"
    )
    RG_REGEX = (
        "^.*(?:(?:#+|//+|<!--|--|/\*|\"\"\"|''')\s*)*\s*"
        + f"(?:^|\\b)({'|'.join(TAGS)})[\s:;-]+(.+?)"
        + "(?:$|-->|#\}\}|\*/|--\}\}|\}\}|#+|#\}|\"\"\"|''')"
    )

    #console.print(" ".join(["./bin/rg", RG_REGEX, args.folder, "-n"]))
    rg_output = subprocess.check_output(["./bin/rg", RG_REGEX, args.folder, "-n"])
    rg_output = rg_output.decode("utf-8")
    by_file = parse_rg_output(rg_output)
    print_parsed_output(by_file, TAGS, REGEX, args)


if __name__ == "__main__":
    main()
