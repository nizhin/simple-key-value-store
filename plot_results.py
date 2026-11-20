"""Turn the benchmark CSVs into charts and a summary table for the report.

Usage:
  python3 -m venv .venv && .venv/bin/pip install matplotlib    # once
  .venv/bin/python plot_results.py

Reads   results/results-disk.csv and results/results-tmpfs.csv (from bench_scaling.sh)
Writes  results/*.png and results/summary.md
Bars show the mean of the runs; dots show each individual run, so run-to-run
noise is visible instead of hidden.
"""
import csv
import os
from collections import defaultdict
from statistics import mean

import matplotlib

matplotlib.use('Agg')
import matplotlib.pyplot as plt  # noqa: E402

RESULTS = 'results'
CONFIGS = ['direct', 'router-1', 'router-2', 'router-3']
CONFIG_NAMES = {'direct': '1 node\ndirect', 'router-1': '1 node\nvia router',
                'router-2': '2 nodes\nvia router', 'router-3': '3 nodes\nvia router'}
STORAGE_NAMES = {'disk': 'Durable (disk)', 'tmpfs': 'In-memory (tmpfs)'}

# Colors and ink from a colorblind-validated palette: series slots 1 and 2,
# text in neutral ink (never the series color), recessive grid and axes.
SERIES = ['#2a78d6', '#eb6834']
INK, INK_2, MUTED = '#0b0b0b', '#52514e', '#898781'
GRID, AXIS, SURFACE = '#e1e0d9', '#c3c2b7', '#fcfcfb'

plt.rcParams.update({
    'figure.facecolor': SURFACE, 'axes.facecolor': SURFACE, 'savefig.facecolor': SURFACE,
    'font.size': 10, 'text.color': INK, 'axes.labelcolor': INK_2,
    'xtick.color': INK_2, 'ytick.color': MUTED, 'axes.edgecolor': AXIS,
    'axes.spines.top': False, 'axes.spines.right': False, 'axes.spines.left': False,
    'axes.grid': True, 'axes.grid.axis': 'y', 'grid.color': GRID, 'grid.linewidth': 0.8,
    'axes.axisbelow': True, 'ytick.left': False, 'xtick.bottom': False,
    'legend.frameon': False,
})


def load(storage):
    """Returns {(config, phase): [row, ...]} with numeric fields converted."""
    path = os.path.join(RESULTS, f'results-{storage}.csv')
    data = defaultdict(list)
    if not os.path.exists(path):
        return data
    with open(path) as f:
        for row in csv.DictReader(f):
            for k in ('throughput_ops', 'avg_ms', 'p50_ms', 'p95_ms', 'p99_ms'):
                row[k] = float(row[k])
            for k in ('ok', 'mismatch', 'error'):
                row[k] = int(row[k])
            data[(row['label'], row['phase'])].append(row)
    return data


def bars(ax, series, ylabel, fmt):
    """Grouped bars: series = [(legend name, {config: [values per run]})].
    Mean as the bar, each run as a dot, the mean printed above the bar."""
    n = len(series)
    width = 0.7 / n
    x = range(len(CONFIGS))
    top = max(v for _, vals in series for runs in vals.values() for v in runs)
    for i, (name, vals) in enumerate(series):
        offs = [xi + (i - (n - 1) / 2) * width for xi in x]
        means = [mean(vals[c]) if vals.get(c) else 0 for c in CONFIGS]
        ax.bar(offs, means, width * 0.92, color=SERIES[i], label=name, zorder=2)
        for o, c, m in zip(offs, CONFIGS, means):
            runs = vals.get(c, [])
            ax.scatter([o] * len(runs), runs, s=14, color=SURFACE, edgecolor=INK_2,
                       linewidth=0.8, zorder=3)
            ax.text(o, max(runs + [m]) + top * 0.02, fmt(m), ha='center', va='bottom',
                    fontsize=8.5, color=INK_2)
    ax.set_xticks(list(x), [CONFIG_NAMES[c] for c in CONFIGS])
    ax.set_ylabel(ylabel)
    ax.set_ylim(0, top * 1.15)
    ax.yaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(lambda v, _: f'{v:,.0f}'))
    if n > 1:
        ax.legend(loc='upper left', ncols=n, bbox_to_anchor=(0, 1.02))


