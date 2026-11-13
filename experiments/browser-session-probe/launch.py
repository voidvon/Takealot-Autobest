#!/usr/bin/env python3
"""Build and start the macOS read-only persistent WKWebView experiment."""
import argparse
import json
import os
from pathlib import Path
import plistlib
import sqlite3
import subprocess
import sys
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--database', type=Path, required=True)
    parser.add_argument('--store', default='default')
    parser.add_argument('--duration-minutes', type=int, default=60)
    parser.add_argument('--round-interval', type=int, default=60)
    args = parser.parse_args()
    if not 1 <= args.duration_minutes <= 1440 or args.round_interval < 30:
        parser.error('duration must be 1..1440 minutes and round interval >=30 seconds')
    if sys.platform != 'darwin':
        parser.error('This WKWebView feasibility experiment currently requires macOS 14+.')
    database = args.database.resolve(strict=True)
    with sqlite3.connect(database.as_uri() + '?mode=ro', uri=True) as connection:
        samples = []
        for target, in connection.execute('SELECT target_key FROM reprice_targets WHERE store_id=? AND selected=1 LIMIT 20', (args.store,)):
            parts = target.split('/')
            if len(parts) == 2 and all(p.isdigit() for p in parts):
                samples.append({'tsin': parts[0], 'plid': parts[1]})
        if not samples:
            samples = [{'tsin': '100392696', 'plid': '98958583'}]
        row = connection.execute('SELECT authorization, proxy_url FROM stores WHERE id=?', (args.store,)).fetchone()
    if not row or not row[0].strip():
        parser.error('The selected store has no API key.')
    # This prototype deliberately uses the actual WebKit stack. Do not claim
    # the Go client's per-store proxy was applied when it was not.
    if row[1].strip():
        parser.error('This prototype uses system networking/TUN; per-store proxy is not supported yet.')
    auth = row[0].strip()
    if not auth.startswith(('Key ', 'Bearer ')):
        auth = 'Key ' + auth
    project = Path(__file__).resolve().parents[2]
    bundle = project / 'build/bin/TakealotSessionProbe.app'
    macos = bundle / 'Contents/MacOS'
    macos.mkdir(parents=True, exist_ok=True)
    binary = macos / 'TakealotSessionProbe'
    source = Path(__file__).with_name('Probe.swift')
    subprocess.run(['xcrun', 'swiftc', '-parse-as-library', '-swift-version', '5', '-target',
                    'arm64-apple-macosx14.0' if os.uname().machine == 'arm64' else 'x86_64-apple-macosx14.0',
                    str(source), '-o', str(binary), '-framework', 'Cocoa', '-framework', 'WebKit'], check=True)
    with (bundle / 'Contents/Info.plist').open('wb') as handle:
        plistlib.dump({'CFBundleIdentifier': 'com.takealot.sessionprobe',
                      'CFBundleName': 'Takealot Session Probe',
                      'CFBundleExecutable': binary.name,
                      'CFBundlePackageType': 'APPL', 'LSMinimumSystemVersion': '14.0',
                      'NSHighResolutionCapable': True}, handle)
    profile = uuid.uuid5(uuid.NAMESPACE_URL, str(database) + '#' + args.store)
    log = database.parent / 'browser-session-probe.log'
    log.touch(mode=0o600, exist_ok=True)
    os.chmod(log, 0o600)
    metrics = database.parent / 'browser-session-metrics.json'
    config = {'authorization': auth, 'profile': str(profile), 'logPath': str(log),
              'metricsPath': str(metrics), 'durationMinutes': args.duration_minutes,
              'roundInterval': args.round_interval, 'samples': samples}
    with log.open('ab') as output:
        process = subprocess.Popen([str(binary)], stdin=subprocess.PIPE, stdout=output,
                                   stderr=output, start_new_session=True)
    # No key in argv, environment, temporary config files, URLs or logs.
    process.stdin.write(json.dumps(config).encode())
    process.stdin.close()
    print(f'Prototype PID: {process.pid}\nApp: {bundle}\nLog: {log}')


if __name__ == '__main__':
    main()
