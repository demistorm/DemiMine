export interface Point {
	x: number;
	y: number;
}

export interface ServerPosition extends Point {
	id: string;
}

export function pointsToPolylineString(points: Point[]): string {
	return points.map(p => `${p.x},${p.y}`).join(' ');
}

/**
 * Simple L-shaped path, horizontal-first.
 * Used when there is only one server.
 */
export function generateManhattanPath(
	x1: number,
	y1: number,
	x2: number,
	y2: number
): Point[] {
	if (y1 === y2 || x1 === x2) {
		return [{ x: x1, y: y1 }, { x: x2, y: y2 }];
	}
	return [{ x: x1, y: y1 }, { x: x2, y: y1 }, { x: x2, y: y2 }];
}

// ─── Public entry point ──────────────────────────────────────────────────────

/**
 * Produces a set of polylines implementing a multi-cluster bus topology:
 *
 *   Proxy ────┬────────────────┬─────── …
 *             │                │
 *          cluster A        cluster B
 *           ┌─┴─┐            ┌─┼─┐
 *           A   B            C D  E
 *
 * Servers are grouped into spatial clusters so that naturally-separate
 * groups each get their own spine rather than being forced onto one spine.
 * Works in all four orientations automatically.
 */
export function computeClusteredConnections(
	proxyPos: Point,
	serverPositions: ServerPosition[]
): Point[][] {
	if (serverPositions.length === 0) return [];
	if (serverPositions.length === 1) {
		return [generateManhattanPath(proxyPos.x, proxyPos.y, serverPositions[0].x, serverPositions[0].y)];
	}

	// 1. Decide primary layout axis based on overall server spread
	const xs = serverPositions.map(s => s.x);
	const ys = serverPositions.map(s => s.y);
	const xSpread = Math.max(...xs) - Math.min(...xs);
	const ySpread = Math.max(...ys) - Math.min(...ys);
	const horizontal = xSpread >= ySpread; // trunk runs horizontally

	// 2. Cluster servers along the primary axis
	const clusters = clusterServers(serverPositions, horizontal);

	// 3. Build per-cluster spines + a single trunk connecting them all
	return buildBus(proxyPos, clusters, horizontal);
}

// ─── Clustering ──────────────────────────────────────────────────────────────

/**
 * Groups servers by their position along the primary axis using a
 * gap-based 1-D clustering algorithm. A new cluster starts whenever
 * the gap between consecutive servers exceeds GAP_THRESHOLD.
 *
 * horizontal layout → cluster by X
 * vertical layout   → cluster by Y
 */
function clusterServers(
	servers: ServerPosition[],
	horizontal: boolean
): ServerPosition[][] {
	const GAP_THRESHOLD = 0; // px — adjust to taste

	const sorted = [...servers].sort((a, b) =>
	horizontal ? a.x - b.x : a.y - b.y
	);

	const clusters: ServerPosition[][] = [[sorted[0]]];
	for (let i = 1; i < sorted.length; i++) {
		const prev = sorted[i - 1];
		const curr = sorted[i];
		const gap = horizontal ? curr.x - prev.x : curr.y - prev.y;
		if (gap > GAP_THRESHOLD) {
			clusters.push([curr]);
		} else {
			clusters[clusters.length - 1].push(curr);
		}
	}
	return clusters;
}

// ─── Bus builder ─────────────────────────────────────────────────────────────

