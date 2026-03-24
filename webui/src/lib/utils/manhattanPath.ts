export interface Point {
	x: number;
	y: number;
}

export interface ServerPosition extends Point {
	id: string;
}

/**
 * Node half-dimensions used for port placement.
 * Pass your actual tile size. Defaults match a 120×80 card.
 */
export interface NodeSize {
	halfW: number;
	halfH: number;
}

const DEFAULT_NODE: NodeSize = { halfW: 60, halfH: 40 };

// ─────────────────────────────────────────────────────────────────────────────
//  Public helpers
// ─────────────────────────────────────────────────────────────────────────────

export function pointsToPolylineString(points: Point[]): string {
	return points.map(p => `${p.x},${p.y}`).join(' ');
}

/**
 * Two-point Manhattan elbow.
 * Exits from the port on the side of origin that faces the target.
 * Enters the target on the port facing the origin.
 * One 90° turn maximum.
 */
export function generateManhattanPath(
	x1: number, y1: number,
	x2: number, y2: number,
	originSize: NodeSize = DEFAULT_NODE,
	targetSize: NodeSize = DEFAULT_NODE,
): Point[] {
	const { exitPort, entryPort } = facingPorts(
		{ x: x1, y: y1 }, { x: x2, y: y2 },
		originSize, targetSize,
	);

	if (exitPort.x === entryPort.x || exitPort.y === entryPort.y) {
		return [exitPort, entryPort];
	}

	const horiz = Math.abs(x2 - x1) >= Math.abs(y2 - y1);
	return horiz
	? [exitPort, { x: entryPort.x, y: exitPort.y  }, entryPort]
	: [exitPort, { x: exitPort.x,  y: entryPort.y }, entryPort];
}

// ─────────────────────────────────────────────────────────────────────────────
//  Main entry point
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Builds an orthogonal bus tree from one proxy node to N server nodes.
 *
 * Direction is determined automatically by comparing the proxy position
 * to the server cloud centroid:
 *   |dx| >= |dy|  →  horizontal tree  (proxy left or right)
 *   |dy| >  |dx|  →  vertical tree    (proxy above or below)
 *
 * Port rule: lines always exit/enter from the side facing the other node.
 *
 * @param clusterGapThreshold  Gap (px) along the secondary axis that splits
 *                             servers into separate spine clusters.
 *                             Set to ~50% of your tile pitch. Default 150.
 */
export function computeClusteredConnections(
	proxyPos: Point,
	serverPositions: ServerPosition[],
	proxySize: NodeSize = DEFAULT_NODE,
	serverSize: NodeSize = DEFAULT_NODE,
	clusterGapThreshold = 150,
): Point[][] {
	if (serverPositions.length === 0) return [];

	if (serverPositions.length === 1) {
		return [generateManhattanPath(
			proxyPos.x, proxyPos.y,
			serverPositions[0].x, serverPositions[0].y,
			proxySize, serverSize,
		)];
	}

	const cx = avg(serverPositions.map(s => s.x));
	const cy = avg(serverPositions.map(s => s.y));
	const dx = cx - proxyPos.x;
	const dy = cy - proxyPos.y;
	const horizontal = Math.abs(dx) >= Math.abs(dy);

	const clusters = spatialCluster(serverPositions, horizontal, clusterGapThreshold);

	return horizontal
	? buildHTree(proxyPos, clusters, proxySize, serverSize)
	: buildVTree(proxyPos, clusters, proxySize, serverSize);
}

// ─────────────────────────────────────────────────────────────────────────────
//  Port helpers
// ─────────────────────────────────────────────────────────────────────────────

type Side = 'left' | 'right' | 'top' | 'bottom';

function oppositeSide(s: Side): Side {
	const map: Record<Side, Side> = { left: 'right', right: 'left', top: 'bottom', bottom: 'top' };
	return map[s];
}

function portPoint(center: Point, size: NodeSize, side: Side): Point {
	switch (side) {
		case 'left':   return { x: center.x - size.halfW, y: center.y };
		case 'right':  return { x: center.x + size.halfW, y: center.y };
		case 'top':    return { x: center.x, y: center.y - size.halfH };
		case 'bottom': return { x: center.x, y: center.y + size.halfH };
	}
}

/** Dominant-axis side from origin toward target. */
function exitSideFor(origin: Point, target: Point): Side {
	const dx = target.x - origin.x;
	const dy = target.y - origin.y;
	if (Math.abs(dx) >= Math.abs(dy)) return dx >= 0 ? 'right' : 'left';
	return dy >= 0 ? 'bottom' : 'top';
}