def runs(data, phase, metric):
    return {c: [r[metric] for r in data.get((c, phase), [])] for c in CONFIGS}


def save(fig, name):
    path = os.path.join(RESULTS, name)
    fig.tight_layout()
    fig.text(0.99, 0.01, 'Bars: mean of runs. Dots: individual runs.', ha='right', va='bottom',
             fontsize=7.5, color=MUTED)
    fig.savefig(path, dpi=160)
    plt.close(fig)
    print('wrote', path)


def main():
    disk, tmpfs = load('disk'), load('tmpfs')

    # 1. Headline: durable write throughput scales with node count.
    fig, ax = plt.subplots(figsize=(6.4, 3.8))
    bars(ax, [('Writes', runs(disk, 'set', 'throughput_ops'))], 'ops / second', lambda v: f'{v:,.0f}')
    ax.set_title('Durable write throughput by node count', loc='left', fontsize=12, color=INK)
    save(fig, 'write_throughput_disk.png')

    # 2. Durable write latency: median and tail.
    fig, ax = plt.subplots(figsize=(6.4, 3.8))
    bars(ax, [('p50 (median)', runs(disk, 'set', 'p50_ms')), ('p95', runs(disk, 'set', 'p95_ms'))],
         'milliseconds', lambda v: f'{v:,.0f}')
    ax.set_title('Durable write latency by node count', loc='left', fontsize=12, color=INK)
    save(fig, 'write_latency_disk.png')

    # 3. Reads: flat in both storage modes (shared CPU is the limit).
    fig, ax = plt.subplots(figsize=(6.4, 3.8))
    bars(ax, [(STORAGE_NAMES['disk'], runs(disk, 'get', 'throughput_ops')),
              (STORAGE_NAMES['tmpfs'], runs(tmpfs, 'get', 'throughput_ops'))],
         'ops / second', lambda v: f'{v / 1000:.1f}k')
    ax.set_title('Read throughput by node count', loc='left', fontsize=12, color=INK)
    save(fig, 'read_throughput.png')

    # 4. In-memory writes: once the disk is gone, writes stop scaling too.
    fig, ax = plt.subplots(figsize=(6.4, 3.8))
    bars(ax, [('Writes', runs(tmpfs, 'set', 'throughput_ops'))], 'ops / second',
         lambda v: f'{v / 1000:.1f}k')
    ax.set_title('In-memory (tmpfs) write throughput by node count', loc='left', fontsize=12, color=INK)
    save(fig, 'write_throughput_tmpfs.png')

    # Summary table (means over runs) for the report.
    lines = ['| Storage | Config | Phase | Runs | Throughput (ops/s) | avg (ms) | p50 (ms) '
             '| p95 (ms) | p99 (ms) | Errors | Mismatches |',
             '|---|---|---|---|---|---|---|---|---|---|---|']
    for storage, data in (('disk', disk), ('tmpfs', tmpfs)):
        for phase in ('set', 'get'):
            for c in CONFIGS:
                rows = data.get((c, phase), [])
                if not rows:
                    continue
                m = {k: mean(r[k] for r in rows) for k in
                     ('throughput_ops', 'avg_ms', 'p50_ms', 'p95_ms', 'p99_ms')}
                lines.append(
                    f"| {storage} | {c} | {phase} | {len(rows)} | {m['throughput_ops']:,.0f} "
                    f"| {m['avg_ms']:.1f} | {m['p50_ms']:.1f} | {m['p95_ms']:.1f} | {m['p99_ms']:.1f} "
                    f"| {sum(r['error'] for r in rows)} | {sum(r['mismatch'] for r in rows)} |")
    with open(os.path.join(RESULTS, 'summary.md'), 'w') as f:
        f.write('\n'.join(lines) + '\n')
    print('wrote', os.path.join(RESULTS, 'summary.md'))


if __name__ == '__main__':
    main()
