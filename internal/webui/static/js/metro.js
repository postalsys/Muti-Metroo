// Metro Map Visualization for Muti Metroo

class MetroMap {
    constructor(svgElement) {
        this.svg = svgElement;
        this.connectionsLayer = document.getElementById('connections-layer');
        this.stationsLayer = document.getElementById('stations-layer');
        this.agents = new Map();
        this.connections = [];
        this.routes = []; // Route data for showing exit CIDRs
        this.gridSize = 200;
        this.stationRadius = 14;
        this.localStationRadius = 18;
        this.lastTopologyHash = null;
        this.positionCache = new Map(); // Cache positions by agent ID

        // Tooltip state
        this.stationTooltip = null;
        this.connectionTooltip = null;
        this.tooltipHovered = false;
        this.tooltipHideTimeout = null;
        this.currentHoveredStation = null;

        this.createTooltips();
    }

    // Update route data for showing exit CIDRs in tooltips
    setRoutes(routes) {
        this.routes = routes || [];
    }

    // Get exit CIDRs for a given agent
    getExitCIDRs(agentShortId) {
        return this.routes
            .filter(r => r.origin_id === agentShortId)
            .map(r => r.network);
    }

    createTooltips() {
        // Station tooltip
        this.stationTooltip = document.createElement('div');
        this.stationTooltip.className = 'station-tooltip';
        this.stationTooltip.style.display = 'none';
        this.stationTooltip.innerHTML = `
            <div class="tooltip-header">
                <div class="tooltip-name"></div>
            </div>
            <div class="tooltip-roles"></div>
            <div class="tooltip-info"></div>
            <div class="tooltip-socks5"></div>
            <div class="tooltip-udp"></div>
            <div class="tooltip-exits"></div>
            <div class="tooltip-domains"></div>
            <div class="tooltip-id-section">
                <div class="tooltip-id-label">Agent ID</div>
                <div class="tooltip-id"></div>
                <div class="tooltip-id-hint">Click to copy</div>
            </div>
        `;
        document.body.appendChild(this.stationTooltip);

        // Station tooltip hover events
        this.stationTooltip.addEventListener('mouseenter', () => {
            this.tooltipHovered = true;
            if (this.tooltipHideTimeout) {
                clearTimeout(this.tooltipHideTimeout);
                this.tooltipHideTimeout = null;
            }
        });

        this.stationTooltip.addEventListener('mouseleave', () => {
            this.tooltipHovered = false;
            this.hideStationTooltip();
        });

        // Click to copy ID
        const idSection = this.stationTooltip.querySelector('.tooltip-id-section');
        idSection.addEventListener('click', () => {
            const idEl = this.stationTooltip.querySelector('.tooltip-id');
            const id = idEl.textContent;
            navigator.clipboard.writeText(id).then(() => {
                const hint = this.stationTooltip.querySelector('.tooltip-id-hint');
                hint.textContent = 'Copied!';
                hint.classList.add('tooltip-id-copied');
                setTimeout(() => {
                    hint.textContent = 'Click to copy';
                    hint.classList.remove('tooltip-id-copied');
                }, 1500);
            });
        });

        // Connection tooltip
        this.connectionTooltip = document.createElement('div');
        this.connectionTooltip.className = 'connection-tooltip';
        this.connectionTooltip.style.display = 'none';
        document.body.appendChild(this.connectionTooltip);
    }

    // Create a hash of topology to detect changes
    hashTopology(topology) {
        const agentIds = topology.agents.map(a => a.short_id).sort().join(',');
        const connIds = topology.connections.map(c => `${c.from_agent}-${c.to_agent}`).sort().join(',');
        return `${agentIds}|${connIds}`;
    }