function facingPorts(
	origin: Point, target: Point,
	originSize: NodeSize, targetSize: NodeSize,
): { exitPort: Point; entryPort: Point } {
	const side = exitSideFor(origin, target);
	return {
		exitPort:  portPoint(origin, originSize, side),
		entryPort: portPoint(target, targetSize, oppositeSide(side)),
	};
}

// ─────────────────────────────────────────────────────────────────────────────
//  Clustering
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Groups servers by proximity along the secondary axis.
 * Horizontal tree → cluster by X (servers at similar X share a spine).
 * Vertical tree   → cluster by Y (servers at similar Y share a spine).
 */
function spatialCluster(
	servers: ServerPosition[],
	horizontal: boolean,
	gapThreshold: number,
): ServerPosition[][] {
	const key = (s: ServerPosition) => horizontal ? s.x : s.y;
	const sorted = [...servers].sort((a, b) => key(a) - key(b));
	const clusters: ServerPosition[][] = [[sorted[0]]];
	for (let i = 1; i < sorted.length; i++) {
		if (key(sorted[i]) - key(sorted[i - 1]) > gapThreshold) {
			clusters.push([sorted[i]]);
		} else {
			clusters[clusters.length - 1].push(sorted[i]);
		}
	}
	return clusters;
}

// ─────────────────────────────────────────────────────────────────────────────
//  Horizontal tree
//
//  Proxy exits RIGHT (if left of cloud) or LEFT (if right of cloud).
//  Trunk  = horizontal line at median Y of all servers.
//  Spines = vertical lines at the median X of each cluster.
//  Stubs  = horizontal lines from spine to each server's entry port.
// ─────────────────────────────────────────────────────────────────────────────
const padding = 80; // or whatever looks good


function buildHTree(
	proxy: Point,
	clusters: ServerPosition[][],
	proxySize: NodeSize,
	serverSize: NodeSize,
): Point[][] {
	const lines: Point[][] = [];
	const allServers = clusters.flat();

	// Trunk level = median Y across all servers
	const trunkY = median(allServers.map(s => s.y));
	// Proxy side
	const proxyIsLeft   = proxy.x < avg(allServers.map(s => s.x));
	const proxySide     = proxyIsLeft ? 'right' as Side : 'left' as Side;
	const serverSide    = oppositeSide(proxySide);

	// Spine X per cluster = median X of servers in that cluster
	const spineXs = clusters.map(cluster => {
		const xs = cluster.map(s => s.x);

		if (proxyIsLeft) {
			// spine must be LEFT of all servers
			return Math.min(...xs) - serverSize.halfW - padding;
		} else {
			// spine must be RIGHT of all servers
			return Math.max(...xs) + serverSize.halfW + padding;
		}
	});
	const trunkLeft  = Math.min(...spineXs);
	const trunkRight = Math.max(...spineXs);

	const trunkAttachX  = proxyIsLeft ? trunkLeft : trunkRight;

	// ── Proxy exit port → trunk attach point ─────────────────────────────
	const proxyExit = portPoint(proxy, proxySize, proxySide);
	if (Math.abs(proxyExit.y - trunkY) < 1) {
		// Proxy already at trunk level — straight horizontal
		lines.push([proxyExit, { x: trunkAttachX, y: trunkY }]);
	} else {
		// Elbow: go horizontal to attach X, then vertical to trunkY
		lines.push([
			proxyExit,
			 { x: trunkAttachX, y: proxyExit.y },
			 { x: trunkAttachX, y: trunkY      },
		]);
	}

	// ── Trunk backbone ────────────────────────────────────────────────────
	if (trunkLeft < trunkRight) {
		lines.push([
			{ x: trunkLeft,  y: trunkY },
			 { x: trunkRight, y: trunkY },
		]);
	}

	// ── Per-cluster spines + stubs ────────────────────────────────────────
	for (let ci = 0; ci < clusters.length; ci++) {
		const cluster = clusters[ci];
		const spineX  = spineXs[ci];

		if (cluster.length === 1) {
			const s         = cluster[0];
			const entryPort = portPoint(s, serverSize, serverSide);
			// Single server: vertical stub from trunkY to server Y, then horizontal stub to entry port
			if (Math.abs(s.y - trunkY) < 1) {
				// On same level as trunk — the trunk line already reaches spineX;
				// just draw a horizontal stub to the entry port if needed
				if (Math.abs(entryPort.x - spineX) > 1) {
					lines.push([{ x: spineX, y: trunkY }, entryPort]);
				}
			} else {
				lines.push([
					{ x: spineX,      y: trunkY      },
			   { x: spineX,      y: s.y         },
			   { x: entryPort.x, y: s.y         },
				]);
			}
			continue;
		}

		// Multi-server: vertical spine spanning from trunkY to outermost server Y
		const serverYs = cluster.map(s => s.y);
		const spineMin = Math.min(trunkY, ...serverYs);
		const spineMax = Math.max(trunkY, ...serverYs);
		lines.push([
			{ x: spineX, y: spineMin },
			 { x: spineX, y: spineMax },
		]);

		// Horizontal stub: spine → server entry port
		for (const s of cluster) {
			const entryPort = portPoint(s, serverSize, serverSide);
			if (Math.abs(entryPort.x - spineX) > 1) {
				lines.push([
					{ x: spineX,      y: s.y },
			   { x: entryPort.x, y: s.y },
				]);
			}
			// If spineX === entryPort.x the spine itself meets the port — no stub needed
		}
	}

	return lines;
}

