#!/usr/bin/env python3
"""Thorough analysis of CMA-ES optimization log for exp10."""

import pandas as pd
import numpy as np
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
from matplotlib.gridspec import GridSpec
import warnings
warnings.filterwarnings('ignore')

OUT = '/Users/pthm-cable/dev/soup/experiments/exp10'
df = pd.read_csv(f'{OUT}/optimize_log.csv')

# Parameter bounds from params.go
BOUNDS = {
    'prey_move_cost':        (0.05, 0.25),
    'pred_move_cost':        (0.01, 0.08),
    'pred_bite_reward':      (0.4, 0.8),
    'pred_digest_time':      (0.2, 3.0),
    'pred_repro_thresh':     (0.5, 0.95),
    'prey_cooldown':         (4.0, 20.0),
    'pred_cooldown':         (6.0, 20.0),
    'parent_energy_split':   (0.4, 0.7),
    'spawn_offset':          (5.0, 30.0),
    'heading_jitter':        (0.0, 1.0),
    'pred_density_k':        (10.0, 600.0),
    'newborn_hunt_cooldown': (0.5, 5.0),
    'refugia_strength':      (0.5, 1.5),
    'part_spawn_rate':       (20.0, 300.0),
    'part_initial_mass':     (0.002, 0.05),
    'part_deposit_rate':     (0.5, 5.0),
    'part_pickup_rate':      (0.1, 3.0),
    'part_cell_capacity':    (0.3, 2.0),
    'detritus_fraction':     (0.0, 0.30),
    'carcass_fraction':      (0.30, 0.90),
    'detritus_decay_rate':   (0.01, 0.20),
    'detritus_decay_eff':    (0.30, 0.80),
    'grazing_diet_cap':      (0.15, 0.50),
    'hunting_diet_floor':    (0.50, 0.85),
    'prey_density_k':        (50.0, 500.0),
}

DEFAULTS = {
    'prey_move_cost': 0.12, 'pred_move_cost': 0.025, 'pred_bite_reward': 0.5,
    'pred_digest_time': 0.8, 'pred_repro_thresh': 0.85, 'prey_cooldown': 8.0,
    'pred_cooldown': 12.0, 'parent_energy_split': 0.55, 'spawn_offset': 15.0,
    'heading_jitter': 0.25, 'pred_density_k': 0, 'newborn_hunt_cooldown': 2.0,
    'refugia_strength': 1.0, 'part_spawn_rate': 100, 'part_initial_mass': 0.01,
    'part_deposit_rate': 2.0, 'part_pickup_rate': 0.5, 'part_cell_capacity': 1.0,
    'detritus_fraction': 0.10, 'carcass_fraction': 0.70, 'detritus_decay_rate': 0.05,
    'detritus_decay_eff': 0.50, 'grazing_diet_cap': 0.30, 'hunting_diet_floor': 0.70,
    'prey_density_k': 200,
}

NEW_PARAMS = [
    'detritus_fraction', 'carcass_fraction', 'detritus_decay_rate',
    'detritus_decay_eff', 'grazing_diet_cap', 'hunting_diet_floor', 'prey_density_k'
]

param_cols = [c for c in df.columns if c not in ('eval', 'fitness')]

# Max ticks = 3M, dt = 1/60
MAX_TICKS = 3_000_000
DT = 1.0/60.0
MAX_SIM_SEC = MAX_TICKS * DT  # 50000 seconds

# ============================================================
# 1. CONVERGENCE PLOT
# ============================================================
fig, axes = plt.subplots(2, 1, figsize=(14, 10), gridspec_kw={'height_ratios': [2, 1]})

# Best-so-far curve
best_so_far = df['fitness'].cummin()
ax = axes[0]
ax.scatter(df['eval'], df['fitness'], alpha=0.35, s=15, c='steelblue', label='Each evaluation')
ax.plot(df['eval'], best_so_far, 'r-', linewidth=2, label='Best so far')
ax.set_xlabel('Evaluation #')
ax.set_ylabel('Fitness (lower = better)')
ax.set_title('CMA-ES Convergence: exp10 (200 evaluations, 25 params)')
ax.legend()
ax.grid(True, alpha=0.3)

