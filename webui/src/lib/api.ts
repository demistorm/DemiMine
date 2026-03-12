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
			throw new Error('Upload failed');
		}

		return response.json();
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
