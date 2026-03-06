import { redirect } from '@sveltejs/kit';
import { browser } from '$app/environment';

export function load({ url }) {
	const token = browser ? localStorage.getItem('token') : null;
	
	// If not logged in and not on login page, redirect to login
	if (!token && url.pathname !== '/login') {
		throw redirect(307, '/login');
	}
	
	// If logged in and on login page, redirect to home
	if (token && url.pathname === '/login') {
		throw redirect(307, '/');
	}
	
	return {
		token: token
	};
}