# Rolling mean of fitness (window=10)
ax2 = axes[1]
rolling = df['fitness'].rolling(10, min_periods=1).mean()
ax2.plot(df['eval'], rolling, 'steelblue', linewidth=1.5, label='Rolling mean (w=10)')
ax2.plot(df['eval'], best_so_far, 'r-', linewidth=1.5, label='Best so far')
ax2.set_xlabel('Evaluation #')
ax2.set_ylabel('Fitness')
ax2.set_title('Rolling Mean Fitness')
ax2.legend()
ax2.grid(True, alpha=0.3)

plt.tight_layout()
plt.savefig(f'{OUT}/convergence.png', dpi=150, bbox_inches='tight')
plt.close()
print("Saved convergence.png")

# ============================================================
# 2. SURVIVAL ANALYSIS
# ============================================================
# Approximate survival seconds: fitness ≈ -survival_ticks * (1 + 0.2*quality)
# We know quality ∈ [0,1], so survival_ticks ≈ -fitness / (1 + 0.2*quality)
# Without quality data, rough estimate: survival_sec ≈ (-fitness / 1.1) * DT  (assume avg quality ~0.5)
# More precise: survival_ticks is between -fitness/1.2 and -fitness/1.0
# Use midpoint estimate with quality ~0.5: survival_ticks ≈ -fitness / 1.1

df['approx_survival_ticks'] = -df['fitness'] / 1.1
df['approx_survival_sec'] = df['approx_survival_ticks'] * DT

# For the theoretical max: MAX_TICKS with quality=1 -> fitness = -(3M * 1.2) = -3600000
# With quality=0 -> fitness = -3M = -3000000
# So anything with fitness <= -3000000 definitely hit the cap
df['likely_hit_cap'] = df['fitness'] <= -MAX_TICKS  # conservative: quality >= 0

fig, axes = plt.subplots(2, 1, figsize=(14, 10))

ax = axes[0]
ax.scatter(df['eval'], df['approx_survival_sec'], alpha=0.4, s=15, c='teal')
ax.axhline(y=MAX_SIM_SEC, color='red', linestyle='--', linewidth=1.5, label=f'Max cap = {MAX_SIM_SEC:.0f}s')
rolling_surv = df['approx_survival_sec'].rolling(10, min_periods=1).mean()
ax.plot(df['eval'], rolling_surv, 'orange', linewidth=2, label='Rolling mean (w=10)')
ax.set_xlabel('Evaluation #')
ax.set_ylabel('Approx Survival (sim seconds)')
ax.set_title('Survival Duration Over Evaluations')
ax.legend()
ax.grid(True, alpha=0.3)

# Histogram of survival
ax2 = axes[1]
bins = np.linspace(0, MAX_SIM_SEC * 1.1, 40)
ax2.hist(df['approx_survival_sec'], bins=bins, color='teal', alpha=0.7, edgecolor='white')
ax2.axvline(x=MAX_SIM_SEC, color='red', linestyle='--', linewidth=1.5, label=f'Max cap = {MAX_SIM_SEC:.0f}s')
ax2.set_xlabel('Approx Survival (sim seconds)')
ax2.set_ylabel('Count')
ax2.set_title('Distribution of Survival Durations')
ax2.legend()
ax2.grid(True, alpha=0.3)

plt.tight_layout()
plt.savefig(f'{OUT}/survival.png', dpi=150, bbox_inches='tight')
plt.close()
print("Saved survival.png")

# ============================================================
# 3. PARAMETER CONVERGENCE
# ============================================================
last20 = df.tail(20)

