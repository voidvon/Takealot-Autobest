#!/usr/bin/env python3
import sys
import re
import subprocess

def get_latest_git_tag():
    try:
        res = subprocess.run(
            ["git", "tag", "-l", "--sort=-v:refname", "v*"],
            capture_output=True, text=True, check=True
        )
        tags = [t.strip() for t in res.stdout.splitlines() if t.strip()]
        for t in tags:
            if re.match(r"^v?\d+\.\d+\.\d+$", t):
                return t
    except Exception:
        pass
    return None

def bump_version(current: str) -> str:
    if not current:
        return "0.1.0"
    
    current = current.lstrip("v")
    m = re.match(r"^(\d+)\.(\d+)\.(\d+)$", current)
    if not m:
        return "0.1.0"
    
    major, minor, patch = int(m.group(1)), int(m.group(2)), int(m.group(3))

    # Rule 1: 到了0.20.0的下一个版本就是1.0.0这样
    if minor >= 20:
        return f"{major + 1}.0.0"

    # Rule 2: 直到到了0.1.20的下一个版本就到0.2.0 (以及 0.19.20 到 0.20.0)
    if patch >= 20:
        if minor + 1 > 20:
            return f"{major + 1}.0.0"
        return f"{major}.{minor + 1}.0"

    # Rule 3: 首个版本是 0.1.0，下一个版本就 0.1.1
    return f"{major}.{minor}.{patch + 1}"

def main():
    if len(sys.argv) > 1 and sys.argv[1]:
        tag = sys.argv[1]
    else:
        tag = get_latest_git_tag()

    next_ver = bump_version(tag)
    print(next_ver)

if __name__ == "__main__":
    main()