    update(topology) {
        // Check if topology actually changed
        const newHash = this.hashTopology(topology);
        if (newHash === this.lastTopologyHash) {
            return; // No changes, skip redraw
        }
        this.lastTopologyHash = newHash;

        this.agents.clear();
        this.connections = [];

        // Parse topology into internal structure
        for (const agent of topology.agents) {
            this.agents.set(agent.short_id, {
                ...agent,
                x: 0,
                y: 0
            });
        }

        for (const conn of topology.connections) {
            this.connections.push(conn);
        }

        // Calculate layout using tree-based approach
        this.calculateTreeLayout();

        // Adjust viewBox to fit content
        this.fitViewBoxToContent();

        // Render
        this.render();
    }

    fitViewBoxToContent() {
        if (this.agents.size === 0) return;

        // Calculate bounds of all agents
        let minX = Infinity, maxX = -Infinity;
        let minY = Infinity, maxY = -Infinity;

        this.agents.forEach(agent => {
            minX = Math.min(minX, agent.x);
            maxX = Math.max(maxX, agent.x);
            minY = Math.min(minY, agent.y);
            maxY = Math.max(maxY, agent.y);
        });

        // Add padding for labels and station circles
        const padding = 100;
        minX -= padding;
        maxX += padding;
        minY -= padding;
        maxY += padding;

        // Calculate content dimensions
        const contentWidth = maxX - minX;
        const contentHeight = Math.max(maxY - minY, 150); // Minimum height for horizontal layouts

        // Set viewBox to fit content
        this.svg.setAttribute('viewBox', `${minX} ${minY} ${contentWidth} ${contentHeight}`);
    }