convergence_data = []
for p in param_cols:
    lo, hi = BOUNDS[p]
    rng = hi - lo
    mean_last20 = last20[p].mean()
    std_last20 = last20[p].std()

    # Best overall evaluation
    best_idx = df['fitness'].idxmin()
    best_val = df.loc[best_idx, p]

    # Normalized std (relative to range)
    norm_std = std_last20 / rng if rng > 0 else 0

    # Check if hitting bounds
    bound_threshold = 0.05 * rng  # within 5% of bound
    hitting_min = mean_last20 <= lo + bound_threshold
    hitting_max = mean_last20 >= hi - bound_threshold

    # Convergence: norm_std < 0.10 = converged, > 0.20 = exploring
    if norm_std < 0.10:
        status = 'converged'
    elif norm_std > 0.20:
        status = 'exploring'
    else:
        status = 'narrowing'

    if hitting_min:
        status += ' (hitting MIN)'
    elif hitting_max:
        status += ' (hitting MAX)'

    convergence_data.append({
        'param': p,
        'min': lo,
        'max': hi,
        'default': DEFAULTS.get(p, None),
        'best': best_val,
        'last20_mean': mean_last20,
        'last20_std': std_last20,
        'norm_std': norm_std,
        'status': status,
    })

conv_df = pd.DataFrame(convergence_data)

# Plot parameter convergence traces
n_params = len(param_cols)
n_cols = 5
n_rows = int(np.ceil(n_params / n_cols))

fig, axes = plt.subplots(n_rows, n_cols, figsize=(22, n_rows * 3))
axes = axes.flatten()

for i, p in enumerate(param_cols):
    ax = axes[i]
    lo, hi = BOUNDS[p]

    ax.plot(df['eval'], df[p], alpha=0.5, linewidth=0.8, color='steelblue')
    # Rolling mean
    rm = df[p].rolling(10, min_periods=1).mean()
    ax.plot(df['eval'], rm, 'orange', linewidth=1.5)

    ax.axhline(y=lo, color='red', linestyle=':', alpha=0.5)
    ax.axhline(y=hi, color='red', linestyle=':', alpha=0.5)

    # Mark best value
    best_idx = df['fitness'].idxmin()
    ax.axhline(y=df.loc[best_idx, p], color='green', linestyle='--', alpha=0.7, linewidth=1)

    ax.set_title(p, fontsize=9, fontweight='bold')
    ax.set_ylim(lo - 0.05*(hi-lo), hi + 0.05*(hi-lo))
    ax.tick_params(labelsize=7)
    ax.grid(True, alpha=0.2)

# Hide unused axes
for i in range(n_params, len(axes)):
    axes[i].set_visible(False)

plt.suptitle('Parameter Traces Over 200 Evaluations (orange=rolling mean, green=best)', fontsize=13)
plt.tight_layout(rect=[0, 0, 1, 0.97])
plt.savefig(f'{OUT}/parameter_convergence.png', dpi=150, bbox_inches='tight')
plt.close()
print("Saved parameter_convergence.png")

# ============================================================
# 4. PARAMETER CORRELATIONS WITH FITNESS
# ============================================================
# Use top 20% of evaluations
top_frac = 0.20
top_n = max(1, int(len(df) * top_frac))
top_df = df.nsmallest(top_n, 'fitness')

# Compute correlations with fitness for all params
corr_all = df[param_cols + ['fitness']].corr()['fitness'].drop('fitness')
corr_top = top_df[param_cols + ['fitness']].corr()['fitness'].drop('fitness')

fig, axes = plt.subplots(1, 2, figsize=(16, 8))

# All evaluations
ax = axes[0]
sorted_corr = corr_all.sort_values()
colors = ['green' if v < 0 else 'red' for v in sorted_corr.values]
ax.barh(range(len(sorted_corr)), sorted_corr.values, color=colors, alpha=0.7)
ax.set_yticks(range(len(sorted_corr)))
ax.set_yticklabels(sorted_corr.index, fontsize=8)
ax.set_xlabel('Correlation with Fitness')
ax.set_title('All Evaluations\n(negative = correlated with BETTER fitness)')
ax.axvline(x=0, color='black', linewidth=0.5)
ax.grid(True, alpha=0.3)

