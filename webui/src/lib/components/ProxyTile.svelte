<script lang="ts">
	import type { Proxy } from '$lib/api';
	import { createEventDispatcher, onDestroy } from 'svelte';

	export let proxy: Proxy;
	export let canvasOffset = { x: 0, y: 0 };
	export let zoom = 1;
	export let pixelScale = 4;
	export let textureUrl: string | null = null;

	const dispatch = createEventDispatcher();

	let isDragging = false;
	let dragStart = { x: 0, y: 0 };
	let currentX = proxy.canvas_x ?? 0;
	let currentY = proxy.canvas_y ?? 0;

	$: if (!isDragging && proxy.canvas_x != null) currentX = proxy.canvas_x;
	$: if (!isDragging && proxy.canvas_y != null) currentY = proxy.canvas_y;

	function handleMouseDown(e: MouseEvent) {
		if (e.ctrlKey && e.button === 0) {
			e.preventDefault();
			e.stopPropagation();
			dispatch('contextmenu', { x: e.clientX, y: e.clientY });
			return;
		}
		
		if (e.button !== 0) return;
        if (!e.shiftKey) {
            dispatch('click');
            return;
        }

        e.preventDefault();
        e.stopPropagation();
        isDragging = true;
        dragStart = {
            x: (e.clientX - canvasOffset.x) / zoom - currentX,
            y: (e.clientY - canvasOffset.y) / zoom - currentY
        };
        window.addEventListener('mousemove', handleMouseMove);
        window.addEventListener('mouseup', handleMouseUp);
	}

	function handleMouseMove(e: MouseEvent) {
		if (!isDragging) return;
		const newX = (e.clientX - canvasOffset.x) / zoom - dragStart.x;
		const newY = (e.clientY - canvasOffset.y) / zoom - dragStart.y;
		currentX = Math.round(newX / 100) * 100;
		currentY = Math.round(newY / 100) * 100;

		dispatch('dragging', { id: proxy.id, x: currentX, y: currentY, type: 'proxy' });
	}

	function handleMouseUp(e: MouseEvent) {
		if (!isDragging) return;
		isDragging = false;
		window.removeEventListener('mousemove', handleMouseMove);
		window.removeEventListener('mouseup', handleMouseUp);
		dispatch('move', { x: Math.round(currentX), y: Math.round(currentY) });
	}

	function handleContextMenu(e: MouseEvent) {
		e.preventDefault();
		dispatch('contextmenu', { x: e.clientX, y: e.clientY });
	}

	function getStatusColor(status: string): string {
		switch (status) {
			case 'running':
				return '#48bb78';
			case 'starting':
				return '#ed8936';
			case 'crashed':
				return '#f56565';
			default:
				return '#718096';
		}
	}

	function getStatusText(status: string): string {
		switch (status) {
			case 'running':
				return 'Online';
			case 'starting':
				return 'Starting';
			case 'crashed':
				return 'Crashed';
			default:
				return 'Offline';
		}
	}
</script>

<div
    class="proxy-tile"
    style="left: {currentX + 50}px; top: {currentY + 50}px; transform: translate(-50%, -50%); --scale: {pixelScale}px; {textureUrl ? `background-image: url('${textureUrl}');` : ''}"
    on:mousedown={handleMouseDown}
    on:contextmenu={handleContextMenu}
