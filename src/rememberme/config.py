"""
Configuration wizard to check for unicode symbol support.
"""

import json
import os
from typing import Any, Dict, Optional
from platformdirs import user_config_dir
from rich.console import Console


SYMBOLS = [
    ("✓", "checkmark"),
    ("✘", "X mark"),
    ("⚠", "warning sign"),
    ("", "lightning"),
    ("☢", "radioactive sign"),
    ("✐", "pencil"),
    ("✄", "scisors"),
]


def get_config() -> Dict[str, Any]:
    try:
        config = json.load(open(f"{user_config_dir()}/rememberme/config.json", encoding="utf-8"))
        return config
    except Exception:
        return {}


def wizard():
    CONSOLE = Console(highlight=False)
    CONSOLE.print("Welcome to the rememberme configuration wizard!")
    CONSOLE.print("Can you see the following symbols correctly?")
    for symbol, name in SYMBOLS:
        CONSOLE.print(f"{symbol} -> {name}")
    choice = ""
    while choice.lower() not in ("y", "n"):
        choice = input("Please choose yes or no [y/n]: ")

    support = choice.lower() == "y"
    if not support:
        CONSOLE.print(
            "no problem, those symbols will not be shown. However, a font with extra unicode symbol support is strongly recommended!"
        )

    os.makedirs(f"{user_config_dir()}/rememberme", exist_ok=True)
    with open(f"{user_config_dir()}/rememberme/config.json", "w", encoding="utf-8") as config:
        json.dump({"extra_symbols": support}, config)
    CONSOLE.print("you can run this wizard again by calling rememberme-config")
