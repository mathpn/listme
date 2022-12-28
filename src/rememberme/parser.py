"""
Parser module with core rememberme functionality.
The main function is the entry-point for running rememberme.
"""

import argparse
from datetime import datetime, timedelta
import os
import re
import subprocess
from typing import Dict, List

from rich.console import Console
from rich.padding import Padding
from rich.panel import Panel
from rich.table import Table

from rememberme.git_tools import blame_lines

INLINE_REGEX = (
    r"^\s*(?:(?:#+|\/\/+|<!--|--|\/\*|\"\"\"|''')\s?)*|(?:-->|#}}|\*\/|--}}|}}|#+|#}|\"\"\"|''')*$"
)
CONSOLE = Console(highlight=False)


class ParsingError(Exception):
    """Parsing error exception."""


def boldify(string: str) -> str:
    """Turn text to bold format."""
    return f"[bold]{string}[/bold]"


def colorize(string: str, tag: str) -> str:
    """Colorize text based on tag."""
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
    """Prepend a unicode symbol to tags."""
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


def parse_rg_output_folder(output: str) -> Dict[str, Dict[str, List]]:
    """Parse ripgrep output of folder search."""
    lines = output.splitlines()
    by_file: dict[str, dict[str, list]] = {}
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


def parse_rg_output_file(output: str) -> Dict[str, List]:
    """Parse ripgrep output of file search."""
    out: dict[str, list] = {}
    lines = output.splitlines()
    for line in lines:
        match = re.findall("([0-9]+):(.*)", line)
        if not match:
            continue
        if len(match[0]) != 2:
            raise ParsingError(
                f"ripgrep output could not be parsed: {line} -> tags_regex match = {match}"
            )
        line, text = match[0]
        out.setdefault("lines", [])
        out.setdefault("texts", [])
        out["lines"].append(line)
        out["texts"].append(text)
    return out


def pad_line_number(number: str, max_digits: int) -> str:
    """Pad line number with spaces."""
    return "\[Line " + " " * (max_digits - len(number)) + number + "] "


def prettify_summary(file_summary: Dict[str, int], bw: bool = False) -> Padding:
    """Add rich text formatting to file summary."""
    file_summary = [(tag, count) for tag, count in file_summary.items() if count > 0]
    summary = (f" {boldify(emojify(tag))}: {count} " for tag, count in file_summary)
    if not bw:
        summary = (colorize(string, tag) for string, (tag, _) in zip(summary, file_summary))

    return Padding(Panel(" ".join(summary), expand=False), pad=(0, 0, 0, 2))


def stylize_filename(file: str, n_lines: int, style: str) -> str:
    """Add rich text formatting to filename."""
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
    """Colorize and tag git author if the entry is too old."""
    if not git_author:
        return ""
    if git_date < datetime.now() - timedelta(days=age_limit):
        git_author = f"[☠ OLD {git_author}]"
        if bw:
            return f" [bold]{git_author}[/] "
        return f" [bold black on red]{git_author}[/] "
    return f" {colorize(f'[{git_author}]', tag)} "


def print_parsed_file(file: str, contents: Dict, tags_regex: str, args: argparse.Namespace) -> None:
    """Print a parsed search output with rich text formatting."""
    if not contents:
        return

    print_lines = []
    tag_counter = {tag: 0 for tag in args.tags}
    lines = contents["lines"]
    texts = contents["texts"]

    blames = blame_lines(file, list(map(int, lines)))
    max_digits = max(len(line_n) for line_n in lines)
    file = shorten_filepath(file, args.path, args.path_type)
    filename_line = stylize_filename(file, len(lines), args.style)
    CONSOLE.print(filename_line)

    for i, text in enumerate(texts):
        matches = re.search(tags_regex, text)
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
        grid.add_column(justify="left", width=CONSOLE.width // 2 - max_digits)
        if args.author:
            grid.add_column(justify="left")
            grid.add_row(pad_line_number(lines[i], max_digits), text, git_author)
        else:
            grid.add_row(pad_line_number(lines[i], max_digits), text)
        padded_grid = Padding(grid, (0, 0, 0, 2))
        print_lines.append(padded_grid)

    if sum(count > 0 for count in tag_counter.values()) > 1 and args.summary:
        if args.style == "full":
            CONSOLE.print(prettify_summary(tag_counter))
        elif args.style == "bw":
            CONSOLE.print(prettify_summary(tag_counter, bw=True))

    for line in print_lines:
        CONSOLE.print(line)


def shorten_filepath(file_path: str, search_path: str, path_type: str) -> str:
    """Shorten filepath depending on the chosen option."""
    if path_type == "relative":
        rel_path = file_path.replace(search_path, "").strip("/")
        rel_path = rel_path if rel_path else os.path.basename(file_path)  # file search
        return rel_path
    return file_path


def main():
    """Run Rememberme."""
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "path",
        nargs="?",
        type=str,
        default=os.getcwd(),
        help="path to folder or file to be parsed. Folder search is recursive.",
    )
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
    path_params = parser.add_mutually_exclusive_group()
    path_params.add_argument(
        "--relative-path",
        "-r",
        action="store_const",
        dest="path_type",
        const="relative",
        default="relative",
    )
    path_params.add_argument(
        "--full-path", "-R", action="store_const", dest="path_type", const="full"
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

    tags_regex = (
        "(?:^|(?:(?:#+|//+|<!--|--|/\*|\"\"\"|''')+\s*)+)\s*"
        + f"(?:^|\\b)({'|'.join(args.tags)})[\s:;-]+(.+?)"
        + "(?:$|-->|#\}\}|\*/|--\}\}|\}\}|#+|#\}|\"\"\"|''')"
    )

    args.path = os.path.abspath(args.path)
    # XXX requires ripgrep installation
    cmd = ["rg", tags_regex, args.path, "-n"]
    if args.glob:
        cmd.extend(["-g", args.glob])
    # CONSOLE.print(" ".join(cmd))
    process_out = subprocess.run(cmd, capture_output=True)
    rg_error = process_out.stderr.decode("utf-8")
    if rg_error:
        raise ParsingError(f"ripgrep search failed: {rg_error}")

    rg_output = process_out.stdout.decode("utf-8")

    if os.path.isdir(args.path):
        by_file = parse_rg_output_folder(rg_output)
    else:
        out = parse_rg_output_file(rg_output)
        by_file = {args.path: out}

    for file in sorted(by_file):
        print_parsed_file(file, by_file[file], tags_regex, args)
