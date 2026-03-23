import { writable } from 'svelte/store';

interface SparkStats {
	tps: number;
	memoryUsed: number;
	memoryMax: number;
	cpu: number;
	mspt?: number;
}

export const serverSparkStats = writable<Map<number, SparkStats>>(new Map());
export const proxySparkStats = writable<Map<number, SparkStats>>(new Map());

export function updateServerStats(serverId: number, stats: Partial<SparkStats>) {
	serverSparkStats.update(current => {
		const existing = current.get(serverId) || { tps: 20, memoryUsed: 0, memoryMax: 0, cpu: 0 };
		const updated = { ...existing, ...stats };
		current.set(serverId, updated);
		return current;
	});
}

export function updateProxyStats(proxyId: number, stats: Partial<SparkStats>) {
	proxySparkStats.update(current => {
		const existing = current.get(proxyId) || { memoryUsed: 0, memoryMax: 0, cpu: 0 };
		const updated = { ...existing, ...stats };
		current.set(proxyId, updated);
		return current;
	});
}

export function getServerStats(serverId: number): SparkStats | null {
	let stats: SparkStats | null = null;
	serverSparkStats.subscribe(current => {
		stats = current.get(serverId) || null;
	})();
	return stats;
}

export function getProxyStats(proxyId: number): SparkStats | null {
	let stats: SparkStats | null = null;
	proxySparkStats.subscribe(current => {
		stats = current.get(proxyId) || null;
	})();
	return stats;
}

export const serverSparkErrors = writable<Map<number, string>>(new Map());
export const proxySparkErrors = writable<Map<number, string>>(new Map());

export function setServerError(serverId: number, error: string) {
	serverSparkErrors.update(current => {
		const updated = new Map(current);
		updated.set(serverId, error);
		return updated;
	});
}

export function setProxyError(proxyId: number, error: string) {
	proxySparkErrors.update(current => {
		const updated = new Map(current);
		updated.set(proxyId, error);
		return updated;
	});
}

export function clearServerError(serverId: number) {
	serverSparkErrors.update(current => {
		const updated = new Map(current);
		updated.delete(serverId);
		return updated;
	});
}

export function clearProxyError(proxyId: number) {
	proxySparkErrors.update(current => {
		const updated = new Map(current);
		updated.delete(proxyId);
		return updated;
	});
}

export function getServerError(serverId: number): string | null {
	let error: string | null = null;
	serverSparkErrors.subscribe(current => {
		error = current.get(serverId) || null;
	})();
	return error;
}

export function getProxyError(proxyId: number): string | null {
	let error: string | null = null;
	proxySparkErrors.subscribe(current => {
		error = current.get(proxyId) || null;
	})();
	return error;
}