    calculateTreeLayout() {
        const viewBox = this.svg.viewBox.baseVal;
        const width = viewBox.width;
        const height = viewBox.height;

        // Find local agent (root of layout)
        const localAgent = [...this.agents.values()].find(a => a.is_local);
        if (!localAgent) return;

        // Build bidirectional adjacency list from connections
        const adjacency = new Map();
        for (const agent of this.agents.values()) {
            adjacency.set(agent.short_id, new Set());
        }
        for (const conn of this.connections) {
            adjacency.get(conn.from_agent)?.add(conn.to_agent);
            adjacency.get(conn.to_agent)?.add(conn.from_agent);
        }

        // BFS-based layout with 8-direction subway-style constraints
        const positioned = new Set();
        const positions = new Map();
        const usedPositions = new Set();

        // Preferred directions for subway layout (45 and 90 degree angles)
        // Order matters: prefer east first for left-to-right tree layout
        const directions = [
            { dx: 1, dy: 0 },   // East (primary direction)
            { dx: 1, dy: 1 },   // Southeast
            { dx: 1, dy: -1 },  // Northeast
            { dx: 0, dy: 1 },   // South
            { dx: 0, dy: -1 },  // North
            { dx: -1, dy: 0 },  // West
            { dx: -1, dy: 1 },  // Southwest
            { dx: -1, dy: -1 }, // Northwest
        ];

        // Start with local agent at center (0,0 in grid coordinates)
        positions.set(localAgent.short_id, { x: 0, y: 0 });
        positioned.add(localAgent.short_id);
        usedPositions.add('0,0');

        const queue = [{ id: localAgent.short_id, x: 0, y: 0 }];

        while (queue.length > 0) {
            const current = queue.shift();
            const neighbors = Array.from(adjacency.get(current.id) || []);

            // Sort neighbors by ID for deterministic order
            neighbors.sort();

            let dirIndex = 0;
            for (const neighborId of neighbors) {
                if (!positioned.has(neighborId)) {
                    // Find a free position
                    let newX, newY;
                    let found = false;

                    // Try different directions
                    for (let attempt = 0; attempt < directions.length; attempt++) {
                        const dir = directions[(dirIndex + attempt) % directions.length];
                        newX = current.x + dir.dx;
                        newY = current.y + dir.dy;
                        const posKey = `${newX},${newY}`;

                        if (!usedPositions.has(posKey)) {
                            usedPositions.add(posKey);
                            found = true;
                            break;
                        }
                    }

                    if (!found) {
                        // Fallback: find any free position extending right
                        for (let dx = 1; dx <= 3; dx++) {
                            for (let dy = -2; dy <= 2; dy++) {
                                newX = current.x + dx;
                                newY = current.y + dy;
                                const posKey = `${newX},${newY}`;
                                if (!usedPositions.has(posKey)) {
                                    usedPositions.add(posKey);
                                    found = true;
                                    break;
                                }
                            }
                            if (found) break;
                        }
                    }

                    if (found) {
                        positions.set(neighborId, { x: newX, y: newY });
                        positioned.add(neighborId);
                        queue.push({ id: neighborId, x: newX, y: newY });
                    }

                    dirIndex++;
                }
            }
        }

        // Calculate bounds in grid coordinates
        let minX = Infinity, maxX = -Infinity;
        let minY = Infinity, maxY = -Infinity;

        positions.forEach((pos) => {
            minX = Math.min(minX, pos.x);
            maxX = Math.max(maxX, pos.x);
            minY = Math.min(minY, pos.y);
            maxY = Math.max(maxY, pos.y);
        });

        // Center the layout in the viewport
        const gridWidth = maxX - minX + 1;
        const gridHeight = maxY - minY + 1;
        const centerOffsetX = (minX + maxX) / 2;
        const centerOffsetY = (minY + maxY) / 2;

        // Apply positions with grid spacing, centered in viewport
        const centerX = width / 2;
        const centerY = height / 2;

        this.agents.forEach((agent) => {
            const pos = positions.get(agent.short_id);
            if (pos) {
                agent.x = centerX + (pos.x - centerOffsetX) * this.gridSize;
                agent.y = centerY + (pos.y - centerOffsetY) * this.gridSize;
            } else {
                // Fallback for unpositioned agents
                agent.x = centerX;
                agent.y = centerY;
            }
        });

        // Assign label positions based on neighbor positions
        // For horizontal layout, prefer above/below to avoid overlapping horizontal connections
        this.agents.forEach(agent => {
            const pos = positions.get(agent.short_id);
            if (!pos) {
                agent.labelPos = 'above';
                return;
            }

            // Check which directions have neighbors
            const neighbors = adjacency.get(agent.short_id) || new Set();
            let hasAbove = false, hasBelow = false, hasLeft = false, hasRight = false;

            neighbors.forEach(neighborId => {
                const neighborPos = positions.get(neighborId);
                if (neighborPos) {
                    if (neighborPos.y < pos.y) hasAbove = true;
                    if (neighborPos.y > pos.y) hasBelow = true;
                    if (neighborPos.x < pos.x) hasLeft = true;
                    if (neighborPos.x > pos.x) hasRight = true;
                }
            });

            // Choose label position to avoid overlapping connections
            // For horizontal layout, prefer above/below since connections run horizontally
            if (!hasAbove) {
                agent.labelPos = 'above';
            } else if (!hasBelow) {
                agent.labelPos = 'below';
            } else if (!hasLeft) {
                agent.labelPos = 'left';
            } else if (!hasRight) {
                agent.labelPos = 'right';
            } else {
                agent.labelPos = 'above';
            }
        });
    }

    snapToGrid(value) {
        return Math.round(value / this.gridSize) * this.gridSize;
    }

    render() {
        // Clear existing content
        this.connectionsLayer.innerHTML = '';
        this.stationsLayer.innerHTML = '';

        // Check for empty state
        if (this.agents.size === 0) {
            this.renderEmptyState();
            return;
        }

        // Render connections first (behind stations)
        for (const conn of this.connections) {
            const from = this.agents.get(conn.from_agent);
            const to = this.agents.get(conn.to_agent);
            if (from && to) {
                this.renderConnection(from, to, conn);
            }
        }

        // Render stations
        // Sort by ID for deterministic render order
        const sortedAgents = [...this.agents.entries()].sort((a, b) => a[0].localeCompare(b[0]));
        for (const [id, agent] of sortedAgents) {
            this.renderStation(agent);
        }
    }

    renderEmptyState() {
        const viewBox = this.svg.viewBox.baseVal;
        const text = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        text.setAttribute('class', 'empty-state');
        text.setAttribute('x', viewBox.width / 2);
        text.setAttribute('y', viewBox.height / 2);
        text.textContent = 'No topology data available';
        this.stationsLayer.appendChild(text);
    }