>
	<div class="proxy-icon">
		{#if proxy.icon_path}
			<img src={proxy.icon_path} alt={proxy.name} />
		{:else}
			<div class="icon-placeholder">
				<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
					<rect x="2" y="2" width="20" height="8" rx="2" ry="2"></rect>
					<rect x="2" y="14" width="20" height="8" rx="2" ry="2"></rect>
					<line x1="6" y1="6" x2="6.01" y2="6"></line>
					<line x1="6" y1="18" x2="6.01" y2="18"></line>
				</svg>
			</div>
		{/if}
	</div>
    
	<div class="proxy-info">
		<div class="proxy-name">{proxy.name}</div>
		<div class="proxy-status">
			<span class="status-dot" style="background-color: {getStatusColor(proxy.status)}"></span>
			<span class="status-text">{getStatusText(proxy.status)}</span>
		</div>
		<div class="server-count">
			{proxy.connected_servers?.length || 0} server{proxy.connected_servers?.length !== 1 ? 's' : ''}
		</div>
		<div class="player-count">
			{proxy.player_count || 0} player{proxy.player_count !== 1 ? 's' : ''}
		</div>
	</div>
</div>

<style>
	.proxy-tile {
		position: absolute;
		width: 120px;
		padding: 1rem;
		background-color: #4a5568;
		border-radius: 0;
		cursor: pointer;
		user-select: none;
		transition: transform 0.2s, box-shadow 0.2s;
		border: 6px solid var(--border);
		image-rendering: pixelated;
		background-repeat: repeat;
		box-shadow:
			calc(var(--scale) * -1) 0 0 rgba(0, 0, 0, 0.15),
			calc(var(--scale) * 1) 0 0 rgba(0, 0, 0, 0.15),
			calc(var(--scale) * -1) calc(var(--scale) * 1) 0 rgba(0, 0, 0, 0.25),
			calc(var(--scale) * 1) calc(var(--scale) * 1) 0 rgba(0, 0, 0, 0.25),
			0 calc(var(--scale) * 1) 0 rgba(0, 0, 0, 0.30);
	}

	.proxy-tile:hover {
		transform: translate(-50%, calc(-50% - 4px)) scale(1.02) !important;
		box-shadow:
			0 calc(var(--scale) * -1) 0 rgba(0, 0, 0, 0.10),
			calc(var(--scale) * -1) 0 0 rgba(0, 0, 0, 0.19),
			calc(var(--scale) * 1) 0 0 rgba(0, 0, 0, 0.19),
			calc(var(--scale) * -1) calc(var(--scale) * 1) 0 rgba(0, 0, 0, 0.26),
			calc(var(--scale) * 1) calc(var(--scale) * 1) 0 rgba(0, 0, 0, 0.26),
			0 calc(var(--scale) * 1) 0 rgba(0, 0, 0, 0.30),
			calc(var(--scale) * -2) 0 0 rgba(0, 0, 0, 0.14),
			calc(var(--scale) * 2) 0 0 rgba(0, 0, 0, 0.14),
			calc(var(--scale) * -2) calc(var(--scale) * 2) 0 rgba(0, 0, 0, 0.19),
			calc(var(--scale) * 2) calc(var(--scale) * 2) 0 rgba(0, 0, 0, 0.19),
			0 calc(var(--scale) * 2) 0 rgba(0, 0, 0, 0.23),
			calc(var(--scale) * -3) 0 0 rgba(0, 0, 0, 0.08),
			calc(var(--scale) * 3) 0 0 rgba(0, 0, 0, 0.08),
			calc(var(--scale) * -3) calc(var(--scale) * 3) 0 rgba(0, 0, 0, 0.11),
			calc(var(--scale) * 3) calc(var(--scale) * 3) 0 rgba(0, 0, 0, 0.11),
			0 calc(var(--scale) * 3) 0 rgba(0, 0, 0, 0.15);
	}

	.proxy-icon {
		width: 64px;
		height: 64px;
		margin: 0 auto 0.5rem;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.proxy-icon img {
		width: 100%;
		height: 100%;
		object-fit: contain;
	}

	.icon-placeholder {
		background-color: var(--bg-tertiary);
		color: var(--text-secondary);
		width: 100%;
		height: 100%;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 0.25rem;
	}

	.proxy-info {
		display: flex;
		flex-direction: column;
		align-items: center;
		margin-top: 0.5rem;
	}

	.proxy-name {
		font-weight: 600;
		color: var(--text-primary);
		margin-bottom: 0.25rem;
		white-space: nowrap;
	}

	.proxy-status {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
		font-size: 0.875rem;
	}

	.status-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
	}

	.status-text {
		color: var(--text-secondary);
		font-weight: 500;
	}

	.server-count {
		color: var(--text-secondary);
		font-size: 0.75rem;
		margin-top: 0.25rem;
	}

	.player-count {
		color: var(--text-secondary);
		font-size: 0.875rem;
	}
</style>
