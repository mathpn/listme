"""
Parser module with core rememberme functionality.
The main function is the entry-point for running rememberme.
"""

import argparse
import os
import re
import subprocess
from datetime import datetime, timedelta
from typing import Dict, List

from rich.console import Console
from rich.padding import Padding
from rich.panel import Panel
from rich.table import Table

from rememberme.git_tools import AuthorInfo, blame_lines
from rememberme.config import get_config, wizard

COMMENT_REGEX = (
    r"^(?:(?:(?:#+|//+|<!--|--|/\*|\"\"\"|''')+\s*)+)\s*|(?:-->|#}}|\*\/|--}}|}}|#+|#}|\"\"\"|''')*$"
)
CONSOLE = Console(highlight=False)
CONFIG = get_config()


class ParsingError(Exception):
    """Parsing error exception."""


def boldify(string: str) -> str:
    """Turn text to bold format."""
    return f"[bold]{string}[/bold]"


def colorize(string: str, tag: str) -> str:
    """Colorize text based on tag."""
    if tag == "TODO":
        return f"[cadet_blue]{string}[/cadet_blue]"
    if tag == "XXX":
        return f"[grey0 on gold3]{string}[/grey0 on gold3]"
    if tag == "FIXME":
        return f"[red1]{string}[/red1]"
    if tag == "OPTIMIZE":
        return f"[orange3]{string}[/orange3]"
    if tag == "BUG":
        return f"[grey93 on dark_red]{string}[/grey93 on dark_red]"
    if tag == "NOTE":
        return f"[dark_sea_green4]{string}[/dark_sea_green4]"
    if tag == "HACK":
        return f"[yellow3]{string}[/yellow3]"
    if tag == "#OLD__COMMIT":  # used to mark old comments (by commit date)
        return f"[grey85 on red3]{string}[/grey85 on red3]"
    return string


def emojify(tag: str) -> str:
    """Prepend a unicode symbol to tags if allowed by CONFIG."""
    if CONFIG.get("extra_symbols"):
        return _emojify(tag)
    return tag


def _emojify(tag: str) -> str:
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
        match = re.findall("^(.+):([0-9]+):(.*)", line)
        if not match:
            continue
        if len(match[0]) != 3:
            raise ParsingError(
                f"ripgrep output could not be parsed: {line} -> tags_regex match = {match}"
            )
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
        match = re.findall("^([0-9]+):(.*)", line)
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


def prettify_summary(file_summary: Dict[str, int], style: str = "full") -> Padding:
    """Add rich text formatting to file summary."""
    file_summary = [(tag, count) for tag, count in file_summary.items() if count > 0]
    summary = (f" {boldify(emojify(tag))}: {count} " for tag, count in file_summary)
    if style == "full":
        summary = (colorize(string, tag) for string, (tag, _) in zip(summary, file_summary))

    return Padding(Panel(" ".join(summary), expand=False), pad=(0, 0, 0, 2))


def prettify_line(text: str, tag: str, style: str):
    """Add rich text formatting to comment line."""
    text = re.sub(COMMENT_REGEX, "", text)
    text = boldify(emojify(tag)) + ": " + text + " "
    if style == "full":
        text = colorize(text, tag)
    return text


def get_plain_line(tag: str, text: str) -> str:
    return f"\t{tag}: {text}"


def stylize_filename(file: str, n_lines: int, style: str) -> str:
    """Add rich text formatting to filename."""
    if style == "plain":
        return file
    if style == "full":
        return f"[bold deep_sky_blue3]• {file}[/bold deep_sky_blue3] ({n_lines} comments):"
    if style == "bw":
        return f"[bold]• {file}[/bold] ({n_lines} comments):"
    return f"{file}"


