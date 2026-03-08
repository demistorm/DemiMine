import * as universal from '../entries/pages/_layout.ts.js';

export const index = 0;
let component_cache;
export const component = async () => component_cache ??= (await import('../entries/pages/_layout.svelte.js')).default;
export { universal };
export const universal_id = "src/routes/+layout.ts";
export const imports = ["_app/immutable/nodes/0.CnKUohKb.js","_app/immutable/chunks/B7-SqECT.js","_app/immutable/chunks/BuriRYb3.js","_app/immutable/chunks/BZmh2sS6.js","_app/immutable/chunks/i8q5GtTH.js","_app/immutable/chunks/Bpp5H_z9.js"];
export const stylesheets = ["_app/immutable/assets/0.Bfoqg-hU.css"];
export const fonts = [];