    renderConnection(from, to, conn) {
        const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
        g.setAttribute('class', 'connection-group');
        g.setAttribute('data-from-id', from.short_id);
        g.setAttribute('data-to-id', to.short_id);

        const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        path.setAttribute('d', this.createMetroPath(from.x, from.y, to.x, to.y));
        let connClass = `connection ${conn.is_direct ? 'direct' : 'indirect'}`;
        if (conn.unresponsive) {
            connClass += ' unresponsive';
        }
        path.setAttribute('class', connClass);
        g.appendChild(path);

        // Connection hover events
        g.addEventListener('mouseenter', (e) => this.onConnectionHover(e, from, to, conn));
        g.addEventListener('mousemove', (e) => this.onConnectionMove(e));
        g.addEventListener('mouseleave', () => this.onConnectionLeave());

        this.connectionsLayer.appendChild(g);
    }

    onConnectionHover(event, from, to, conn) {
        const fromName = from.display_name || from.short_id;
        const toName = to.display_name || to.short_id;

        let html = `
            <div class="connection-direction">
                <span>${fromName}</span>
                <svg class="connection-arrow" viewBox="0 0 24 24" width="16" height="16">
                    <path d="M4 12h14m-4-4l4 4-4 4" stroke="currentColor" stroke-width="2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
                <span>${toName}</span>
            </div>
            <div class="connection-role">${fromName} (dialer) connects to ${toName} (listener)</div>
        `;

        if (conn.transport) {
            const transportName = {
                'quic': 'QUIC',
                'h2': 'HTTP/2',
                'ws': 'WebSocket'
            }[conn.transport] || conn.transport.toUpperCase();
            html += `<div class="connection-transport">Transport: ${transportName}</div>`;
        }

        if (conn.rtt_ms && conn.rtt_ms > 0) {
            html += `<div class="connection-rtt">RTT: ${conn.rtt_ms}ms</div>`;
        }

        if (conn.unresponsive) {
            html += `<div class="connection-rtt" style="color: #dc3545; font-weight: 600;">UNRESPONSIVE (RTT > 60s)</div>`;
        }

        this.connectionTooltip.innerHTML = html;
        this.connectionTooltip.style.display = 'block';
        this.positionConnectionTooltip(event);
    }

    onConnectionMove(event) {
        this.positionConnectionTooltip(event);
    }

    onConnectionLeave() {
        this.connectionTooltip.style.display = 'none';
    }

    positionConnectionTooltip(event) {
        const tooltip = this.connectionTooltip;
        const rect = tooltip.getBoundingClientRect();
        let x = event.clientX + 15;
        let y = event.clientY + 15;

        // Keep tooltip within viewport
        if (x + rect.width > window.innerWidth) {
            x = event.clientX - rect.width - 15;
        }
        if (y + rect.height > window.innerHeight) {
            y = event.clientY - rect.height - 15;
        }

        tooltip.style.left = `${x}px`;
        tooltip.style.top = `${y}px`;
    }

    createMetroPath(x1, y1, x2, y2) {
        const dx = x2 - x1;
        const dy = y2 - y1;

        // If nearly aligned horizontally or vertically, draw straight line
        if (Math.abs(dx) < 10) {
            return `M${x1},${y1} L${x2},${y2}`;
        }
        if (Math.abs(dy) < 10) {
            return `M${x1},${y1} L${x2},${y2}`;
        }

        // Create angular path - horizontal first (tree goes right), then diagonal
        const diagonalLength = Math.min(Math.abs(dx), Math.abs(dy));

        // Go horizontal first, then diagonal to destination
        const midX = x1 + (Math.abs(dx) - diagonalLength) * Math.sign(dx);
        return `M${x1},${y1} H${midX} L${x2},${y2}`;
    }

