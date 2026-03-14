const API_BASE = import.meta.env.VITE_API_URL || '';

interface ApiError {
	error: string;
	message?: string;
}

class ApiClient {
	private baseUrl: string;
	private token: string | null = null;

	constructor(baseUrl: string) {
		this.baseUrl = baseUrl;
		if (typeof window !== 'undefined') {
			this.token = localStorage.getItem('token');
		}
	}

	setToken(token: string | null) {
		this.token = token;
		if (typeof window !== 'undefined') {
			if (token) {
				localStorage.setItem('token', token);
			} else {
				localStorage.removeItem('token');
			}
		}
	}

	private async request<T>(
		endpoint: string,
		options: RequestInit = {}
	): Promise<T> {
		const url = `${this.baseUrl}${endpoint}`;
		
		const headers: HeadersInit = {
			'Content-Type': 'application/json',
			...options.headers,
		};

		if (this.token) {
			(headers as Record<string, string>)['Authorization'] = `Bearer ${this.token}`;
		}

		const response = await fetch(url, {
			...options,
			headers,
		});

		if (response.status === 401) {
			this.setToken(null);
			if (typeof window !== 'undefined') {
				window.location.replace('/login');
			}
			return new Promise(() => {});
		}

		if (!response.ok) {
			const error: ApiError = await response.json().catch(() => ({ 
				error: 'unknown_error',
				message: response.statusText 
			}));
			throw new Error(error.message || error.error);
		}

		if (response.status === 204) {
			return {} as T;
		}

		return response.json();
	}

	async get<T>(endpoint: string): Promise<T> {
		return this.request<T>(endpoint);
	}

	async put<T>(endpoint: string, data: unknown): Promise<T> {
		return this.request<T>(endpoint, {
			method: 'PUT',
			body: JSON.stringify(data),
		});
	}

	async post<T>(endpoint: string, data?: unknown): Promise<T> {
		return this.request<T>(endpoint, {
			method: 'POST',
			body: data ? JSON.stringify(data) : undefined,
		});
	}

	async patch<T>(endpoint: string, data: unknown): Promise<T> {
		return this.request<T>(endpoint, {
			method: 'PATCH',
			body: JSON.stringify(data),
		});
	}

	async delete<T>(endpoint: string): Promise<T> {
		return this.request<T>(endpoint, {
			method: 'DELETE',
		});
	}

	async upload<T>(endpoint: string, file: File): Promise<T> {
		const formData = new FormData();
		formData.append('file', file);

		const url = `${this.baseUrl}${endpoint}`;
		const headers: HeadersInit = {};
		
		if (this.token) {
			headers['Authorization'] = `Bearer ${this.token}`;
		}

		const response = await fetch(url, {
			method: 'POST',
			headers,
			body: formData,
		});

		if (response.status === 401) {
			this.setToken(null);
			if (typeof window !== 'undefined') {
				window.location.replace('/login');
			}
			return new Promise(() => {});
		}

		if (!response.ok) {
			const error = await response.json().catch(() => ({ error: 'Upload failed' }));
			throw new Error(error.error || 'Upload failed');
		}

		return response.json();
	}

	async uploadIcon(serverId: number, file: File): Promise<{ success: boolean }> {
		const formData = new FormData();
		formData.append('icon', file);

		const url = `${this.baseUrl}/api/servers/${serverId}/icon`;
		const headers: HeadersInit = {};
		
		if (this.token) {
			headers['Authorization'] = `Bearer ${this.token}`;
		}

		const response = await fetch(url, {
			method: 'POST',
			headers,
			body: formData,
		});

		if (response.status === 401) {
			this.setToken(null);
			if (typeof window !== 'undefined') {
				window.location.replace('/login');
			}
			return new Promise(() => {});
		}

		if (!response.ok) {
			const error = await response.json().catch(() => ({ error: 'Upload failed' }));
			throw new Error(error.error || 'Upload failed');
		}

		return response.json();
	}

	async deleteIcon(serverId: number): Promise<{ success: boolean }> {
		return this.delete<{ success: boolean }>(`/api/servers/${serverId}/icon`);
	}

	async uploadProxyIcon(proxyId: number, file: File): Promise<{ success: boolean }> {
		const formData = new FormData();
		formData.append('icon', file);

		const url = `${this.baseUrl}/api/proxies/${proxyId}/icon`;
		const headers: HeadersInit = {};
		
		if (this.token) {
			headers['Authorization'] = `Bearer ${this.token}`;
		}

		const response = await fetch(url, {
			method: 'POST',
			headers,
			body: formData,
		});

		if (response.status === 401) {
			this.setToken(null);
			if (typeof window !== 'undefined') {
				window.location.replace('/login');
			}
			return new Promise(() => {});
		}

		if (!response.ok) {
			const error = await response.json().catch(() => ({ error: 'Upload failed' }));
			throw new Error(error.error || 'Upload failed');
		}

		return response.json();
	}

	async deleteProxyIcon(proxyId: number): Promise<{ success: boolean }> {
		return this.delete<{ success: boolean }>(`/api/proxies/${proxyId}/icon`);
	}
}

export function getToken(): string | null {
	if (typeof window !== 'undefined') {
		return localStorage.getItem('token');
	}
	return null;
}

export const api = new ApiClient(API_BASE);

