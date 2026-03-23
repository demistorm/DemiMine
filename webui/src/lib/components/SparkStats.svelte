<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	
	export let stats: { tps?: number; memoryUsed?: number; memoryMax?: number; cpu?: number } | null = null;
	export let isProxy = false;
	export let error: string | null = null;
	
	let history: number[] = [];
	const maxHistory = 30;
	
	onMount(() => {
		if (stats?.memoryUsed !== undefined) {
			history = [stats.memoryUsed];
		}
	});
	
	$: if (stats?.memoryUsed !== undefined) {
		history = [...history, stats.memoryUsed].slice(-maxHistory);
	}
	
	function getTPSColor(tps: number): string {
		if (tps >= 18) return 'var(--success)';
		if (tps >= 15) return 'var(--warning)';
		return 'var(--error)';
	}
	
	function getSparklinePath(): string {
		if (history.length < 2) return '';
		const max = Math.max(...history, 1);
		const min = Math.min(...history, 0);
		const range = max - min || 1;
		const height = 40;
		const width = 100;
		const step = width / (maxHistory - 1);
		
		return history.map((val, i) => {
			const x = i * step;
			const y = height - ((val - min) / range) * height;
			return `${i === 0 ? 'M' : 'L'} ${x} ${y}`;
		}).join(' ');
	}
 </script>
 
<div class="spark-stats">
	{#if error}
		<div class="error-indicator" title={error}>
			<span class="warning-icon">⚠️</span>
		</div>
	{:else if stats}
		<div class="ram-display">
			<svg class="sparkline" viewBox="0 0 100 40" preserveAspectRatio="none">
				<path d={getSparklinePath()} />
			</svg>
			<span class="ram-value">{stats.memoryUsed}MB</span>
		</div>
		{#if !isProxy && stats.tps !== undefined}
			<span class="tps" style:color={getTPSColor(stats.tps)}>
				{stats.tps.toFixed(1)} TPS
			</span>
		{:else}
			<span class="stats-unavailable">--</span>
		{/if}
	{:else}
		<span class="stats-unavailable">--</span>
	{/if}
</div>
 
<style>
	.spark-stats {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}
	
	.error-indicator {
		display: flex;
		align-items: center;
	}
	
	.warning-icon {
		color: var(--error);
		font-size: 0.9rem;
		cursor: help;
	}
	
	.ram-display {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	
	.sparkline {
		width: 60px;
		height: 24px;
	}
	
	.sparkline path {
		fill: none;
		stroke: var(--accent);
		stroke-width: 2;
		vector-effect: non-scaling-stroke;
	}
	
	.ram-value {
		font-size: 0.8rem;
		font-weight: 500;
		color: var(--text-primary);
		min-width: 60px;
	}
	
	.tps {
		font-size: 0.8rem;
		font-weight: 500;
		min-width: 70px;
	}
	
	.stats-unavailable {
		font-size: 0.8rem;
		color: var(--text-secondary);
	}
 </style>