def tag_git_author(author_info: AuthorInfo, tag: str, age_limit: int, style: str) -> str:
    """Colorize and tag git author if the entry is too old."""
    if not author_info.name or style == "plain":
        return ""

    git_author = author_info.name
    if author_info.date < datetime.now() - timedelta(days=age_limit):
        if CONFIG.get("extra_symbols"):
            git_author = f"[bold]☠ OLD {git_author}[/bold]"
        else:
            git_author = f"[bold]OLD {git_author}[/bold]"
        tag = "#OLD__COMMIT"

    git_author = f"[{git_author}]"
    if style == "full":
        git_author = f"{colorize(git_author, tag)} "
    return f" {git_author} "


def log_warning(msg: str, verbose: bool = False) -> None:
    """Log warning if verbose is set to True."""
    if verbose:
        print(f"WARNING: {msg}")


def print_summary(tag_counter: Dict[str, int], style: str):
    """Print a file summary with rich formatting."""
    if sum(count > 0 for count in tag_counter.values()) > 1 and style != "plain":
        CONSOLE.print(prettify_summary(tag_counter, style=style))


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
            log_warning(f"the following line could not be parsed:\n{text}", args.verbose)
            continue
        groups = matches.groups()
        if len(groups) != 2:
            log_warning(f"the following line could not be parsed:\n{text}", args.verbose)
            continue

        tag, txt = groups
        tag_counter[tag] += 1

        if args.style == "plain":
            text = get_plain_line(tag, txt)
            print_lines.append(text)
            continue

        text = prettify_line(txt, tag, args.style)
        grid = Table.grid(expand=False, pad_edge=True)
        grid.add_column(justify="left", width=max_digits + 8)
        grid.add_column(justify="left", width=CONSOLE.width // 2 - max_digits)
        row = [pad_line_number(lines[i], max_digits), text]
        if args.author:
            author_info: AuthorInfo = blames[i]
            git_author = tag_git_author(author_info, tag, args.age_limit, args.style)
            grid.add_column(justify="left")
            row.append(git_author)
        grid.add_row(*row)
        padded_grid = Padding(grid, (0, 0, 0, 2))
        print_lines.append(padded_grid)

    tag_counter = tag_counter if args.summary else {}
    print_summary(tag_counter, args.style)

    for line in print_lines:
        CONSOLE.print(line)
    CONSOLE.print()


def shorten_filepath(file_path: str, search_path: str, path_type: str) -> str:
    """Shorten filepath depending on the chosen option."""
    if path_type == "relative":
        rel_path = file_path.replace(search_path, "").strip("/")
        rel_path = rel_path if rel_path else os.path.basename(file_path)  # file search
        return rel_path
    return file_path


def main(raw_args=None):
    """Run Rememberme."""
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "path",
        nargs="?",
        type=str,
        default=os.getcwd(),
        help="Path to folder or file to be parsed. Folder search is recursive.",
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
        help="Glob pattern to include/exclude files or folders in the search. Must be a single-quoted string. Same syntax as ripgrep.",
    )
    parser.add_argument(
        "--age-limit",
        "-l",
        type=int,
        default=60,
        help="Age limit for comments. Comments older than this limit are marked.",
    )
    parser.add_argument("--verbose", "-v", action="store_true")
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
    args = parser.parse_args(raw_args)

    if not all(re.match("^(\w+)$", tag) for tag in args.tags):
        raise ValueError(
            "provided tags must be non-empty and contain only alphanumeric or underscore characters"
        )

    global CONFIG
    if not CONFIG:
        wizard()
        CONFIG = get_config()

    tags_regex = (
        "(?:^|(?:(?:#+|//+|<!--|--|/\*|\"\"\"|''')+\s*)+)\s*"
        + f"(?:^|\\b)({'|'.join(args.tags)})[\s:;-]+(.+?)"
        + "(?:$|-->|#\}\}|\*/|--\}\}|\}\}|#+|#\}|\"\"\"|''')*$"
    )

    args.path = os.path.abspath(args.path)
    # NOTE requires ripgrep installation
    cmd = ["rg", tags_regex, args.path, "-n"]
    if args.glob:
        cmd.extend(["-g", args.glob])

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