# Top 20%
ax = axes[1]
sorted_corr_top = corr_top.sort_values()
colors = ['green' if v < 0 else 'red' for v in sorted_corr_top.values]
ax.barh(range(len(sorted_corr_top)), sorted_corr_top.values, color=colors, alpha=0.7)
ax.set_yticks(range(len(sorted_corr_top)))
ax.set_yticklabels(sorted_corr_top.index, fontsize=8)
ax.set_xlabel('Correlation with Fitness')
ax.set_title('Top 20% Evaluations\n(negative = correlated with BETTER fitness)')
ax.axvline(x=0, color='black', linewidth=0.5)
ax.grid(True, alpha=0.3)

plt.suptitle('Parameter Correlations with Fitness', fontsize=13)
plt.tight_layout(rect=[0, 0, 1, 0.95])
plt.savefig(f'{OUT}/correlations.png', dpi=150, bbox_inches='tight')
plt.close()
print("Saved correlations.png")

# ============================================================
# 5. NEW EXP10 PARAMETERS ANALYSIS
# ============================================================
fig, axes = plt.subplots(2, 4, figsize=(18, 8))
axes = axes.flatten()

for i, p in enumerate(NEW_PARAMS):
    ax = axes[i]
    lo, hi = BOUNDS[p]

    ax.scatter(df['eval'], df[p], c=df['fitness'], cmap='RdYlGn_r', alpha=0.6, s=20)
    rm = df[p].rolling(10, min_periods=1).mean()
    ax.plot(df['eval'], rm, 'black', linewidth=2)

    ax.axhline(y=lo, color='red', linestyle=':', alpha=0.5)
    ax.axhline(y=hi, color='red', linestyle=':', alpha=0.5)
    ax.axhline(y=DEFAULTS[p], color='blue', linestyle='--', alpha=0.5, label=f'default={DEFAULTS[p]}')

    best_idx = df['fitness'].idxmin()
    ax.axhline(y=df.loc[best_idx, p], color='green', linestyle='-', alpha=0.8, linewidth=2, label=f'best={df.loc[best_idx, p]:.4f}')

    ax.set_title(p, fontsize=10, fontweight='bold')
    ax.set_ylim(lo - 0.05*(hi-lo), hi + 0.05*(hi-lo))
    ax.legend(fontsize=7)
    ax.grid(True, alpha=0.2)

# Hide last subplot if odd number
if len(NEW_PARAMS) < len(axes):
    axes[-1].set_visible(False)

plt.suptitle('New exp10 Parameters (color=fitness, black=rolling mean)', fontsize=13)
plt.tight_layout(rect=[0, 0, 1, 0.95])
plt.savefig(f'{OUT}/new_params.png', dpi=150, bbox_inches='tight')
plt.close()
print("Saved new_params.png")

# ============================================================
# PRINT REPORT
# ============================================================
print("\n" + "="*80)
print("CMA-ES OPTIMIZATION ANALYSIS: exp10")
print("="*80)

print(f"\n--- OVERVIEW ---")
print(f"Total evaluations: {len(df)}")
print(f"Parameters optimized: {len(param_cols)}")
print(f"Max ticks per run: {MAX_TICKS:,} ({MAX_SIM_SEC:,.0f} sim-seconds)")

print(f"\n--- 1. CONVERGENCE ---")
best_row = df.loc[df['fitness'].idxmin()]
print(f"Best fitness: {best_row['fitness']:.2f} (eval #{int(best_row['eval'])})")
print(f"Worst fitness: {df['fitness'].max():.2f}")
print(f"Mean fitness (all): {df['fitness'].mean():.2f}")
print(f"Mean fitness (last 20): {last20['fitness'].mean():.2f}")
print(f"Best-so-far at eval 50: {df.head(50)['fitness'].min():.2f}")
print(f"Best-so-far at eval 100: {df.head(100)['fitness'].min():.2f}")
print(f"Best-so-far at eval 150: {df.head(150)['fitness'].min():.2f}")
print(f"Best-so-far at eval 200: {df['fitness'].min():.2f}")