function buildBus(
	proxyPos: Point,
	clusters: ServerPosition[][],
	horizontal: boolean
): Point[][] {
	const polylines: Point[][] = [];

	if (horizontal) {
		// Trunk runs horizontally at trunkY.
		// Spines are vertical, one per cluster.
		const trunkY = weightedMidpoint(
			proxyPos.y,
			avg(clusters.flat().map(s => s.y))
		);

		const allX = clusters.flat().map(s => s.x);
		const serverMinX = Math.min(...allX);
		const serverMaxX = Math.max(...allX);

		// Trunk end: extend to cover all clusters
		const trunkFarX = proxyPos.x <= serverMinX ? serverMaxX : serverMinX;

		// Trunk polyline (proxy → trunk height → far end)
		if (Math.abs(proxyPos.y - trunkY) < 1) {
			polylines.push([
				{ x: proxyPos.x, y: trunkY },
				{ x: trunkFarX, y: trunkY }
			]);
		} else {
			polylines.push([
				{ x: proxyPos.x, y: proxyPos.y },
				{ x: proxyPos.x, y: trunkY },
				{ x: trunkFarX, y: trunkY }
			]);
		}

		for (const cluster of clusters) {
			const cys = cluster.map(s => s.y);
			const cxs = cluster.map(s => s.x);
			const minY = Math.min(...cys);
			const maxY = Math.max(...cys);
			const spineX = avg(cxs);

			if (cluster.length === 1) {
				// Single server: vertical stub from trunk down/up to server
				const s = cluster[0];
				if (Math.abs(s.y - trunkY) > 1) {
					polylines.push([{ x: s.x, y: trunkY }, { x: s.x, y: s.y }]);
				}
				continue;
			}

			// Vertical connector from trunk to the spine's nearest end
			const spineNearY = clamp(trunkY, minY, maxY);
			if (Math.abs(trunkY - spineNearY) > 1 || Math.abs(spineX - spineX) > 1) {
				polylines.push([
					{ x: spineX, y: trunkY },
				   { x: spineX, y: spineNearY }
				]);
			}

			// Vertical spine: full height of cluster
			polylines.push([
				{ x: spineX, y: minY },
				{ x: spineX, y: maxY }
			]);

			// Horizontal branches: spine → each server
			for (const s of cluster) {
				if (Math.abs(s.x - spineX) > 1) {
					polylines.push([{ x: spineX, y: s.y }, { x: s.x, y: s.y }]);
				}
			}
		}
	} else {
		// Trunk runs vertically at trunkX.
		// Spines are horizontal, one per cluster.
		const trunkX = weightedMidpoint(
			proxyPos.x,
			avg(clusters.flat().map(s => s.x))
		);

		const allY = clusters.flat().map(s => s.y);
		const serverMinY = Math.min(...allY);
		const serverMaxY = Math.max(...allY);

		const trunkFarY = proxyPos.y <= serverMinY ? serverMaxY : serverMinY;

		// Trunk polyline (proxy → trunk X → far end)
		if (Math.abs(proxyPos.x - trunkX) < 1) {
			polylines.push([
				{ x: trunkX, y: proxyPos.y },
				{ x: trunkX, y: trunkFarY }
			]);
		} else {
			polylines.push([
				{ x: proxyPos.x, y: proxyPos.y },
				{ x: trunkX, y: proxyPos.y },
				{ x: trunkX, y: trunkFarY }
			]);
		}

		for (const cluster of clusters) {
			const cxs = cluster.map(s => s.x);
			const cys = cluster.map(s => s.y);
			const minX = Math.min(...cxs);
			const maxX = Math.max(...cxs);
			const spineY = avg(cys);

			if (cluster.length === 1) {
				const s = cluster[0];
				if (Math.abs(s.x - trunkX) > 1) {
					polylines.push([{ x: trunkX, y: s.y }, { x: s.x, y: s.y }]);
				}
				continue;
			}

			// Horizontal connector from trunk to spine's nearest end
			const spineNearX = clamp(trunkX, minX, maxX);
			if (Math.abs(trunkX - spineNearX) > 1) {
				polylines.push([
					{ x: trunkX, y: spineY },
				   { x: spineNearX, y: spineY }
				]);
			}

			// Horizontal spine: full width of cluster
			polylines.push([
				{ x: minX, y: spineY },
				{ x: maxX, y: spineY }
			]);

			// Vertical branches: spine → each server
			for (const s of cluster) {
				if (Math.abs(s.y - spineY) > 1) {
					polylines.push([{ x: s.x, y: spineY }, { x: s.x, y: s.y }]);
				}
			}
		}
	}

	return polylines;
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

/** Bias the trunk toward the server mass (2:1 servers vs proxy). */
function weightedMidpoint(proxyVal: number, serverAvg: number): number {
	return proxyVal * 0.33 + serverAvg * 0.67;
}

function avg(values: number[]): number {
	return values.reduce((a, b) => a + b, 0) / values.length;
}

function clamp(value: number, min: number, max: number): number {
	return Math.max(min, Math.min(max, value));
}
