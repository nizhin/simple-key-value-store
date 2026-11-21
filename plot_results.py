"""Turn the benchmark CSVs into charts and a summary table for the report.

Usage:
  python3 -m venv .venv && .venv/bin/pip install matplotlib    # once
  .venv/bin/python plot_results.py

Reads   results/results-{disk,tmpfs}.csv (bench_scaling.sh) and results-aws.csv (aws/bench_aws.sh)
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
STORAGE_NAMES = {'disk': 'Laptop, durable (disk)', 'tmpfs': 'Laptop, in-memory (tmpfs)',
                 'aws': 'AWS, durable (EBS)'}

# Colors and ink from a colorblind-validated palette: series slots 1 to 3,
# text in neutral ink (never the series color), recessive grid and axes.
SERIES = ['#2a78d6', '#eb6834', '#1baf7a']
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


def bars(ax, series, ylabel, fmt, colors=None):
    """Grouped bars: series = [(legend name, {config: [values per run]})].
    Mean as the bar, each run as a dot, the mean printed above the bar."""
    colors = colors or SERIES
    n = len(series)
    width = 0.7 / n
    x = range(len(CONFIGS))
    top = max(v for _, vals in series for runs in vals.values() for v in runs)
    for i, (name, vals) in enumerate(series):
        offs = [xi + (i - (n - 1) / 2) * width for xi in x]
        means = [mean(vals[c]) if vals.get(c) else 0 for c in CONFIGS]
        ax.bar(offs, means, width * 0.92, color=colors[i], label=name, zorder=2)
        for o, c, m in zip(offs, CONFIGS, means):
            runs = vals.get(c, [])
            ax.scatter([o] * len(runs), runs, s=14, color=SURFACE, edgecolor=INK_2,
                       linewidth=0.8, zorder=3)
            ax.text(o, max(runs + [m]) + top * 0.02, fmt(m), ha='center', va='bottom',
                    fontsize=8.5 if n < 3 else 7.5, color=INK_2)
    ax.set_xticks(list(x), [CONFIG_NAMES[c] for c in CONFIGS])
    ax.set_ylabel(ylabel)
    ax.set_ylim(0, top * 1.15)
    ax.yaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(lambda v, _: f'{v:,.0f}'))
    if n > 1:
        ax.legend(loc="upper left", ncols=n, bbox_to_anchor=(0, 1.02), fontsize=9 if n < 3 else 8.5)


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


def speedup(data):
    """Mean write throughput of router-N divided by router-1, for N = 1, 2, 3."""
    base = mean(runs(data, 'set', 'throughput_ops')['router-1'])
    return [mean(runs(data, 'set', 'throughput_ops')[f'router-{n}']) / base for n in (1, 2, 3)]


def scaling_chart(disk, aws):
    """Write speedup vs node count: AWS (accent), laptop (grey), ideal (dashed reference)."""
    fig, ax = plt.subplots(figsize=(6.4, 3.8))
    nodes = [1, 2, 3]
    ax.plot(nodes, nodes, color=AXIS, linewidth=1.5, linestyle=(0, (4, 3)), zorder=1)
    ax.text(3.06, 3, 'Ideal (3.0×)', va='center', fontsize=9, color=MUTED)
    for data, color, name in ((disk, MUTED, 'Laptop'), (aws, SERIES[0], 'AWS')):
        ys = speedup(data)
        ax.plot(nodes, ys, color=color, linewidth=2, marker='o', markersize=7,
                markeredgecolor=SURFACE, markeredgewidth=1.5, zorder=3)
        ax.text(3.06, ys[-1], f'{name} ({ys[-1]:.1f}×)', va='center', fontsize=9,
                color=INK, fontweight='bold' if name == 'AWS' else 'normal')
    ax.set_xticks(nodes, ['1 node', '2 nodes', '3 nodes'])
    ax.set_xlim(0.8, 3.75)
    ax.set_ylim(0, 3.3)
    ax.set_ylabel('write throughput vs 1 node (×)')
    ax.set_title('On AWS, durable writes scale 2.8× with 3 nodes; on the laptop, 1.3×',
                 loc='left', fontsize=11, color=INK)
    fig.tight_layout()
    fig.text(0.99, 0.01, 'All via the router; baseline = router with 1 node. Means of 3 runs.',
             ha='right', va='bottom', fontsize=7.5, color=MUTED)
    path = os.path.join(RESULTS, 'write_scaling_laptop_vs_aws.png')
    fig.savefig(path, dpi=160)
    plt.close(fig)
    print('wrote', path)


def main():
    disk, tmpfs, aws = load('disk'), load('tmpfs'), load('aws')

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

    # 3. Reads: flat everywhere (laptop: shared CPU; AWS: the 2-vCPU client).
    fig, ax = plt.subplots(figsize=(8.6, 4.2))
    read_series = [(STORAGE_NAMES['disk'], runs(disk, 'get', 'throughput_ops')),
                   (STORAGE_NAMES['tmpfs'], runs(tmpfs, 'get', 'throughput_ops'))]
    if aws:
        read_series.append((STORAGE_NAMES['aws'], runs(aws, 'get', 'throughput_ops')))
    bars(ax, read_series, 'ops / second', lambda v: f'{v / 1000:.1f}k')
    ax.set_title('Reads stay flat at 12–18k ops/s in every setup', loc='left', fontsize=12, color=INK)
    save(fig, 'read_throughput.png')

    # 4. In-memory writes: once the disk is gone, writes stop scaling too.
    fig, ax = plt.subplots(figsize=(6.4, 3.8))
    bars(ax, [('Writes', runs(tmpfs, 'set', 'throughput_ops'))], 'ops / second',
         lambda v: f'{v / 1000:.1f}k')
    ax.set_title('In-memory (tmpfs) write throughput by node count', loc='left', fontsize=12, color=INK)
    save(fig, 'write_throughput_tmpfs.png')

    if aws:
        # 5. AWS durable writes: each node has its own disk, so writes scale.
        fig, ax = plt.subplots(figsize=(6.4, 3.8))
        bars(ax, [('Writes', runs(aws, 'set', 'throughput_ops'))], 'ops / second',
             lambda v: f'{v:,.0f}')
        ax.set_title('AWS: durable writes grow from 154 to 436 ops/s with 3 nodes',
                     loc='left', fontsize=12, color=INK)
        save(fig, 'write_throughput_aws.png')

        # 6. AWS durable write latency.
        fig, ax = plt.subplots(figsize=(6.4, 3.8))
        bars(ax, [('p50 (median)', runs(aws, 'set', 'p50_ms')), ('p95', runs(aws, 'set', 'p95_ms'))],
             'milliseconds', lambda v: f'{v:,.0f}')
        ax.set_title('AWS: median write latency falls from 288 to 55 ms', loc='left',
                     fontsize=12, color=INK)
        save(fig, 'write_latency_aws.png')

        # 7. Headline comparison: speedup vs node count, laptop vs AWS vs ideal.
        scaling_chart(disk, aws)

    # Summary table (means over runs) for the report.
    lines = ['| Storage | Config | Phase | Runs | Throughput (ops/s) | avg (ms) | p50 (ms) '
             '| p95 (ms) | p99 (ms) | Errors | Mismatches |',
             '|---|---|---|---|---|---|---|---|---|---|---|']
    for storage, data in (('disk', disk), ('tmpfs', tmpfs), ('aws', aws)):
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