# Check if still improving: compare best in first half vs second half
first_half_best = df.head(100)['fitness'].min()
second_half_best = df.tail(100)['fitness'].min()
improvement = first_half_best - second_half_best
print(f"\nFirst 100 evals best: {first_half_best:.2f}")
print(f"Last 100 evals best: {second_half_best:.2f}")
print(f"Improvement in second half: {improvement:.2f}")

# Last 20 improvement rate
last20_best = last20['fitness'].min()
prev20 = df.iloc[-40:-20]
prev20_best = prev20['fitness'].min()
print(f"Best in evals 161-180: {prev20_best:.2f}")
print(f"Best in evals 181-200: {last20_best:.2f}")

if abs(improvement) < abs(first_half_best) * 0.02:
    print("VERDICT: Largely converged (< 2% improvement in second half)")
elif improvement > 0:
    print("VERDICT: Still improving (second half found better solutions)")
else:
    print("VERDICT: Converged early; no improvement in second half")

print(f"\n--- 2. SURVIVAL ANALYSIS ---")
approx_surv = df['approx_survival_sec']
n_hit_cap_conservative = (df['fitness'] <= -MAX_TICKS).sum()
n_hit_cap_generous = (df['fitness'] <= -MAX_TICKS * 1.2).sum()
print(f"Approx survival (mean): {approx_surv.mean():.0f}s")
print(f"Approx survival (median): {approx_surv.median():.0f}s")
print(f"Approx survival (max): {approx_surv.max():.0f}s")
print(f"Approx survival (min): {approx_surv.min():.0f}s")
print(f"Runs hitting max cap (fitness <= -{MAX_TICKS}): {n_hit_cap_conservative}/{len(df)} ({100*n_hit_cap_conservative/len(df):.1f}%)")
print(f"Runs with fitness <= -{MAX_TICKS*1.2:.0f} (cap + max quality): {n_hit_cap_generous}/{len(df)} ({100*n_hit_cap_generous/len(df):.1f}%)")

# Survival by quartile of evaluations
for q, label in [(slice(0, 50), 'Evals 1-50'), (slice(50, 100), 'Evals 51-100'),
                  (slice(100, 150), 'Evals 101-150'), (slice(150, 200), 'Evals 151-200')]:
    chunk = df.iloc[q]
    n_cap = (chunk['fitness'] <= -MAX_TICKS).sum()
    print(f"  {label}: mean surv={chunk['approx_survival_sec'].mean():.0f}s, "
          f"hit cap={n_cap}/{len(chunk)} ({100*n_cap/len(chunk):.0f}%)")

print(f"\n--- 3. PARAMETER CONVERGENCE ---")
print(f"\n{'Parameter':<25} {'Best':>10} {'Last20 Mean':>12} {'Last20 Std':>11} {'NormStd':>8} {'Status'}")
print("-" * 100)
for _, row in conv_df.iterrows():
    print(f"{row['param']:<25} {row['best']:>10.4f} {row['last20_mean']:>12.4f} {row['last20_std']:>11.4f} {row['norm_std']:>8.3f} {row['status']}")

# Group by status
converged = conv_df[conv_df['status'].str.startswith('converged')]
narrowing = conv_df[conv_df['status'].str.startswith('narrowing')]
exploring = conv_df[conv_df['status'].str.startswith('exploring')]
hitting_bound = conv_df[conv_df['status'].str.contains('hitting')]

