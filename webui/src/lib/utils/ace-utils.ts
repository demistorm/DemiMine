export function getAceMode(filename: string): string | null {
	const ext = filename.split('.').pop()?.toLowerCase();

	switch (ext) {
		case 'json':
			return 'json';
		case 'yml':
		case 'yaml':
			return 'yaml';
		case 'properties':
			return 'properties';
		default:
			return null;
	}
}
