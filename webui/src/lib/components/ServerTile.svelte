<script lang="ts">
	import type { Server } from '$lib/api';
	import { createEventDispatcher, onDestroy } from 'svelte';

	export let server: Server;
	export let canvasOffset = { x: 0, y: 0 };
	export let zoom = 1;
	export let pixelScale = 4;
	export let textureUrl: string | null = null;

	const dispatch = createEventDispatcher();

	let isDragging = false;
	let dragStart = { x: 0, y: 0 };
	let currentX = server.canvas_x ?? 0;
	let currentY = server.canvas_y ?? 0;

	let longPressTimer: ReturnType<typeof setTimeout> | null = null;
	let touchStartPos = { x: 0, y: 0 };
	let touchDragging = false;
	let touchMoved = false;

	$: if (!isDragging && !touchDragging && server.canvas_x != null) currentX = server.canvas_x;
	$: if (!isDragging && !touchDragging && server.canvas_y != null) currentY = server.canvas_y;

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

        e.preventDefault();
        const newX = (e.clientX - canvasOffset.x) / zoom - dragStart.x;
        const newY = (e.clientY - canvasOffset.y) / zoom - dragStart.y;

        currentX = Math.round(newX / 100) * 100;
        currentY = Math.round(newY / 100) * 100;

        dispatch('dragging', { id: server.id, x: currentX, y: currentY, type: 'server' });
    }

	function handleMouseUp(e: MouseEvent) {
        if (isDragging) {
            dispatch('move', { x: currentX, y: currentY });
        }
        isDragging = false;
        window.removeEventListener('mousemove', handleMouseMove);
        window.removeEventListener('mouseup', handleMouseUp);
    }

	function handleTouchStart(e: TouchEvent) {
		if (e.touches.length !== 1) return;
		const touch = e.touches[0];
		e.preventDefault();

		touchStartPos = { x: touch.clientX, y: touch.clientY };
		touchMoved = false;
		touchDragging = false;

		longPressTimer = setTimeout(() => {
			touchDragging = true;
			dragStart = {
				x: (touchStartPos.x - canvasOffset.x) / zoom - currentX,
				y: (touchStartPos.y - canvasOffset.y) / zoom - currentY
			};
			window.addEventListener('touchmove', handleTouchMove);
			window.addEventListener('touchend', handleTouchEnd);
		}, 500);

		window.addEventListener('touchmove', handleTouchMoveEarly);
		window.addEventListener('touchend', handleTouchEndEarly);
	}

	function handleTouchMoveEarly(e: TouchEvent) {
		if (e.touches.length !== 1) return;
		const touch = e.touches[0];
		const dx = touch.clientX - touchStartPos.x;
		const dy = touch.clientY - touchStartPos.y;

		if (Math.sqrt(dx * dx + dy * dy) > 10) {
			touchMoved = true;
			if (longPressTimer) {
				clearTimeout(longPressTimer);
				longPressTimer = null;
			}
			window.removeEventListener('touchmove', handleTouchMoveEarly);
			window.removeEventListener('touchend', handleTouchEndEarly);
		}
	}

	function handleTouchEndEarly() {
		if (longPressTimer) {
			clearTimeout(longPressTimer);
			longPressTimer = null;
		}
		if (!touchMoved && !touchDragging) {
			dispatch('click');
		}
		window.removeEventListener('touchmove', handleTouchMoveEarly);
		window.removeEventListener('touchend', handleTouchEndEarly);
	}

	function handleTouchMove(e: TouchEvent) {
		if (!touchDragging || e.touches.length !== 1) return;
		e.preventDefault();
		const touch = e.touches[0];
		const newX = (touch.clientX - canvasOffset.x) / zoom - dragStart.x;
		const newY = (touch.clientY - canvasOffset.y) / zoom - dragStart.y;

		currentX = Math.round(newX / 100) * 100;
		currentY = Math.round(newY / 100) * 100;

		dispatch('dragging', { id: server.id, x: currentX, y: currentY, type: 'server' });
	}

	function handleTouchEnd() {
		if (touchDragging) {
			dispatch('move', { x: currentX, y: currentY });
		}
		touchDragging = false;
		window.removeEventListener('touchmove', handleTouchMove);
		window.removeEventListener('touchend', handleTouchEnd);
	}

	onDestroy(() => {
        window.removeEventListener('mousemove', handleMouseMove);
        window.removeEventListener('mouseup', handleMouseUp);
		if (longPressTimer) clearTimeout(longPressTimer);
		window.removeEventListener('touchmove', handleTouchMove);
		window.removeEventListener('touchend', handleTouchEnd);
		window.removeEventListener('touchmove', handleTouchMoveEarly);
		window.removeEventListener('touchend', handleTouchEndEarly);
    });

	function getStatusColor(status: string) {
        switch (status) {
            case 'running':
                return 'var(--success)';
            case 'stopped':
                return 'var(--error)';
            default:
                return 'var(--text-secondary)';
        }
    }

	function getStatusText(status: string) {
        switch (status) {
            case 'running':
                return 'Online';
            case 'stopped':
                return 'Offline';
            case 'starting':
                return 'Starting';
            case 'crashed':
                return 'Crashed';
            default:
                return status;
        }
    }
