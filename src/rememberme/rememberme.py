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

from rememberme.git_tools import blame_lines

INLINE_REGEX = (
    r"^\s*(?:(?:#+|\/\/+|<!--|--|\/\*|\"\"\"|''')\s?)*|(?:-->|#}}|\*\/|--}}|}}|#+|#}|\"\"\"|''')*$"
)
console = Console(highlight=False)


class ParsingError(Exception):
    pass


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
        match = re.findall("(.+):([0-9]+):(.*)", line)
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


def prettify_summary(file_summary: dict[str, int], bw: bool = False) -> str:
    summary = (
        f" {boldify(emojify(tag))}: {count} " for tag, count in file_summary.items() if count > 0
    )
    if not bw:
        summary = (colorize(string, tag) for string, tag in zip(summary, file_summary))

    return Padding(Panel(" ".join(summary), expand=False), pad=(0, 0, 0, 2))


def stylize_filename(file: str, n_lines: int, style: str):
    if style == "full":
        return (
            f"\n[bold cyan]• {file}[/bold cyan] [bright_white]({n_lines} comments):[/bright_white]"
        )
    if style == "bw":
        return f"\n[bold]• {file}[/bold] ({n_lines} comments):"
    return f"\n{file}"


def tag_git_author(
    git_author: str, git_date: datetime, tag: str, age_limit: int, bw: bool = False
) -> str:
    if not git_author:
        return ""
    if git_date < datetime.now() - timedelta(days=age_limit):
        git_author = f"[☠ OLD {git_author}]"
        if bw:
            return f" [bold]{git_author}[/] "
        else:
            return f" [bold black on red]{git_author}[/] "
    return f" {colorize(f'[{git_author}]', tag)} "


def print_parsed_file(
    file: str, contents: dict, tags: list[str], regex: re.Pattern, args: argparse.Namespace
):
    print_lines = []
    tag_counter = {tag: 0 for tag in tags}
    lines = contents["lines"]
    texts = contents["texts"]
    blames = blame_lines(file, list(map(int, lines)))
    max_digits = max(len(line_n) for line_n in lines)
    filename_line = stylize_filename(file, len(lines), args.style)
    console.print(filename_line)
    for i, text in enumerate(texts):
        matches = re.search(regex, text)
        if not matches:
            raise ParsingError(f"the following line could not be parsed:\n{text}")
        groups = matches.groups()
        if len(groups) != 2:
            raise ParsingError(f"the following line could not be parsed:\n{text}")
        tag, txt = groups
        tag_counter[tag] += 1
        git_author, git_date = blames[i]
        if args.style == "plain":
            text = re.sub(INLINE_REGEX, "", txt).strip()
            git_author = ""
        elif args.style == "bw":
            text = boldify(emojify(tag)) + ": " + re.sub(INLINE_REGEX, "", txt).strip() + " "
            git_author = tag_git_author(git_author, git_date, tag, args.age_limit, bw=True)
        else:
            text = colorize(
                boldify(emojify(tag)) + ": " + re.sub(INLINE_REGEX, "", txt).strip() + " ",
                tag,
            )
            git_author = tag_git_author(git_author, git_date, tag, args.age_limit)

        grid = Table.grid(expand=False, pad_edge=True)
        grid.add_column(justify="left", width=max_digits + 8)
        grid.add_column(justify="left", width=console.width // 2 - max_digits)
        if args.author:
            grid.add_column(justify="left")
            grid.add_row(pad_line_number(lines[i], max_digits), text, git_author)
        else:
            grid.add_row(pad_line_number(lines[i], max_digits), text)
        grid = Padding(grid, (0, 0, 0, 2))
        print_lines.append(grid)
    if sum(count > 0 for count in tag_counter.values()) > 1 and args.summary:
        if args.style == "full":
            console.print(prettify_summary(tag_counter))
        elif args.style == "bw":
            console.print(prettify_summary(tag_counter, bw=True))
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
    parser.add_argument(
        "--glob",
        "-g",
        type=str,
        default=None,
        help="glob pattern to include/exclude files in the search. Must be a quoted string. Same syntax as ripgrep.",
    )
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
    REGEX = (
        "(?:^|(?:(?:#+|//+|<!--|--|/\*|\"\"\"|''')+\s*)+)\s*"
        + f"(?:^|\\b)({'|'.join(TAGS)})[\s:;-]+(.+?)"
        + "(?:$|-->|#\}\}|\*/|--\}\}|\}\}|#+|#\}|\"\"\"|''')"
    )

    # XXX requires ripgrep installation
    cmd = ["rg", REGEX, args.folder, "-n"]
    if args.glob:
        cmd.extend(["-g", args.glob])
    console.print(" ".join(cmd))
    process_out = subprocess.run(cmd, capture_output=True)
    rg_error = process_out.stderr.decode("utf-8")
    if rg_error:
        raise ParsingError(f"ripgrep search failed: {rg_error}")

    rg_output = process_out.stdout.decode("utf-8")
    by_file = parse_rg_output(rg_output)
    for file in sorted(by_file):
        print_parsed_file(file, by_file[file], TAGS, REGEX, args)


if __name__ == "__main__":
    main()
