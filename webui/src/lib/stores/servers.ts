import { writable } from 'svelte/store';
import type { Server, Proxy } from '$lib/api';
import { api } from '$lib/api';

export const servers = writable<Server[]>([]);
export const proxies = writable<Proxy[]>([]);
export const isAuthenticated = writable<boolean>(false);
export const canvasZoom = writable<number>(1);
export const canvasPan = writable<{ x: number; y: number }>({ x: 0, y: 0 });

export async function loadServers() {
	try {
		const data = await api.get<Server[]>('/api/servers');
		servers.set(data);
	} catch (error) {
		console.error('Failed to load servers:', error);
	}
}

export async function loadProxies() {
	try {
		const data = await api.get<Proxy[]>('/api/proxies');
		proxies.set(data);
	} catch (error) {
		console.error('Failed to load proxies:', error);
	}
}

export function updateServerPosition(serverId: number, x: number, y: number) {
	servers.update(serversList => 
		serversList.map(s => 
			s.id === serverId ? { ...s, canvas_x: x, canvas_y: y } : s
		)
	);
}

export function updateServerStatus(serverId: number, status: string, playerCount?: number) {
	servers.update(serversList =>
		serversList.map(s => 
			s.id === serverId ? { 
				...s, 
				status,
				player_count: playerCount !== undefined ? playerCount : s.player_count
			} : s
		)
	);
}