    renderStation(agent) {
        const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
        const stationType = agent.is_local ? 'local' : (agent.is_connected ? 'connected' : 'remote');
        g.setAttribute('class', `station ${stationType}`);
        g.setAttribute('transform', `translate(${agent.x},${agent.y})`);
        g.setAttribute('data-agent-id', agent.short_id);

        const radius = agent.is_local ? this.localStationRadius : this.stationRadius;

        // Role indicator circles (concentric rings)
        // Draw from outermost to innermost so they layer correctly
        const roles = agent.roles || ['transit'];
        const roleRadiusBase = radius + 4;  // Start outside the station circle
        const roleSpacing = 5;  // Gap between role rings

        // Render role circles in reverse order (outermost first) so inner ones are on top
        const sortedRoles = [...roles].reverse();
        sortedRoles.forEach((role, index) => {
            const roleCircle = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
            const roleRadius = roleRadiusBase + (sortedRoles.length - 1 - index) * roleSpacing;
            roleCircle.setAttribute('class', `station-role ${role}`);
            roleCircle.setAttribute('r', roleRadius);
            g.appendChild(roleCircle);
        });

        // Outer circle (border)
        const outer = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
        outer.setAttribute('class', 'station-outer');
        outer.setAttribute('r', radius);

        // Inner circle (fill)
        const inner = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
        inner.setAttribute('class', 'station-inner');
        inner.setAttribute('r', radius - 4);

        g.appendChild(outer);
        g.appendChild(inner);

        // Label - adjust position based on role rings
        const totalRoleRadius = roleRadiusBase + (roles.length - 1) * roleSpacing;
        const label = document.createElementNS('http://www.w3.org/2000/svg', 'text');
        label.setAttribute('class', `station-label ${agent.labelPos || 'below'}`);

        // Position label based on direction, accounting for role rings
        switch (agent.labelPos) {
            case 'above':
                label.setAttribute('y', -(totalRoleRadius + 6));
                break;
            case 'below':
                label.setAttribute('y', totalRoleRadius + 16);
                break;
            case 'left':
                label.setAttribute('x', -(totalRoleRadius + 6));
                label.setAttribute('y', 4);
                break;
            case 'right':
                label.setAttribute('x', totalRoleRadius + 6);
                label.setAttribute('y', 4);
                break;
        }

        label.textContent = agent.display_name || agent.short_id;
        g.appendChild(label);

        // Add hover event
        g.addEventListener('mouseenter', (e) => this.onStationHover(agent, g, e));
        g.addEventListener('mouseleave', () => this.onStationLeave(agent, g));

        this.stationsLayer.appendChild(g);
    }

