<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { servers, proxies, loadServers, loadProxies, updateServerPosition, updateServerStatus, deleteServerFromStore, updateProxyPosition, deleteProxyFromStore } from '$lib/stores/servers';
	import { dragPositions } from '$lib/stores/dragPositions';
	import { backgroundTextureUrl, serverTileTextureUrl, proxyTileTextureUrl, globalSettings, loadBackgroundTexture, loadServerTileTexture, loadProxyTileTexture, loadGlobalSettings } from '$lib/stores/settings';
	import { api, type Server, type Proxy } from '$lib/api';
	import ServerTile from '$lib/components/ServerTile.svelte';
	import ProxyTile from '$lib/components/ProxyTile.svelte';
	import CreateServer from '$lib/components/CreateServer.svelte';
	import CreateProxy from '$lib/components/CreateProxy.svelte';
	import { generateManhattanPath, pointsToPolylineString, computeClusteredConnections, type ServerPosition } from '$lib/utils/manhattanPath';

	let canvasOffset = { x: 0, y: 0 };
	let zoom = 0.6;
	let isDragging = false;
	let dragStart = { x: 0, y: 0 };
	let showCreateModal = false;
	let showCreateProxyModal = false;
	let contextMenuServer: Server | null = null;
	let contextMenuProxy: Proxy | null = null;
	let contextMenuPos = { x: 0, y: 0 };
	let deleteStep = 0;
	let ignoreNextClick = false;
	let scaledBackgroundTextureUrl: string | null = null;
	let scaledServerTileTextureUrl: string | null = null;
	let scaledProxyTileTextureUrl: string | null = null;
	let touchStartDistance = 0;
	let touchStartZoom = 1;
	let lastTouchMidpoint = { x: 0, y: 0 };
	let activeTouches = 0;

	$: serverList = $servers || [];
	$: proxyList = $proxies || [];
	$: globalSettings, backgroundTextureUrl, serverTileTextureUrl, proxyTileTextureUrl;

	$: if ($backgroundTextureUrl && $globalSettings.background_texture_scale) {
		scaleTexture($backgroundTextureUrl, $globalSettings.background_texture_scale);
	} else {
		scaledBackgroundTextureUrl = null;
	}

	$: if ($serverTileTextureUrl && $globalSettings.background_texture_scale) {
		scaleServerTileTexture($serverTileTextureUrl, $globalSettings.background_texture_scale);
	} else {
		scaledServerTileTextureUrl = null;
	}

	$: if ($proxyTileTextureUrl && $globalSettings.background_texture_scale) {
		scaleProxyTileTexture($proxyTileTextureUrl, $globalSettings.background_texture_scale);
	} else {
		scaledProxyTileTextureUrl = null;
	}

	onMount(() => {
		loadGlobalSettings();
		loadServers();
		loadProxies();
		loadBackgroundTexture();
		loadServerTileTexture();
		loadProxyTileTexture();
		const navbarHeight = 56;
		const canvasSize = 8000;
		canvasOffset = {
			x: window.innerWidth / 2 - (canvasSize / 2) * zoom,
			y: (window.innerHeight - navbarHeight) / 2 - (canvasSize / 2) * zoom
		};
	});

	function scaleTexture(url: string, scale: number) {
		const img = new Image();
		img.onload = () => {
			const canvas = document.createElement('canvas');
			const ctx = canvas.getContext('2d');
			if (!ctx) return;

			const scaledSize = 16 * scale;
			canvas.width = scaledSize;
			canvas.height = scaledSize;

			ctx.imageSmoothingEnabled = false;
			ctx.drawImage(img, 0, 0, scaledSize, scaledSize);

			scaledBackgroundTextureUrl = canvas.toDataURL('image/png');
		};
		img.src = url;
	}

	function scaleServerTileTexture(url: string, scale: number) {
		const img = new Image();
		img.onload = () => {
			const canvas = document.createElement('canvas');
			const ctx = canvas.getContext('2d');
			if (!ctx) return;

			const scaledSize = 16 * scale;
			canvas.width = scaledSize;
			canvas.height = scaledSize;

			ctx.imageSmoothingEnabled = false;
			ctx.drawImage(img, 0, 0, scaledSize, scaledSize);

			scaledServerTileTextureUrl = canvas.toDataURL('image/png');
		};
		img.src = url;
	}

	function scaleProxyTileTexture(url: string, scale: number) {
		const img = new Image();
		img.onload = () => {
			const canvas = document.createElement('canvas');
			const ctx = canvas.getContext('2d');
			if (!ctx) return;

			const scaledSize = 16 * scale;
			canvas.width = scaledSize;
			canvas.height = scaledSize;

			ctx.imageSmoothingEnabled = false;
			ctx.drawImage(img, 0, 0, scaledSize, scaledSize);

			scaledProxyTileTextureUrl = canvas.toDataURL('image/png');
		};
		img.src = url;
	}

	function handleWheel(e: WheelEvent) {
		e.preventDefault();
		
        const mouseX = (e.clientX - canvasOffset.x) / zoom;
        const mouseY = (e.clientY - canvasOffset.y) / zoom;
        
        const delta = e.deltaY > 0 ? -0.1 : 0.1;
        const newZoom = Math.max(0.1, Math.min(4.0, zoom + delta));
        
        canvasOffset = {
            x: e.clientX - mouseX * newZoom,
            y: e.clientY - mouseY * newZoom
        };
        zoom = newZoom;
    }

    function handleMouseDown(e: MouseEvent) {
        const target = e.target as HTMLElement;
        const isInsideTile = target.closest('.server-tile, .proxy-tile');
        
        if (isInsideTile) {
            return;
        }
        isDragging = true;
        dragStart = { x: e.clientX - canvasOffset.x, y: e.clientY - canvasOffset.y };
        target.style.cursor = 'grabbing';
    }

    function handleMouseMove(e: MouseEvent) {
        if (!isDragging) {
            return;
        }
        canvasOffset = { x: e.clientX - dragStart.x, y: e.clientY - dragStart.y };
    }

    function handleMouseUp(e: MouseEvent) {
        isDragging = false;
        (e.target as HTMLElement).style.cursor = 'grab';
    }

	function getTouchDistance(touches: TouchList): number {
		const dx = touches[0].clientX - touches[1].clientX;
		const dy = touches[0].clientY - touches[1].clientY;
		return Math.sqrt(dx * dx + dy * dy);
	}

	function getTouchMidpoint(touches: TouchList): { x: number; y: number } {
		return {
			x: (touches[0].clientX + touches[1].clientX) / 2,
			y: (touches[0].clientY + touches[1].clientY) / 2
		};
	}

	function handleTouchStart(e: TouchEvent) {
		if (e.touches.length === 2) {
			e.preventDefault();
			touchStartDistance = getTouchDistance(e.touches);
			touchStartZoom = zoom;
			lastTouchMidpoint = getTouchMidpoint(e.touches);
			activeTouches = 2;
		}
	}

	function handleTouchMove(e: TouchEvent) {
		if (e.touches.length < 2) {
			activeTouches = 0;
			return;
		}

		e.preventDefault();
		const currentDistance = getTouchDistance(e.touches);
		const currentMidpoint = getTouchMidpoint(e.touches);

		if (touchStartDistance > 0) {
			const rawScale = currentDistance / touchStartDistance;
			const scale = 1 + (rawScale - 1) * 0.7;
			const newZoom = Math.max(0.1, Math.min(4.0, touchStartZoom * scale));

			const beforeX = (currentMidpoint.x - canvasOffset.x) / zoom;
			const beforeY = (currentMidpoint.y - canvasOffset.y) / zoom;

			canvasOffset = {
				x: currentMidpoint.x - beforeX * newZoom,
				y: currentMidpoint.y - beforeY * newZoom
			};
			zoom = newZoom;
		}

		const dx = currentMidpoint.x - lastTouchMidpoint.x;
		const dy = currentMidpoint.y - lastTouchMidpoint.y;
		canvasOffset = { x: canvasOffset.x + dx, y: canvasOffset.y + dy };

		lastTouchMidpoint = currentMidpoint;
	}

	function handleTouchEnd(e: TouchEvent) {
		if (e.touches.length < 2) {
			activeTouches = 0;
			touchStartDistance = 0;
		}
	}

    function handleServerClick(server: Server) {
        goto(`/servers/${server.id}`);
    }

    function handleProxyClick(proxy: Proxy) {
        goto(`/proxies/${proxy.id}`);
    }

    async function handleServerMove(server: Server, newX: number, newY: number) {
        await updateServerPosition(server.id, newX, newY);
        dragPositions.clearPosition(server.id);
    }

    async function handleProxyMove(proxy: Proxy, newX: number, newY: number) {
        await updateProxyPosition(proxy.id, newX, newY);
        dragPositions.clearPosition(proxy.id);
    }

    async function handleServerStart(server: Server) {
        try {
            await api.post(`/api/servers/${server.id}/start`);
            updateServerStatus(server.id, 'running');
        } catch (err) {
            console.error('Failed to start server:', err);
        }
    }

    async function handleServerStop(server: Server) {
        try {
            await api.post(`/api/servers/${server.id}/stop`);
            updateServerStatus(server.id, 'stopped');
        } catch (err) {
            console.error('Failed to stop server:', err);
        }
    }

    async function handleServerDelete(server: Server) {
        try {
            await api.delete(`/api/servers/${server.id}`);
            deleteServerFromStore(server.id);
        } catch (err) {
            console.error('Failed to delete server:', err);
        }
    }

    async function handleProxyStart(proxy: Proxy) {
        try {
            await api.post(`/api/proxies/${proxy.id}/start`);
        } catch (err) {
            console.error('Failed to start proxy:', err);
        }
    }

    async function handleProxyStop(proxy: Proxy) {
        try {
            await api.post(`/api/proxies/${proxy.id}/stop`);
        } catch (err) {
            console.error('Failed to stop proxy:', err);
        }
    }

    async function handleProxyDelete(proxy: Proxy) {
        try {
            await api.delete(`/api/proxies/${proxy.id}`);
            deleteProxyFromStore(proxy.id);
        } catch (err) {
            console.error('Failed to delete proxy:', err);
        }
    }

    function handleServerContextMenu(server: Server, x: number, y: number) {
        contextMenuServer = server;
        contextMenuProxy = null;
        contextMenuPos = { x, y };
        deleteStep = 0;
        ignoreNextClick = true;
    }

    function handleProxyContextMenu(proxy: Proxy, x: number, y: number) {
        contextMenuProxy = proxy;
        contextMenuServer = null;
        contextMenuPos = { x, y };
        deleteStep = 0;
        ignoreNextClick = true;
    }

    function closeContextMenu() {
        contextMenuServer = null;
        contextMenuProxy = null;
        deleteStep = 0;
    }

    function handleServerContextMenuAction(action: 'start' | 'stop' | 'delete') {
        if (action === 'delete') {
            if (deleteStep === 0) {
                deleteStep = 1;
                return;
            } else if (deleteStep === 1) {
                deleteStep = 2;
                return;
            }
        }
        if (contextMenuServer) {
            if (action === 'start') handleServerStart(contextMenuServer);
            if (action === 'stop') handleServerStop(contextMenuServer);
            if (action === 'delete') handleServerDelete(contextMenuServer);
        }
        closeContextMenu();
    }

    function handleProxyContextMenuAction(action: 'start' | 'stop' | 'delete') {
        if (action === 'delete') {
            if (deleteStep === 0) {
                deleteStep = 1;
                return;
            } else if (deleteStep === 1) {
                deleteStep = 2;
                return;
            }
        }
        if (contextMenuProxy) {
            if (action === 'start') handleProxyStart(contextMenuProxy);
            if (action === 'stop') handleProxyStop(contextMenuProxy);
            if (action === 'delete') handleProxyDelete(contextMenuProxy);
        }
        closeContextMenu();
    }

    function handleGlobalClick(e: MouseEvent) {
        if (ignoreNextClick) {
            ignoreNextClick = false;
            return;
        }
        if ((contextMenuServer || contextMenuProxy) && !(e.target as HTMLElement).closest('.context-menu')) {
            closeContextMenu();
        }
    }

    function handleGlobalKeydown(e: KeyboardEvent) {
        if (e.key === 'Escape') closeContextMenu();
    }

    function getCanvasTransform() {
        return `translate(${canvasOffset.x}px, ${canvasOffset.y}px) scale(${zoom})`;
    }

	$: connections = computeConnections(proxyList, serverList, $dragPositions);

    function computeConnections(proxies: Proxy[], servers: Server[], livePos: typeof $dragPositions) {
        return proxies.flatMap(proxy => {
            const proxyServers = servers.filter(s => s.proxy_id === proxy.id);

			if (proxyServers.length === 0) return [];

			const pPos = livePos[proxy.id] ?? { x: proxy.canvas_x ?? 0, y: proxy.canvas_y ?? 0 };
			const serverPositions: ServerPosition[] = proxyServers.map(server => ({
				id: server.id,
				x: (livePos[server.id]?.x ?? server.canvas_x ?? 0) + 50,
				y: (livePos[server.id]?.y ?? server.canvas_y ?? 0) + 50
			}));

			const proxyCenter = { x: pPos.x + 50, y: pPos.y + 50 };
			const polylines = computeClusteredConnections(proxyCenter, serverPositions);

			return polylines.map(points => ({ points }));
        });
    }

    function handleDragging(e: CustomEvent) {
        const { id, x, y, type } = e.detail;
        dragPositions.updatePosition(id, x, y, type);
    }

    function clearDragging(e: CustomEvent) {
        const { id } = e.detail;
        dragPositions.clearPosition(id);
    }