print(f"\nConverged ({len(converged)}): {', '.join(converged['param'].tolist())}")
print(f"Narrowing ({len(narrowing)}): {', '.join(narrowing['param'].tolist())}")
print(f"Exploring ({len(exploring)}): {', '.join(exploring['param'].tolist())}")
print(f"Hitting bounds ({len(hitting_bound)}): {', '.join(hitting_bound['param'].tolist())}")

print(f"\n--- 4. PARAMETER CORRELATIONS WITH FITNESS ---")
print(f"\n(Negative correlation = associated with BETTER fitness)")
print(f"\n{'Parameter':<25} {'Corr (all)':>12} {'Corr (top20%)':>14}")
print("-" * 55)
combined = pd.DataFrame({'all': corr_all, 'top20': corr_top})
combined['abs_all'] = combined['all'].abs()
combined = combined.sort_values('abs_all', ascending=False)
for p, row in combined.iterrows():
    marker = '***' if abs(row['all']) > 0.3 else '**' if abs(row['all']) > 0.2 else '*' if abs(row['all']) > 0.1 else ''
    print(f"{p:<25} {row['all']:>+12.3f} {row['top20']:>+14.3f} {marker}")

print(f"\nStrongest positive correlations with fitness (BAD - higher value = worse fitness):")
bad = corr_all.nlargest(5)
for p, v in bad.items():
    print(f"  {p}: r={v:+.3f}")

print(f"\nStrongest negative correlations with fitness (GOOD - higher value = better fitness):")
good = corr_all.nsmallest(5)
for p, v in good.items():
    print(f"  {p}: r={v:+.3f}")

print(f"\n--- 5. NEW EXP10 PARAMETERS ---")
for p in NEW_PARAMS:
    lo, hi = BOUNDS[p]
    rng = hi - lo
    row = conv_df[conv_df['param'] == p].iloc[0]
    corr_val = corr_all[p]

    best_val = row['best']
    mean_val = row['last20_mean']
    default_val = DEFAULTS[p]

    # Distance from default as % of range
    shift_from_default = (best_val - default_val) / rng * 100

    print(f"\n  {p}:")
    print(f"    Bounds: [{lo}, {hi}], Default: {default_val}, Best: {best_val:.4f}")
    print(f"    Last 20 mean: {mean_val:.4f} +/- {row['last20_std']:.4f}")
    print(f"    Shift from default: {shift_from_default:+.1f}% of range")
    print(f"    Fitness correlation: r={corr_val:+.3f}")
    print(f"    Status: {row['status']}")

print(f"\n--- 6. BEST CONFIG SUMMARY ---")
print(f"Best evaluation: #{int(best_row['eval'])}")
print(f"Best fitness: {best_row['fitness']:.2f}")

# Estimate survival and quality
# fitness = -(survival_ticks * (1 + 0.2*quality))
# At best fitness, if hit cap: survival_ticks = 3M, quality = (-fitness/3M - 1) / 0.2
best_fitness = best_row['fitness']
if -best_fitness >= MAX_TICKS:
    implied_quality = (-best_fitness / MAX_TICKS - 1.0) / 0.2
    implied_quality = min(max(implied_quality, 0), 1)
    print(f"Implied: hit 3M tick cap with quality ~{implied_quality:.2f}")
else:
    approx_ticks = -best_fitness / 1.1
    print(f"Implied: survived ~{approx_ticks:.0f} ticks (~{approx_ticks*DT:.0f} sim-seconds)")

print(f"\nBest parameter values:")
for p in param_cols:
    lo, hi = BOUNDS[p]
    val = best_row[p]
    norm = (val - lo) / (hi - lo) * 100
    print(f"  {p:<25} = {val:>10.4f}  ({norm:>5.1f}% of range)")

print("\n" + "="*80)
print("PLOTS SAVED:")
print(f"  {OUT}/convergence.png")
print(f"  {OUT}/survival.png")
print(f"  {OUT}/parameter_convergence.png")
print(f"  {OUT}/correlations.png")
print(f"  {OUT}/new_params.png")
print("="*80)