    onStationHover(agent, element, event) {
        element.classList.add('selected');
        this.currentHoveredStation = agent.short_id;

        // Clear any pending hide timeout
        if (this.tooltipHideTimeout) {
            clearTimeout(this.tooltipHideTimeout);
            this.tooltipHideTimeout = null;
        }

        // Update tooltip content
        const nameEl = this.stationTooltip.querySelector('.tooltip-name');
        nameEl.textContent = agent.display_name || agent.short_id;

        // Build roles section
        const rolesEl = this.stationTooltip.querySelector('.tooltip-roles');
        const roles = agent.roles || ['transit'];
        if (roles.length > 0) {
            rolesEl.innerHTML = roles.map(role =>
                `<span class="tooltip-role-badge ${role}">${role}</span>`
            ).join('');
            rolesEl.style.display = 'flex';
        } else {
            rolesEl.style.display = 'none';
        }

        // Build info section
        const infoEl = this.stationTooltip.querySelector('.tooltip-info');
        let infoHtml = '';

        if (agent.hostname) {
            infoHtml += `<div class="tooltip-info-line"><span class="tooltip-info-label">Hostname</span><span class="tooltip-info-value">${agent.hostname}</span></div>`;
        }
        if (agent.os || agent.arch) {
            const platform = [agent.os, agent.arch].filter(Boolean).join('/');
            infoHtml += `<div class="tooltip-info-line"><span class="tooltip-info-label">Platform</span><span class="tooltip-info-value">${platform}</span></div>`;
        }
        if (agent.version) {
            infoHtml += `<div class="tooltip-info-line"><span class="tooltip-info-label">Version</span><span class="tooltip-info-value">${agent.version}</span></div>`;
        }
        if (agent.uptime_hours !== undefined && agent.uptime_hours > 0) {
            const hours = agent.uptime_hours;
            let uptimeStr;
            if (hours < 1) {
                uptimeStr = `${Math.round(hours * 60)}m`;
            } else if (hours < 24) {
                uptimeStr = `${Math.round(hours)}h`;
            } else {
                const days = Math.floor(hours / 24);
                const remainingHours = Math.round(hours % 24);
                uptimeStr = `${days}d ${remainingHours}h`;
            }
            infoHtml += `<div class="tooltip-info-line"><span class="tooltip-info-label">Uptime</span><span class="tooltip-info-value">${uptimeStr}</span></div>`;
        }
        if (agent.ip_addresses && agent.ip_addresses.length > 0) {
            // Show first 2 IPs max
            const ips = agent.ip_addresses.slice(0, 2).join(', ');
            const suffix = agent.ip_addresses.length > 2 ? ` (+${agent.ip_addresses.length - 2})` : '';
            infoHtml += `<div class="tooltip-info-line"><span class="tooltip-info-label">IPs</span><span class="tooltip-info-value">${ips}${suffix}</span></div>`;
        }

        infoEl.innerHTML = infoHtml;

        // Build SOCKS5 info section (for ingress agents)
        const socks5El = this.stationTooltip.querySelector('.tooltip-socks5');
        if (agent.socks5_addr) {
            socks5El.innerHTML = `
                <div class="tooltip-socks5-header">SOCKS5 Proxy</div>
                <div class="tooltip-socks5-addr">${agent.socks5_addr}</div>
            `;
            socks5El.style.display = 'block';
        } else {
            socks5El.innerHTML = '';
            socks5El.style.display = 'none';
        }

        // Build UDP relay info section (for exit agents)
        const udpEl = this.stationTooltip.querySelector('.tooltip-udp');
        if (agent.udp_enabled) {
            udpEl.innerHTML = `
                <div class="tooltip-udp-header">UDP Relay</div>
                <div class="tooltip-udp-status">Enabled</div>
            `;
            udpEl.style.display = 'block';
        } else {
            udpEl.innerHTML = '';
            udpEl.style.display = 'none';
        }

        // Build exit routes section (CIDR routes)
        const exitsEl = this.stationTooltip.querySelector('.tooltip-exits');
        const exitRoutes = agent.exit_routes || [];
        if (exitRoutes.length > 0) {
            let exitsHtml = '<div class="tooltip-exits-header">CIDR Routes</div>';
            exitsHtml += '<div class="tooltip-exits-list">';
            // Show first 5 CIDRs, with count if more
            const displayCIDRs = exitRoutes.slice(0, 5);
            exitsHtml += displayCIDRs.map(cidr => `<span class="tooltip-cidr">${cidr}</span>`).join('');
            if (exitRoutes.length > 5) {
                exitsHtml += `<span class="tooltip-cidr-more">+${exitRoutes.length - 5} more</span>`;
            }
            exitsHtml += '</div>';
            exitsEl.innerHTML = exitsHtml;
            exitsEl.style.display = 'block';
        } else {
            exitsEl.innerHTML = '';
            exitsEl.style.display = 'none';
        }

        // Build domain routes section
        const domainsEl = this.stationTooltip.querySelector('.tooltip-domains');
        const domainRoutes = agent.domain_routes || [];
        if (domainRoutes.length > 0) {
            let domainsHtml = '<div class="tooltip-exits-header">Domain Routes</div>';
            domainsHtml += '<div class="tooltip-exits-list">';
            // Show first 5 domains, with count if more
            const displayDomains = domainRoutes.slice(0, 5);
            domainsHtml += displayDomains.map(domain => `<span class="tooltip-domain">${domain}</span>`).join('');
            if (domainRoutes.length > 5) {
                domainsHtml += `<span class="tooltip-cidr-more">+${domainRoutes.length - 5} more</span>`;
            }
            domainsHtml += '</div>';
            domainsEl.innerHTML = domainsHtml;
            domainsEl.style.display = 'block';
        } else {
            domainsEl.innerHTML = '';
            domainsEl.style.display = 'none';
        }

        // Update ID
        const idEl = this.stationTooltip.querySelector('.tooltip-id');
        idEl.textContent = agent.id || agent.short_id;

        // Reset hint
        const hint = this.stationTooltip.querySelector('.tooltip-id-hint');
        hint.textContent = 'Click to copy';
        hint.classList.remove('tooltip-id-copied');

        // Position and show tooltip
        this.stationTooltip.style.display = 'block';
        this.positionStationTooltip(element);
    }