</script>

<svelte:head>
    <title>{server.name} - DemiMine</title>
</svelte:head>

<div
    class="server-tile"
    class:dragging={isDragging}
    style="left: {currentX + 50}px; top: {currentY + 50}px; transform: translate(-50%, -50%); --scale: {pixelScale}px; {textureUrl ? `background-image: url('${textureUrl}');` : ''}"
    on:mousedown={handleMouseDown}
    on:touchstart={handleTouchStart}
    role="button"
    tabindex={0}
>
    <div class="server-icon">
        {#if server.icon_path}
            <img src={server.icon_path} alt={server.name} />
        {:else}
            <div class="icon-placeholder">
                Icon
            </div>
        {/if}
    </div>
    
    <div class="server-info">
        <div class="server-name">{server.name}</div>
        <div class="server-status">
            <span class="status-dot" style="background-color: {getStatusColor(server.status)}"></span>
            <span class="status-text">{getStatusText(server.status)}</span>
            {#if server.status === 'running'}
                <span class="player-count">({server.player_count})</span>
            {/if}
        </div>
    </div>
</div>

<style>
    .server-tile {
        position: absolute;
        width: 120px;
        padding: 1rem;
        background-color: var(--bg-secondary);
        border: 5px solid var(--border);
        border-radius: 0;
        cursor: pointer;
        transition: transform 0.2s, box-shadow 0.2s;
        user-select: none;
        touch-action: none;
        image-rendering: pixelated;
        background-repeat: repeat;
        box-shadow:
            calc(var(--scale) * -1) 0 0 rgba(0, 0, 0, 0.15),
            calc(var(--scale) * 1) 0 0 rgba(0, 0, 0, 0.15),
            calc(var(--scale) * -1) calc(var(--scale) * 1) 0 rgba(0, 0, 0, 0.25),
            calc(var(--scale) * 1) calc(var(--scale) * 1) 0 rgba(0, 0, 0, 0.25),
            0 calc(var(--scale) * 1) 0 rgba(0, 0, 0, 0.30);
    }

    .server-tile:hover {
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

    .server-tile.dragging {
        opacity: 0.7;
        cursor: move;
        z-index: 10;
        transform: translate(-50%, -50%);
    }

    .server-tile:active:not(.dragging) {
        transform: translate(-50%, -50%) scale(0.97);
    }

    .server-icon {
        width: 64px;
        height: 64px;
        margin: 0 auto 0.5rem;
        display: flex;
        align-items: center;
        justify-content: center;
    }

    .server-icon img {
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

    .server-info {
        display: flex;
        flex-direction: column;
        align-items: center;
        margin-top: 0.5rem;
    }

    .server-name {
        font-weight: 600;
        color: var(--text-primary);
        margin-bottom: 0.25rem;
        white-space: nowrap;
    }

    .server-status {
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

    .player-count {
        color: var(--text-secondary);
        font-size: 0.875rem;
    }
</style>