// ─────────────────────────────────────────────────────────────────────────────
//  Vertical tree
//
//  Proxy exits BOTTOM (if above cloud) or TOP (if below cloud).
//  Trunk  = vertical line at median X of all servers.
//  Spines = horizontal lines at the median Y of each cluster.
//  Stubs  = vertical lines from spine to each server's entry port.
// ─────────────────────────────────────────────────────────────────────────────

function buildVTree(
	proxy: Point,
	clusters: ServerPosition[][],
	proxySize: NodeSize,
	serverSize: NodeSize,
): Point[][] {
	const lines: Point[][] = [];
	const allServers = clusters.flat();

	const trunkX = median(allServers.map(s => s.x));
	const proxyIsAbove  = proxy.y < avg(allServers.map(s => s.y));
	const proxySide     = proxyIsAbove ? 'bottom' as Side : 'top' as Side;
	const serverSide    = oppositeSide(proxySide);

	const spineYs = clusters.map(cluster => {
		const ys = cluster.map(s => s.y);

		if (proxyIsAbove) {
			// spine ABOVE all servers
			return Math.min(...ys) - serverSize.halfH - padding;
		} else {
			// spine BELOW all servers
			return Math.max(...ys) + serverSize.halfH + padding;
		}
	});
	const trunkTop    = Math.min(...spineYs);
	const trunkBottom = Math.max(...spineYs);

	const trunkAttachY  = proxyIsAbove ? trunkTop : trunkBottom;

	// ── Proxy exit port → trunk attach point ─────────────────────────────
	const proxyExit = portPoint(proxy, proxySize, proxySide);
	if (Math.abs(proxyExit.x - trunkX) < 1) {
		lines.push([proxyExit, { x: trunkX, y: trunkAttachY }]);
	} else {
		lines.push([
			proxyExit,
			 { x: proxyExit.x, y: trunkAttachY },
			 { x: trunkX,      y: trunkAttachY },
		]);
	}

	// ── Trunk backbone ────────────────────────────────────────────────────
	if (trunkTop < trunkBottom) {
		lines.push([
			{ x: trunkX, y: trunkTop    },
			 { x: trunkX, y: trunkBottom },
		]);
	}

	// ── Per-cluster spines + stubs ────────────────────────────────────────
	for (let ci = 0; ci < clusters.length; ci++) {
		const cluster = clusters[ci];
		const spineY  = spineYs[ci];

		if (cluster.length === 1) {
			const s         = cluster[0];
			const entryPort = portPoint(s, serverSize, serverSide);
			if (Math.abs(s.x - trunkX) < 1) {
				if (Math.abs(entryPort.y - spineY) > 1) {
					lines.push([{ x: trunkX, y: spineY }, entryPort]);
				}
			} else {
				lines.push([
					{ x: trunkX,      y: spineY      },
			   { x: s.x,         y: spineY      },
			   { x: s.x,         y: entryPort.y },
				]);
			}
			continue;
		}

		// Horizontal spine spanning from trunkX to outermost server X
		const serverXs = cluster.map(s => s.x);
		const spineMin = Math.min(trunkX, ...serverXs);
		const spineMax = Math.max(trunkX, ...serverXs);
		lines.push([
			{ x: spineMin, y: spineY },
			 { x: spineMax, y: spineY },
		]);

		// Vertical stub: spine → server entry port
		for (const s of cluster) {
			const entryPort = portPoint(s, serverSize, serverSide);
			if (Math.abs(entryPort.y - spineY) > 1) {
				lines.push([
					{ x: s.x,         y: spineY       },
			   { x: s.x,         y: entryPort.y  },
				]);
			}
		}
	}

	return lines;
}

// ─────────────────────────────────────────────────────────────────────────────
//  Math helpers
// ─────────────────────────────────────────────────────────────────────────────

function avg(values: number[]): number {
	return values.reduce((a, b) => a + b, 0) / values.length;
}

function median(values: number[]): number {
	const s = [...values].sort((a, b) => a - b);
	const mid = Math.floor(s.length / 2);
	return s.length % 2 === 0 ? (s[mid - 1] + s[mid]) / 2 : s[mid];
}