    positionStationTooltip(stationElement) {
        const tooltip = this.stationTooltip;
        const svgRect = this.svg.getBoundingClientRect();
        const stationRect = stationElement.getBoundingClientRect();

        // Position tooltip to the right of the station
        let x = stationRect.right + 10;
        let y = stationRect.top;

        // After showing, adjust if it goes off screen
        setTimeout(() => {
            const tooltipRect = tooltip.getBoundingClientRect();

            // Keep within viewport
            if (x + tooltipRect.width > window.innerWidth - 10) {
                x = stationRect.left - tooltipRect.width - 10;
            }
            if (y + tooltipRect.height > window.innerHeight - 10) {
                y = window.innerHeight - tooltipRect.height - 10;
            }
            if (y < 10) {
                y = 10;
            }

            tooltip.style.left = `${x}px`;
            tooltip.style.top = `${y}px`;
        }, 0);

        tooltip.style.left = `${x}px`;
        tooltip.style.top = `${y}px`;
    }

    onStationLeave(agent, element) {
        element.classList.remove('selected');

        // Delay hiding to allow moving to tooltip
        this.tooltipHideTimeout = setTimeout(() => {
            if (!this.tooltipHovered) {
                this.hideStationTooltip();
            }
        }, 150);
    }

    hideStationTooltip() {
        this.stationTooltip.style.display = 'none';
        this.currentHoveredStation = null;
    }

    // Highlight a path (array of short IDs) on the map
    highlightPath(pathIds) {
        if (!pathIds || pathIds.length === 0) return;

        // Create a Set for faster lookups
        const pathSet = new Set(pathIds);

        // Highlight stations in the path
        const stations = this.stationsLayer.querySelectorAll('.station');
        stations.forEach(station => {
            const agentId = station.dataset.agentId;
            if (pathSet.has(agentId)) {
                station.classList.add('path-highlighted');
            } else {
                station.classList.add('path-dimmed');
            }
        });

        // Highlight connections that are part of the path
        // A connection is highlighted if both endpoints are in the path and adjacent
        const connections = this.connectionsLayer.querySelectorAll('.connection-group');
        connections.forEach(connGroup => {
            const fromId = connGroup.dataset.fromId;
            const toId = connGroup.dataset.toId;

            // Check if this connection is between adjacent nodes in the path
            let isInPath = false;
            for (let i = 0; i < pathIds.length - 1; i++) {
                if ((pathIds[i] === fromId && pathIds[i + 1] === toId) ||
                    (pathIds[i] === toId && pathIds[i + 1] === fromId)) {
                    isInPath = true;
                    break;
                }
            }

            if (isInPath) {
                connGroup.classList.add('path-highlighted');
            } else {
                connGroup.classList.add('path-dimmed');
            }
        });
    }

    // Clear all path highlighting
    clearHighlight() {
        // Remove highlighting from stations
        const stations = this.stationsLayer.querySelectorAll('.station');
        stations.forEach(station => {
            station.classList.remove('path-highlighted', 'path-dimmed');
        });

        // Remove highlighting from connections
        const connections = this.connectionsLayer.querySelectorAll('.connection-group');
        connections.forEach(conn => {
            conn.classList.remove('path-highlighted', 'path-dimmed');
        });
    }
}

// Export for use in app.js
window.MetroMap = MetroMap;
