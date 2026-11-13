#!/usr/bin/env python3
"""Watch a running probe independently; never read credentials or response bodies."""
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import time


def utc_now():
    return datetime.now(timezone.utc)


def parse_date(value):
    return datetime.fromisoformat(value.replace('Z', '+00:00'))


def write_report(path, state):
    metrics = state.get('metrics')
    if not metrics:
        return
    terminal = state['finished']
    clean = (metrics['phase'] == 'completed' and metrics['failures'] == 0
             and not state['incidents'])
    conclusion = ('本轮计划时长内未发现失败或监控异常。' if clean else '本轮存在失败、暂停或监控异常，需检查记录。') if terminal else '仍在运行，暂不作稳定性结论。'
    lines = ['# WebView 持续只读观察', '', conclusion, '',
             f"- 开始（UTC）：{metrics['started_at']}",
             f"- 原定结束（UTC）：{metrics['planned_end']}",
             f"- 报告更新（UTC）：{state['updated_at']}",
             f"- 观察时间：{metrics['elapsed_seconds'] / 60:.2f} 分钟（包括暂停或等待）",
             f"- 阶段：{metrics['phase']}；结束：{terminal}；进程存活：{state['process_alive']}",
             f"- 已完成请求：{metrics['requests']}；成功：{metrics['successes']}；失败：{metrics['failures']}",
             f"- 再次验证：{metrics['challenges']}；限流：{metrics['rate_limits']}",
             f"- 平均延迟：{metrics['mean_latency_ms']:.0f} ms；最大延迟：{metrics['max_latency_ms']:.0f} ms",
             f"- 独立监控采样：{state['samples']}；异常事件：{len(state['incidents'])}", '',
             '只覆盖本机、当前出口、每分钟 5 次 GET、20 个轮换商品。没有价格写入；此结果不能证明永久免验证或正式调价已修复。', '',
             '## 监控事件', '']
    lines.extend(f"- {entry['at']} {entry['issue']}" for entry in state['incidents'])
    if not state['incidents']:
        lines.append('- 暂无。' if not terminal else '- 无。')
    temporary = path.with_suffix('.tmp')
    temporary.write_text('\n'.join(lines) + '\n')
    os.chmod(temporary, 0o600)
    temporary.replace(path)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--metrics', type=Path, required=True)
    parser.add_argument('--pid', type=int, required=True)
    args = parser.parse_args()
    report = args.metrics.with_name('browser-session-watchdog.json')
    samples = 0
    incidents = []
    active_incidents = set()
    monitoring_started = utc_now()
    try:
        previous = json.loads(report.read_text())
        if previous['probe_pid'] == args.pid:
            samples = previous['samples']
            incidents = previous['incidents']
            active_incidents = set(previous['active_incidents'])
            monitoring_started = parse_date(previous['monitor_started_at'])
    except (OSError, ValueError, KeyError):
        pass
    while True:
        now = utc_now()
        try:
            os.kill(args.pid, 0)
            alive = True
        except ProcessLookupError:
            alive = False
        try:
            metrics = json.loads(args.metrics.read_text())
            if metrics['pid'] != args.pid:
                metrics = None
        except (OSError, ValueError, KeyError):
            metrics = None
        issues = set()
        if not alive:
            issues.add('process_exited')
        if metrics:
            if (now - parse_date(metrics['updated_at'])).total_seconds() > 60:
                issues.add('heartbeat_stale')
            last_response = metrics.get('last_response') or metrics['started_at']
            if not metrics['finished'] and (now - parse_date(last_response)).total_seconds() > 180:
                issues.add('no_response_for_180_seconds')
            if metrics['phase'] in ('verification_required', 'auth_required', 'http_failure', 'invalid_result'):
                issues.add(metrics['phase'])
        elif (now - monitoring_started).total_seconds() > 60:
            issues.add('metrics_missing')
        for issue in sorted(issues - active_incidents):
            incidents.append({'at': now.isoformat(), 'issue': issue})
        active_incidents = issues
        samples += 1
        finished = bool(metrics and metrics['finished']) or not alive
        state = {'monitor_started_at': monitoring_started.isoformat(), 'updated_at': now.isoformat(),
                 'probe_pid': args.pid, 'process_alive': alive, 'samples': samples,
                 'finished': finished, 'active_incidents': sorted(issues),
                 'incidents': incidents, 'metrics': metrics}
        temporary = report.with_suffix('.tmp')
        temporary.write_text(json.dumps(state, ensure_ascii=False, indent=2) + '\n')
        os.chmod(temporary, 0o600)
        temporary.replace(report)
        write_report(report.with_name('browser-session-soak-results.md'), state)
        if finished or (now - monitoring_started).total_seconds() > 26 * 3600:
            return
        time.sleep(15)


if __name__ == '__main__':
    main()
