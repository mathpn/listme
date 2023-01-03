"""
Git integration tools.
"""

import os
import re
import subprocess
from dataclasses import dataclass
from datetime import datetime
from typing import Dict, List, Tuple


@dataclass
class AuthorInfo:
    """Minimal git author information."""

    name: str = ""
    date: datetime = datetime.today()


def blame_file(file_path: str) -> List[str]:
    """Return the git blame output for the given file."""
    blame = subprocess.getoutput(
        f"cd {os.path.dirname(file_path)} && git blame {file_path} --porcelain"
    )
    return blame.splitlines()


def parse_blame(
    blame: str, current_hash: str, lines_hash: Dict[int, str], hash_authors: Dict[str, AuthorInfo]
) -> Tuple[str, Dict[int, str], Dict[str, AuthorInfo]]:
    """Parse a git blame output."""
    hash_regex = r"^(.{40}) \d+ (\d+)"
    author_regex = r"^author (.+)"
    time_regex = r"^author-time (\d+)"

    if match := re.match(hash_regex, blame):
        groups = match.groups()
        if len(groups) == 2:
            current_hash, line = groups
            lines_hash[int(line)] = current_hash
    elif match := re.match(author_regex, blame):
        groups = match.groups()
        if len(groups) == 1:
            hash_authors.setdefault(current_hash, AuthorInfo())
            hash_authors[current_hash].name = groups[0]
    elif match := re.match(time_regex, blame):
        groups = match.groups()
        if len(groups) == 1:
            timestamp = groups[0]
            hash_authors.setdefault(current_hash, AuthorInfo())
            hash_authors[current_hash].date = datetime.fromtimestamp(float(timestamp))

    return current_hash, lines_hash, hash_authors


def blame_lines(file_path: str, lines: List[int]) -> List[AuthorInfo]:
    """Return a list of (name, datetime) for each line to be blamed."""
    blames = blame_file(file_path)
    default_author = AuthorInfo()
    if blames[0].startswith("fatal"):
        return [default_author for _ in range(1 + max(lines))]
    cur_hash = "null"
    lines_hash: Dict[int, str] = {}
    hash_authors: Dict[str, AuthorInfo] = {}
    for blame in blames:
        cur_hash, lines_hash, hash_authors = parse_blame(blame, cur_hash, lines_hash, hash_authors)
    return [hash_authors.get(lines_hash.get(line, "no-hash"), default_author) for line in lines]