export interface Server {
	id: number;
	name: string;
	type: string;
	version: string;
	proxy_id: number | null;
	proxy_name: string | null;
	ram_mb: number;
	domain: string | null;
	host_port: number | null;
	status: string;
	player_count: number;
	canvas_x: number;
	canvas_y: number;
	icon_path: string | null;
	backup_interval_days: number;
	auto_shutdown_minutes: number;
	scheduled_start: string | null;
	scheduled_stop: string | null;
	created_at: string;
}

export interface Proxy {
	id: number;
	name: string;
	host_port: number;
	ram_mb: number;
	forwarding_secret: string;
	status: string;
	player_count: number;
	connected_servers: string[];
	canvas_x: number;
	canvas_y: number;
	icon_path: string | null;
	created_at: string;
}

export interface AuthStatus {
	setup_complete: boolean;
	logged_in: boolean;
}

export interface LoginResponse {
	token: string;
	expires_at: string;
}

export interface ModrinthSearchResult {
	hits: ModrinthProjectHit[];
	offset: number;
	limit: number;
	total_hits: number;
}

export interface ModrinthProjectHit {
	project_id: string;
	slug: string;
	title: string;
	description: string;
	categories: string[];
	project_type: string;
	downloads: number;
	follows: number;
	icon_url: string;
	date_created: string;
	date_modified: string;
	latest_version: string;
	license: string;
	loaders: string[];
	game_versions: string[];
}

export interface ModrinthProject {
	id: string;
	slug: string;
	title: string;
	description: string;
	categories: string[];
	body: string;
	project_type: string;
	downloads: number;
	followers: number;
	icon_url: string;
	date_created: string;
	date_modified: string;
	license: { id: string; name: string; url: string };
	gallery: { url: string; featured: boolean; title: string }[];
}

export interface ModrinthVersion {
	id: string;
	project_id: string;
	name: string;
	version_number: string;
	changelog: string;
	game_versions: string[];
	loaders: string[];
	dependencies: ModrinthDependency[];
	files: ModrinthVersionFile[];
	date_created: string;
	featured: boolean;
	version_type: string;
}

export interface ModrinthDependency {
	version_id: string;
	project_id: string;
	dependency_type: string;
}

export interface ModrinthVersionFile {
	hashes: { sha1: string; sha512: string };
	url: string;
	filename: string;
	primary: boolean;
	size: number;
}

export interface InstalledPlugin {
	id: number;
	target_type: string;
	target_id: number;
	project_id: string;
	project_slug: string;
	project_name: string;
	version_id: string;
	version_number: string;
	filename: string;
	file_hash: string;
	installed_at: string;
	dependencies?: InstalledPlugin[];
}

export interface PluginUpdateResult {
	plugin: InstalledPlugin;
	latest_version: string;
	current_version: string;
	has_update: boolean;
}

export interface InstallPluginRequest {
	project_id: string;
	version_id?: string;
	game_version?: string;
}

export const modrinthApi = {
	search: (params: {
		query?: string;
		limit?: number;
		offset?: number;
		loaders?: string[];
		game_version?: string;
	}) => {
		const searchParams = new URLSearchParams();
		if (params.query) searchParams.set('query', params.query);
		if (params.limit) searchParams.set('limit', params.limit.toString());
		if (params.offset) searchParams.set('offset', params.offset.toString());
		if (params.loaders?.length) searchParams.set('loaders', params.loaders.join(','));
		if (params.game_version) searchParams.set('game_version', params.game_version);
		return api.get<ModrinthSearchResult>(`/api/modrinth/search?${searchParams}`);
	},

	getProject: (slug: string) => 
		api.get<ModrinthProject>(`/api/modrinth/project/${slug}`),

	getVersions: (slug: string, gameVersions?: string[], loaders?: string[]) => {
		const params = new URLSearchParams();
		if (gameVersions?.length) params.set('game_versions', JSON.stringify(gameVersions));
		if (loaders?.length) params.set('loaders', JSON.stringify(loaders));
		const query = params.toString();
		return api.get<ModrinthVersion[]>(`/api/modrinth/project/${slug}/versions${query ? '?' + query : ''}`);
	},
};

export const pluginApi = {
	getInstalled: (type: 'server' | 'proxy', id: number) =>
		api.get<InstalledPlugin[]>(`/api/plugins/${type}/${id}`),

	install: (type: 'server' | 'proxy', id: number, data: InstallPluginRequest) =>
		api.post<InstalledPlugin>(`/api/plugins/${type}/${id}/install`, data),

	uninstall: (type: 'server' | 'proxy', id: number, projectId: string) =>
		api.delete<{ success: boolean }>(`/api/plugins/${type}/${id}/${projectId}`),

	checkUpdates: (type: 'server' | 'proxy', id: number) =>
		api.get<PluginUpdateResult[]>(`/api/plugins/${type}/${id}/updates`),

	update: (type: 'server' | 'proxy', id: number, projectId: string, gameVersion?: string) =>
		api.post<InstalledPlugin>(`/api/plugins/${type}/${id}/${projectId}/update`, { game_version: gameVersion }),
};

export const settingsApi = {
	get: () =>
		api.get<{ [key: string]: string }>('/api/settings'),

	update: (data: { proxy_mc_version: string }) =>
		api.put<{ success: boolean }>('/api/settings', data),
};