</script>

<svelte:window on:click={handleGlobalClick} on:keydown={handleGlobalKeydown} />

<svelte:head>
    <title>Servers - DemiMine</title>
</svelte:head>

<main class="canvas-container" 
    on:wheel={handleWheel}
    on:mousedown={handleMouseDown}
    on:mousemove={handleMouseMove}
    on:mouseup={handleMouseUp}
    on:mouseleave={handleMouseUp}
    on:touchstart={handleTouchStart}
    on:touchmove={handleTouchMove}
    on:touchend={handleTouchEnd}
>
    <div class="canvas" style="transform: translate({canvasOffset.x}px, {canvasOffset.y}px) scale({zoom})">
        <div class="grid-background" style="background-image: {scaledBackgroundTextureUrl ? `url(${scaledBackgroundTextureUrl})` : 'none'};"></div>
        
        <svg class="connection-lines">
            {#each connections as conn}
                <polyline
                    points={pointsToPolylineString(conn.points)}
                    fill="none"
                    stroke="#676767"
                    stroke-width="8"
                />
            {/each}
        </svg>

        {#each proxyList as proxy}
            <ProxyTile
                {proxy}
                {canvasOffset}
                {zoom}
                pixelScale={$globalSettings.background_texture_scale}
                textureUrl={scaledProxyTileTextureUrl}
                on:click={() => handleProxyClick(proxy)}
                on:dragging={handleDragging}
                on:move={(e) => handleProxyMove(proxy, e.detail.x, e.detail.y)}
                on:contextmenu={(e) => handleProxyContextMenu(proxy, e.detail.x, e.detail.y)}
            />
        {/each}

        {#each serverList as server}
            <ServerTile
                {server}
                {canvasOffset}
                {zoom}
                pixelScale={$globalSettings.background_texture_scale}
                textureUrl={scaledServerTileTextureUrl}
                on:click={() => handleServerClick(server)}
                on:dragging={handleDragging}
                on:move={(e) => handleServerMove(server, e.detail.x, e.detail.y)}
                on:contextmenu={(e) => handleServerContextMenu(server, e.detail.x, e.detail.y)}
            />
        {/each}
    </div>

    <div class="create-buttons">
        <button class="create-btn" style="--scale: {$globalSettings.background_texture_scale}px;" on:click={() => showCreateModal = true}>
            Create Server
        </button>
        <button class="create-btn proxy" style="--scale: {$globalSettings.background_texture_scale}px;" on:click={() => showCreateProxyModal = true}>
            Create Proxy
        </button>
    </div>
</main>

<CreateServer bind:show={showCreateModal} />
<CreateProxy bind:show={showCreateProxyModal} />

{#if contextMenuServer}
    <div 
        class="context-menu" 
        style="left: {contextMenuPos.x}px; top: {contextMenuPos.y}px;"
    >
        {#if contextMenuServer.status === 'running'}
            <button class="menu-item" on:click={() => handleServerContextMenuAction('stop')}>
                Stop
            </button>
        {:else}
            <button class="menu-item" on:click={() => handleServerContextMenuAction('start')}>
                Start
            </button>
        {/if}
        <button 
            class="menu-item danger" 
            class:confirm={deleteStep > 0}
            class:final-warning={deleteStep === 2}
            on:click={() => handleServerContextMenuAction('delete')}
        >
            {#if deleteStep === 0}
                Delete
            {:else if deleteStep === 1}
                Are you sure?
            {:else}
                FINAL WARNING
            {/if}
        </button>
    </div>
{/if}

{#if contextMenuProxy}
    <div 
        class="context-menu" 
        style="left: {contextMenuPos.x}px; top: {contextMenuPos.y}px;"
    >
        {#if contextMenuProxy.status === 'running'}
            <button class="menu-item" on:click={() => handleProxyContextMenuAction('stop')}>
                Stop
            </button>
        {:else}
            <button class="menu-item" on:click={() => handleProxyContextMenuAction('start')}>
                Start
            </button>
        {/if}
        <button 
            class="menu-item danger" 
            class:confirm={deleteStep > 0}
            class:final-warning={deleteStep === 2}
            on:click={() => handleProxyContextMenuAction('delete')}
        >
            {#if deleteStep === 0}
                Delete
            {:else if deleteStep === 1}
                Are you sure?
            {:else}
                FINAL WARNING
            {/if}
        </button>
    </div>
{/if}

<style>
    .canvas-container {
        position: fixed;
        top: 56px;
        left: 0;
        right: 0;
        bottom: 0;
        overflow: hidden;
        cursor: grab;
        background: var(--bg-primary);
        touch-action: none;
    }

    .canvas {
        position: absolute;
        width: 8000px;
        height: 8000px;
        transform-origin: 0 0;
        z-index: 1;
    }

    .grid-background {
        position: absolute;
        top: 0;
        left: 0;
        width: 8000px;
        height: 8000px;
        background-image: none;
        background-size: auto;
        background-repeat: repeat;
        image-rendering: pixelated;
        pointer-events: none;
    }

    .connection-lines {
        position: absolute;
        top: 0;
        left: 0;
        width: 8000px;
        height: 8000px;
        pointer-events: none;
        z-index: 0;
    }

    .create-buttons {
        position: fixed;
        left: 24px;
        top: 80px;
        display: flex;
        flex-direction: column;
        gap: 0.5rem;
        z-index: 10;
    }

    .create-btn {
        @apply bg-accent hover:bg-accent-hover text-text-primary;
        padding: 0.6rem 1.2rem;
        font-size: 0.875rem;
        font-weight: 600;
        border-radius: 0;
        transition: transform 0.2s, box-shadow 0.2s;
        box-shadow:
            calc(var(--scale) * -1) 0 0 rgba(0, 0, 0, 0.15),
            calc(var(--scale) * 1) 0 0 rgba(0, 0, 0, 0.15),
            calc(var(--scale) * -1) calc(var(--scale) * 1) 0 rgba(0, 0, 0, 0.25),
            calc(var(--scale) * 1) calc(var(--scale) * 1) 0 rgba(0, 0, 0, 0.25),
            0 calc(var(--scale) * 1) 0 rgba(0, 0, 0, 0.30);
    }

    .create-btn:hover {
        transform: translateY(-2px);
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

    .create-btn.proxy {
        background-color: var(--bg-tertiary);
        border: 4px solid var(--border);
    }

    .create-btn.proxy:hover {
        background-color: var(--bg-secondary);
    }

    .context-menu {
        position: fixed;
        background-color: var(--bg-secondary);
        border: 3px solid var(--border);
        border-radius: 0;
        padding: 0.25rem;
        z-index: 1000;
        min-width: 140px;
        box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
    }

    .menu-item {
        display: block;
        width: 100%;
        padding: 0.5rem 0.75rem;
        background: transparent;
        border: none;
        color: var(--text-primary);
        text-align: left;
        cursor: pointer;
        border-radius: 0;
        font-size: 0.875rem;
        transition: background-color 0.15s;
    }

    .menu-item:hover {
        background-color: var(--bg-tertiary);
    }

    .menu-item.danger {
        color: var(--error);
    }

    .menu-item.danger:hover {
        background-color: rgba(239, 68, 68, 0.1);
    }

    .menu-item.confirm {
        font-weight: 500;
    }

    .menu-item.final-warning {
        font-weight: 600;
    }
</style>
